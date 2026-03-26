package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	signet "github.com/VYLTH/signet-sdk-go/signet"

	"github.com/iamdecatalyst/hummingbird/orchestrator/alerts"
	"github.com/iamdecatalyst/hummingbird/orchestrator/auth"
	"github.com/iamdecatalyst/hummingbird/orchestrator/bot"
	"github.com/iamdecatalyst/hummingbird/orchestrator/config"
	"github.com/iamdecatalyst/hummingbird/orchestrator/db"
	"github.com/iamdecatalyst/hummingbird/orchestrator/eventlog"
	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
	"github.com/iamdecatalyst/hummingbird/orchestrator/portfolio"
	"github.com/iamdecatalyst/hummingbird/orchestrator/trader"
	"github.com/iamdecatalyst/hummingbird/orchestrator/userbot"
)

func main() {
	cfg := config.Load()
	mux := http.NewServeMux()

	// Always public — lets the frontend know which mode to render
	mux.HandleFunc("GET /mode", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"multi_tenant":%v}`, cfg.MultiTenant)
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"status":"ok","multi_tenant":%v}`, cfg.MultiTenant)
	})

	if cfg.MultiTenant {
		startMultiTenant(cfg, mux)
	} else {
		startSingleTenant(cfg, mux)
	}

	addr := ":" + cfg.Port
	log.Printf("[main] Hummingbird listening on %s (multi_tenant=%v)", addr, cfg.MultiTenant)
	if err := http.ListenAndServe(addr, withCORS(mux)); err != nil {
		log.Fatalf("[main] server error: %v", err)
	}
}

// ── Single-tenant mode ────────────────────────────────────────────────────────

func startSingleTenant(cfg *config.Config, mux *http.ServeMux) {
	configured := false
	var walletID string
	var err error

	signetClient := signet.NewClient(cfg.SignetAPIKey, cfg.SignetAPISecret).
		WithBaseURL(cfg.SignetBaseURL)

	wid, err := trader.EnsureWallet(signetClient, "hummingbird-trader")
	if err != nil {
		log.Printf("[main] WARNING: wallet setup failed: %v", err)
		log.Printf("[main] Unconfigured mode — set SIGNET_API_KEY + SIGNET_API_SECRET in .env and restart.")
	} else {
		walletID = wid
		configured = true
	}

	port := portfolio.New(1.0, cfg.MaxConcurrentPositions, cfg.MaxDailyLossPercent)

	var notifier alerts.Notifier = noopNotifier{}
	var tgBot *bot.Bot

	if cfg.TelegramToken != "" && cfg.TelegramChatID != "" {
		chatID, parseErr := strconv.ParseInt(cfg.TelegramChatID, 10, 64)
		if parseErr == nil {
			tgBot, err = bot.New(cfg.TelegramToken, chatID, port, nil)
			if err != nil {
				log.Printf("[main] bot init failed: %v", err)
			} else {
				notifier = tgBot
			}
		}
	}

	tr := trader.New(signetClient, walletID, port, notifier, cfg.SolanaRPC, "http://localhost:8001")

	if tgBot != nil {
		tgBot.SetExecutor(tr)
		go tgBot.Run()
	}
	go dailyStats(tgBot, port)

	if configured {
		log.Printf("[main] wallet=%s | max_pos=%d | daily_loss_limit=%.0f%%",
			walletID, cfg.MaxConcurrentPositions, cfg.MaxDailyLossPercent*100)
		notifier.Alert("Hummingbird is online and watching pump.fun")
	} else {
		log.Printf("[main] UNCONFIGURED — trading disabled")
	}

	type statsResp struct {
		portfolio.Stats
		Configured bool `json:"configured"`
	}

	mux.HandleFunc("POST /trade", func(w http.ResponseWriter, r *http.Request) {
		if !configured {
			http.Error(w, `{"error":"not configured"}`, http.StatusServiceUnavailable)
			return
		}
		var result models.ScoreResult
		if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		tr.Execute(&result)
		fmt.Fprint(w, `{"status":"queued"}`)
	})

	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(statsResp{port.Stats(), configured})
	})

	mux.HandleFunc("GET /positions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(port.OpenPositions())
	})

	mux.HandleFunc("GET /closed", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(port.RecentClosed(50))
	})

	mux.HandleFunc("GET /logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(eventlog.All())
	})

	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, r *http.Request) {
		if !configured {
			fmt.Fprint(w, `{"status":"not configured"}`)
			return
		}
		tr.ExitAll(models.ExitManual)
		notifier.Alert("Bot stopped manually.")
		fmt.Fprint(w, `{"status":"stopped"}`)
	})

	mux.HandleFunc("POST /resume", func(w http.ResponseWriter, r *http.Request) {
		port.Resume()
		fmt.Fprint(w, `{"status":"resumed"}`)
	})
}

// ── Multi-tenant mode ─────────────────────────────────────────────────────────
// Identity = Nexus user ID (VYLTH SSO).
// Flow: user clicks "Continue with Nexus" → OAuth → Nexus access token sent to backend
//       → backend validates via Nexus profile API → upsert user → issue our JWT.
// First login: prompt for Signet API key once → encrypted in DB → bot starts.
// Subsequent logins: Nexus → JWT → bot resumes from stored credentials.

func startMultiTenant(cfg *config.Config, mux *http.ServeMux) {
	database, err := db.New(cfg.DatabaseURL, cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("[main] db init failed: %v", err)
	}
	log.Printf("[main] Postgres connected, multi-tenant mode active (Nexus SSO)")

	mgr := userbot.NewManager(cfg)

	// Resume bots for all configured users on startup
	go func() {
		ids, err := database.AllConfiguredUsers()
		if err != nil {
			log.Printf("[main] failed to load users: %v", err)
			return
		}
		for _, uid := range ids {
			apiKey, apiSecret, err := database.GetSignetCredentials(uid)
			if err != nil {
				log.Printf("[main] skip resume for user %s: %v", uid[:8], err)
				continue
			}
			if err := mgr.Start(uid, apiKey, apiSecret); err != nil {
				log.Printf("[main] resume failed for user %s: %v", uid[:8], err)
			}
		}
	}()

	requireAuth := func(r *http.Request) (string, error) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return "", fmt.Errorf("missing token")
		}
		return auth.ParseToken(strings.TrimPrefix(h, "Bearer "), cfg.JWTSecret)
	}

	// validateNexusToken calls the Nexus profile endpoint to verify the token
	// and returns the user's Nexus profile.
	validateNexusToken := func(accessToken string) (id, username, firstName, lastName, email, avatar string, err error) {
		req, _ := http.NewRequest("GET", "https://auth.vylth.com/api/nexus/account/profile", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", "", "", "", "", "", fmt.Errorf("nexus unreachable: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", "", "", "", "", "", fmt.Errorf("invalid nexus token (status %d)", resp.StatusCode)
		}
		var envelope struct {
			Investor struct {
				ID        string `json:"id"`
				Username  string `json:"username"`
				Email     string `json:"email"`
				FirstName string `json:"first_name"`
				LastName  string `json:"last_name"`
				Avatar    string `json:"avatar_url"`
			} `json:"investor"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
			return "", "", "", "", "", "", fmt.Errorf("decode profile: %w", err)
		}
		p := envelope.Investor
		if p.ID == "" {
			return "", "", "", "", "", "", fmt.Errorf("nexus profile missing id")
		}
		return p.ID, p.Username, p.FirstName, p.LastName, p.Email, p.Avatar, nil
	}

	// POST /auth/nexus — exchange Nexus access token for a Hummingbird JWT.
	// Called by the frontend after the Nexus OAuth callback.
	mux.HandleFunc("POST /auth/nexus", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AccessToken == "" {
			http.Error(w, `{"error":"access_token required"}`, http.StatusBadRequest)
			return
		}

		nexusID, username, firstName, lastName, email, avatar, err := validateNexusToken(req.AccessToken)
		if err != nil {
			log.Printf("[auth/nexus] validation failed: %v", err)
			http.Error(w, `{"error":"invalid Nexus token"}`, http.StatusUnauthorized)
			return
		}

		// Upsert profile (updates name/avatar on every login)
		if err := database.UpsertProfile(nexusID, username, firstName, lastName, email, avatar); err != nil {
			log.Printf("[auth/nexus] upsert failed for %s: %v", nexusID, err)
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}

		user, _ := database.GetUser(nexusID)
		token, _ := auth.IssueToken(nexusID, cfg.JWTSecret)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"token":      token,
			"has_signet": user != nil && user.HasSignet,
			"user": map[string]string{
				"id":         nexusID,
				"username":   username,
				"first_name": firstName,
				"last_name":  lastName,
				"email":      email,
				"avatar":     avatar,
			},
		})
	})

	// GET /auth/me
	mux.HandleFunc("GET /auth/me", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user, err := database.GetUser(nexusID)
		if err != nil || user == nil {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		inst := mgr.Get(nexusID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":         user.NexusUserID,
			"username":   user.Username,
			"first_name": user.FirstName,
			"last_name":  user.LastName,
			"email":      user.Email,
			"avatar":     user.Avatar,
			"has_signet": user.HasSignet,
			"wallet_id":  user.WalletID,
			"bot_active": inst != nil,
		})
	})

	// POST /auth/setup-signet — first-time Signet key entry after Nexus login.
	mux.HandleFunc("POST /auth/setup-signet", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		var req struct {
			APIKey    string `json:"api_key"`
			APISecret string `json:"api_secret"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.APIKey == "" || req.APISecret == "" {
			http.Error(w, `{"error":"api_key and api_secret required"}`, http.StatusBadRequest)
			return
		}

		// Verify credentials + get wallet ID
		client := signet.NewClient(req.APIKey, req.APISecret).WithBaseURL(cfg.SignetBaseURL)
		short := nexusID
		if len(short) > 8 {
			short = short[:8]
		}
		walletName := fmt.Sprintf("hb-%s", short)
		walletID, err := trader.EnsureWallet(client, walletName)
		if err != nil {
			http.Error(w, `{"error":"invalid Signet credentials"}`, http.StatusBadRequest)
			return
		}

		if err := database.SetSignetCredentials(nexusID, req.APIKey, req.APISecret, walletID); err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}

		// Start the bot
		if mgr.Get(nexusID) == nil {
			mgr.Start(nexusID, req.APIKey, req.APISecret)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok","bot_active":true}`)
	})

	// Per-user trading endpoints (keyed by Nexus user ID)

	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		inst := mgr.Get(nexusID)
		type statsResp struct {
			portfolio.Stats
			Configured bool `json:"configured"`
		}
		w.Header().Set("Content-Type", "application/json")
		if inst == nil {
			user, _ := database.GetUser(nexusID)
			json.NewEncoder(w).Encode(statsResp{portfolio.Stats{}, user != nil && user.HasSignet})
			return
		}
		json.NewEncoder(w).Encode(statsResp{inst.Port.Stats(), true})
	})

	mux.HandleFunc("GET /positions", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		inst := mgr.Get(nexusID)
		w.Header().Set("Content-Type", "application/json")
		if inst == nil {
			json.NewEncoder(w).Encode([]*models.Position{})
			return
		}
		json.NewEncoder(w).Encode(inst.Port.OpenPositions())
	})

	mux.HandleFunc("GET /closed", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		inst := mgr.Get(nexusID)
		w.Header().Set("Content-Type", "application/json")
		if inst == nil {
			json.NewEncoder(w).Encode([]*models.ClosedPosition{})
			return
		}
		json.NewEncoder(w).Encode(inst.Port.RecentClosed(50))
	})

	mux.HandleFunc("GET /logs", func(w http.ResponseWriter, r *http.Request) {
		if _, err := requireAuth(r); err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(eventlog.All())
	})

	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		mgr.Stop(nexusID)
		fmt.Fprint(w, `{"status":"stopped"}`)
	})

	mux.HandleFunc("POST /resume", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		inst := mgr.Get(nexusID)
		if inst == nil {
			apiKey, apiSecret, err := database.GetSignetCredentials(nexusID)
			if err == nil {
				mgr.Start(nexusID, apiKey, apiSecret)
			}
		} else {
			inst.Port.Resume()
		}
		fmt.Fprint(w, `{"status":"resumed"}`)
	})

	// GET /wallets — list all Signet wallets for this user
	mux.HandleFunc("GET /wallets", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		apiKey, apiSecret, err := database.GetSignetCredentials(nexusID)
		if err != nil {
			http.Error(w, `{"error":"no signet credentials"}`, http.StatusBadRequest)
			return
		}
		client := signet.NewClient(apiKey, apiSecret).WithBaseURL(cfg.SignetBaseURL)
		wallets, err := client.Wallets.List()
		if err != nil {
			http.Error(w, `{"error":"signet error"}`, http.StatusBadGateway)
			return
		}
		// Fetch SOL balance for each wallet
		type walletWithBalance struct {
			ID      string  `json:"id"`
			Address string  `json:"address"`
			Label   string  `json:"label"`
			Balance float64 `json:"balance_sol"`
		}
		result := make([]walletWithBalance, 0, len(wallets))
		for _, wal := range wallets {
			label := ""
			if wal.Label != nil {
				label = *wal.Label
			}
			bal := fetchSOLBalance(cfg.SignetBaseURL, apiKey, apiSecret, wal.ID)
			result = append(result, walletWithBalance{
				ID:      wal.ID,
				Address: wal.Address,
				Label:   label,
				Balance: bal,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	// POST /wallets — create a new Solana wallet
	mux.HandleFunc("POST /wallets", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		var req struct {
			Label string `json:"label"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		apiKey, apiSecret, err := database.GetSignetCredentials(nexusID)
		if err != nil {
			http.Error(w, `{"error":"no signet credentials"}`, http.StatusBadRequest)
			return
		}
		client := signet.NewClient(apiKey, apiSecret).WithBaseURL(cfg.SignetBaseURL)
		label := req.Label
		if label == "" {
			label = "hummingbird"
		}
		wal, err := client.Wallets.Create(signet.CreateWalletParams{Chain: "solana", Label: label})
		if err != nil {
			log.Printf("[wallets] create failed: %v", err)
			http.Error(w, `{"error":"failed to create wallet"}`, http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(wal)
	})
}

// fetchSOLBalance calls Signet balance API directly (not in SDK yet).
func fetchSOLBalance(baseURL, apiKey, apiSecret, walletID string) float64 {
	if baseURL == "" {
		baseURL = "https://api.signet.vylth.com/v1"
	}
	url := strings.TrimRight(baseURL, "/") + "/wallets/" + walletID + "/balance"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0
	}
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("X-API-Secret", apiSecret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return 0
	}
	defer resp.Body.Close()
	var result struct {
		SOL float64 `json:"sol"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.SOL
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func dailyStats(tgBot *bot.Bot, port *portfolio.Portfolio) {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		time.Sleep(time.Until(next))
		if tgBot != nil {
			tgBot.DailyStats(port.Stats())
		}
	}
}

func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

type noopNotifier struct{}

func safeShort(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func (noopNotifier) Entered(p *models.Position) {
	log.Printf("[notify] entered %s | %.3f SOL", safeShort(p.Mint), p.EntryAmountSOL)
}
func (noopNotifier) Exited(c *models.ClosedPosition) {
	log.Printf("[notify] exited %s | P&L %+.4f SOL", safeShort(c.Mint), c.PnLSOL)
}
func (noopNotifier) Alert(text string) { log.Printf("[notify] %s", text) }
