package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	signet "github.com/VYLTH/signet-sdk-go/signet"

	"github.com/iamdecatalyst/hummingbird/orchestrator/alerts"
	"github.com/iamdecatalyst/hummingbird/orchestrator/auth"
	"github.com/iamdecatalyst/hummingbird/orchestrator/bot"
	"github.com/iamdecatalyst/hummingbird/orchestrator/config"
	"github.com/iamdecatalyst/hummingbird/orchestrator/cricket"
	"github.com/iamdecatalyst/hummingbird/orchestrator/db"
	"github.com/iamdecatalyst/hummingbird/orchestrator/eventlog"
	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
	"github.com/iamdecatalyst/hummingbird/orchestrator/monitor"
	"github.com/iamdecatalyst/hummingbird/orchestrator/pnl"
	"github.com/iamdecatalyst/hummingbird/orchestrator/portfolio"
	"github.com/iamdecatalyst/hummingbird/orchestrator/scalper"
	"github.com/iamdecatalyst/hummingbird/orchestrator/trader"
	"github.com/iamdecatalyst/hummingbird/orchestrator/userbot"
)

func main() {
	cfg := config.Load()

	// ── Startup validation ────────────────────────────────────────────────────
	if cfg.MultiTenant {
		if cfg.DatabaseURL == "" {
			log.Fatal("[main] DATABASE_URL is required in multi-tenant mode")
		}
		if cfg.EncryptionKey == "" {
			log.Fatal("[main] ENCRYPTION_KEY is required in multi-tenant mode")
		}
		if cfg.CricketKey == "" {
			log.Fatal("[main] CRICKET_API_KEY is required in multi-tenant mode — get your key at https://cricket.vylth.com")
		}
	} else {
		if cfg.SignetAPIKey == "" || cfg.SignetAPISecret == "" {
			log.Printf("[main] WARNING: SIGNET_API_KEY / SIGNET_API_SECRET not set — trading disabled until configured")
		}
		if cfg.CricketKey == "" {
			log.Printf("[main] WARNING: CRICKET_API_KEY not set — all tokens will be scored as 0 and skipped")
		}
	}
	if cfg.JWTSecret == "change-me-in-production" {
		log.Printf("[main] WARNING: JWT_SECRET is using the default value — set a real secret before going live")
	}
	if cfg.TelegramToken == "" {
		log.Printf("[main] WARNING: TELEGRAM_BOT_TOKEN not set — Telegram alerts disabled")
	}

	// ── Startup banner ────────────────────────────────────────────────────────
	mode := "single-tenant"
	if cfg.MultiTenant {
		mode = "multi-tenant"
	}
	check := func(v string) string {
		if v != "" {
			return "✓"
		}
		return "✗"
	}
	log.Printf("[main] Hummingbird starting\n"+
		"  Mode:     %s\n"+
		"  Cricket:  %s %s\n"+
		"  Signet:   %s %s\n"+
		"  Telegram: %s\n"+
		"  Database: %s\n"+
		"  Listener: waiting on :%s/score",
		mode,
		check(cfg.CricketKey), cfg.CricketURL,
		check(cfg.SignetBaseURL), cfg.SignetBaseURL,
		check(cfg.TelegramToken),
		check(cfg.DatabaseURL),
		cfg.Port,
	)

	// ── Services ──────────────────────────────────────────────────────────────
	mux := http.NewServeMux()
	cc := cricket.New(cfg.CricketURL, cfg.CricketKey)

	mux.HandleFunc("GET /mode", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"multi_tenant":%v}`, cfg.MultiTenant)
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"status":"ok","multi_tenant":%v}`, cfg.MultiTenant)
	})

	if cfg.MultiTenant {
		startMultiTenant(cfg, cc, mux)
	} else {
		startSingleTenant(cfg, cc, mux)
	}

	addr := ":" + cfg.Port
	log.Printf("[main] Hummingbird listening on %s", addr)
	// CORS must wrap rate limit so 429 responses still carry Access-Control headers
	// and browsers can read the error instead of getting a opaque network failure.
	var handler http.Handler = mux
	if cfg.MultiTenant {
		handler = withRateLimit(handler, cfg.JWTSecret)
	}
	handler = withCORS(handler)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("[main] server error: %v", err)
	}
}

// ── Single-tenant mode ────────────────────────────────────────────────────────

func startSingleTenant(cfg *config.Config, cc *cricket.Client, mux *http.ServeMux) {
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

	// Scalper: finds second-wave entries via Cricket Firefly signals.
	// Uses a shared var to break the trader↔scalper init cycle.
	var tr *trader.Trader
	sc := scalper.New(cc, func(r *models.ScoreResult) {
		if tr != nil {
			tr.Execute(r)
		}
	})
	tr = trader.New(signetClient, walletID, port, notifier, cc, sc, monitor.DefaultMonitorConfig(), 0, cfg.SolanaRPC)

	go sc.Run(context.Background())

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
		WalletBalanceSOL float64 `json:"wallet_balance_sol"`
		Configured       bool    `json:"configured"`
	}

	// POST /score — receives TokenDetected from the Rust listener.
	// Scores the token via Cricket and enters a position if it passes.
	// Replaces the Python scorer service entirely.
	mux.HandleFunc("POST /score", func(w http.ResponseWriter, r *http.Request) {
		if !configured {
			http.Error(w, `{"error":"not configured"}`, http.StatusServiceUnavailable)
			return
		}
		var token cricket.TokenDetected
		if err := json.NewDecoder(r.Body).Decode(&token); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		go scoreAndTrade(cc, tr.Execute, cfg.TelegramToken, cfg.TelegramChannelID, token)
		fmt.Fprintf(w, `{"status":"queued","mint":"%s"}`, token.Mint)
	})

	// POST /trade — receives score results from Python scorer.
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
		if cfg.TelegramChannelID != "" && cfg.TelegramToken != "" {
			go broadcastTradeResult(cfg.TelegramToken, cfg.TelegramChannelID, &result)
		}
		tr.Execute(&result)
		fmt.Fprint(w, `{"status":"queued"}`)
	})

	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(statsResp{port.Stats(), tr.Balance(), configured})
	})

	mux.HandleFunc("GET /config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			MaxConcurrentPositions int     `json:"max_concurrent_positions"`
			MaxDailyLossPercent    float64 `json:"max_daily_loss_percent"`
			WalletID               string  `json:"wallet_id"`
		}{cfg.MaxConcurrentPositions, cfg.MaxDailyLossPercent, walletID})
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

func startMultiTenant(cfg *config.Config, cc *cricket.Client, mux *http.ServeMux) {
	database, err := db.New(cfg.DatabaseURL, cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("[main] db init failed: %v", err)
	}
	log.Printf("[main] Postgres connected, multi-tenant mode active (Nexus SSO)")

	// Shared scalper — dispatch is wired after mgr is created to break the init cycle.
	var mgr *userbot.Manager
	sc := scalper.New(cc, func(r *models.ScoreResult) {
		if mgr != nil {
			for _, inst := range mgr.All() {
				inst.Trader.Execute(r)
			}
		}
	})

	mgr = userbot.NewManager(cfg, database, cc, sc)

	go sc.Run(context.Background())

	// Telegram bot (multi-tenant mode)
	var tgBot *bot.Bot
	if cfg.TelegramToken != "" {
		tgBot, err = bot.NewMultiTenant(
			cfg.TelegramToken,
			// resolve: given chat_id → portfolio + executor
			func(chatID int64) (string, *portfolio.Portfolio, bot.Executor, bool) {
				nexusID, inst := mgr.GetByChatID(strconv.FormatInt(chatID, 10))
				if inst == nil {
					return "", nil, nil, false
				}
				return nexusID, inst.Port, inst.Trader, true
			},
			// onLink: save chat_id to DB + restart instance with Telegram notifier
			func(nexusID string, chatID int64) {
				chatStr := strconv.FormatInt(chatID, 10)
				database.SetTelegramChatID(nexusID, chatStr)
				apiKey, apiSecret, err := database.GetSignetCredentials(nexusID)
				if err != nil {
					return
				}
				user, _ := database.GetUser(nexusID)
				walletID := ""
				if user != nil && user.MainWalletID != "" {
					walletID = user.MainWalletID
				}
				userCfg, _ := database.GetUserConfig(nexusID)
				mgr.Stop(nexusID)
				if walletID != "" {
					mgr.StartWithWallet(nexusID, apiKey, apiSecret, walletID, chatStr, userCfg)
				} else {
					mgr.Start(nexusID, apiKey, apiSecret, chatStr, userCfg)
				}
				log.Printf("[bot] linked chat %s to user %s", chatStr, nexusID[:8])
			},
			// onStop: remove instance from manager
			func(nexusID string) {
				mgr.Stop(nexusID)
			},
			// onGetConfig: load per-user config from DB as BotConfig
			func(nexusID string) bot.BotConfig {
				return dbCfgToBotCfg(database, nexusID)
			},
			// onSetConfig: save BotConfig back to DB
			func(nexusID string, bcfg bot.BotConfig) {
				uc := botCfgToDBCfg(bcfg)
				if err := database.SetUserConfig(nexusID, uc); err != nil {
					log.Printf("[bot] setConfig failed for %s: %v", nexusID[:8], err)
				}
				// Restart user instance to pick up new settings
				apiKey, apiSecret, err := database.GetSignetCredentials(nexusID)
				if err != nil {
					return
				}
				user, _ := database.GetUser(nexusID)
				chatID := ""
				if user != nil {
					chatID = user.TelegramChatID
				}
				walletID := ""
				if user != nil && user.MainWalletID != "" {
					walletID = user.MainWalletID
				}
				mgr.Stop(nexusID)
				if walletID != "" {
					mgr.StartWithWallet(nexusID, apiKey, apiSecret, walletID, chatID, uc)
				} else {
					mgr.Start(nexusID, apiKey, apiSecret, chatID, uc)
				}
			},
		)
		if err != nil {
			log.Printf("[main] telegram bot init failed: %v", err)
			tgBot = nil
		} else {
			go tgBot.Run()
		}
	}

	// Resume bots for all configured users on startup
	go func() {
		users, err := database.AllConfiguredUsersData()
		if err != nil {
			log.Printf("[main] failed to load users: %v", err)
			return
		}
		for _, u := range users {
			apiKey, apiSecret, err := database.GetSignetCredentials(u.NexusUserID)
			if err != nil {
				log.Printf("[main] skip resume for user %s: %v", u.NexusUserID[:8], err)
				continue
			}
			userCfg, _ := database.GetUserConfig(u.NexusUserID)
			if u.MainWalletID != "" {
				err = mgr.StartWithWallet(u.NexusUserID, apiKey, apiSecret, u.MainWalletID, u.TelegramChatID, userCfg)
			} else {
				err = mgr.Start(u.NexusUserID, apiKey, apiSecret, u.TelegramChatID, userCfg)
			}
			if err != nil {
				log.Printf("[main] resume failed for user %s: %v", u.NexusUserID[:8], err)
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
			"id":                 user.NexusUserID,
			"username":           user.Username,
			"first_name":         user.FirstName,
			"last_name":          user.LastName,
			"email":              user.Email,
			"avatar":             user.Avatar,
			"has_signet":         user.HasSignet,
			"signet_key_prefix":  user.SignetKeyPrefix,
			"wallet_id":          user.WalletID,
			"main_wallet_id":     user.MainWalletID,
			"telegram_chat_id":   user.TelegramChatID,
			"bot_active":         inst != nil,
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
			user, _ := database.GetUser(nexusID)
			chatID := ""
			if user != nil {
				chatID = user.TelegramChatID
			}
			userCfg, _ := database.GetUserConfig(nexusID)
			mgr.Start(nexusID, req.APIKey, req.APISecret, chatID, userCfg)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok","bot_active":true}`)
	})

	// POST /score — receives TokenDetected from the Rust listener.
	// Scores via Cricket and fans out to all active user traders.
	// This replaces the Python scorer service entirely.
	mux.HandleFunc("POST /score", func(w http.ResponseWriter, r *http.Request) {
		var token cricket.TokenDetected
		if err := json.NewDecoder(r.Body).Decode(&token); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		go scoreAndTrade(cc, func(result *models.ScoreResult) {
			for _, inst := range mgr.All() {
				inst.Trader.Execute(result)
			}
		}, cfg.TelegramToken, cfg.TelegramChannelID, token)
		fmt.Fprintf(w, `{"status":"queued","mint":"%s"}`, token.Mint)
	})

	// POST /trade — receives score results from Python scorer, fans out to traders.
	mux.HandleFunc("POST /trade", func(w http.ResponseWriter, r *http.Request) {
		var result models.ScoreResult
		if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if cfg.TelegramChannelID != "" && cfg.TelegramToken != "" {
			go broadcastTradeResult(cfg.TelegramToken, cfg.TelegramChannelID, &result)
		}
		instances := mgr.All()
		for _, inst := range instances {
			inst.Trader.Execute(&result)
		}
		fmt.Fprintf(w, `{"status":"queued","instances":%d}`, len(instances))
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
			WalletBalanceSOL float64 `json:"wallet_balance_sol"`
			Configured       bool    `json:"configured"`
		}
		w.Header().Set("Content-Type", "application/json")
		if inst == nil {
			user, _ := database.GetUser(nexusID)
			json.NewEncoder(w).Encode(statsResp{portfolio.Stats{}, 0, user != nil && user.HasSignet})
			return
		}
		json.NewEncoder(w).Encode(statsResp{inst.Port.Stats(), inst.Trader.Balance(), true})
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
		w.Header().Set("Content-Type", "application/json")
		closed, err := database.ClosedPositionsByUser(nexusID, 50)
		if err != nil {
			log.Printf("[closed] db query failed for user %s: %v", nexusID[:8], err)
			json.NewEncoder(w).Encode([]*models.ClosedPosition{})
			return
		}
		if closed == nil {
			closed = []*models.ClosedPosition{}
		}
		json.NewEncoder(w).Encode(closed)
	})

	// GET /card/{mint} — generate a PnL share card PNG for a closed position.
	// Returns image/png. Requires auth. Only works for profitable trades.
	mux.HandleFunc("GET /card/{mint}", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		mint := r.PathValue("mint")
		if mint == "" {
			http.Error(w, "missing mint", http.StatusBadRequest)
			return
		}
		// Look up from DB (most reliable, covers past sessions)
		closed, dbErr := database.ClosedPositionsByUser(nexusID, 50)
		if dbErr != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		var target *models.ClosedPosition
		for _, c := range closed {
			if c.Mint == mint {
				target = c
				break
			}
		}
		// Fallback to in-memory portfolio
		if target == nil {
			inst := mgr.Get(nexusID)
			if inst != nil {
				target, _ = inst.Port.GetClosedByMint(mint)
			}
		}
		if target == nil {
			http.Error(w, "position not found", http.StatusNotFound)
			return
		}
		pngBytes, err := pnl.GenerateCard(target)
		if err != nil {
			log.Printf("[card] generate failed for %s: %v", mint[:8], err)
			http.Error(w, "card generation failed — wkhtmltoimage may not be installed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Disposition", `attachment; filename="hb-trade.png"`)
		w.Write(pngBytes)
	})

	mux.HandleFunc("GET /logs", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		inst := mgr.Get(nexusID)
		if inst == nil {
			json.NewEncoder(w).Encode([]eventlog.Event{})
			return
		}
		json.NewEncoder(w).Encode(inst.Log.All())
	})

	mux.HandleFunc("GET /logs/export", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		inst := mgr.Get(nexusID)
		var events []eventlog.Event
		if inst != nil {
			events = inst.Log.All()
		}
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="hummingbird-logs.csv"`)
		fmt.Fprintln(w, "time,type,token,amount_sol,pnl_sol,pnl_pct,reason,message")
		for _, e := range events {
			fmt.Fprintf(w, "%s,%s,%s,%.6f,%.6f,%.2f,%s,%q\n",
				e.Time.Format(time.RFC3339), e.Type, e.Token, e.AmtSOL, e.PnLSOL, e.PnLPct, e.Reason, e.Message)
		}
	})

	mux.HandleFunc("GET /config", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		userCfg, _ := database.GetUserConfig(nexusID)
		if userCfg == nil {
			userCfg = db.DefaultUserConfig()
		}
		inst := mgr.Get(nexusID)
		walletID := ""
		if inst != nil {
			walletID = inst.WalletID
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"wallet_id":        walletID,
			"sniper_enabled":   userCfg.SniperEnabled,
			"scalper_enabled":  userCfg.ScalperEnabled,
			"max_position_sol": userCfg.MaxPositionSOL,
			"max_positions":    userCfg.MaxPositions,
			"stop_loss_pct":    userCfg.StopLossPercent,
			"daily_loss_limit": userCfg.DailyLossLimit,
			"take_profit_1x":   userCfg.TakeProfit1x,
			"take_profit_2x":   userCfg.TakeProfit2x,
			"take_profit_3x":   userCfg.TakeProfit3x,
			"timeout_minutes":  userCfg.TimeoutMinutes,
			"min_balance_sol":  userCfg.MinBalanceSOL,
		})
	})

	mux.HandleFunc("PUT /config", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		var req db.UserConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
			return
		}
		// Sanitise bounds
		if req.MaxPositionSOL < 0.01 { req.MaxPositionSOL = 0.01 }
		if req.MaxPositionSOL > 5.0  { req.MaxPositionSOL = 5.0 }
		if req.MaxPositions   < 1    { req.MaxPositions = 1 }
		if req.MaxPositions   > 20   { req.MaxPositions = 20 }
		if req.StopLossPercent < 0.05 { req.StopLossPercent = 0.05 }
		if req.StopLossPercent > 0.90 { req.StopLossPercent = 0.90 }
		if req.DailyLossLimit  < 0.05 { req.DailyLossLimit = 0.05 }
		if req.DailyLossLimit  > 0.90 { req.DailyLossLimit = 0.90 }
		if req.TakeProfit1x < 1.2   { req.TakeProfit1x = 1.2 }
		if req.TakeProfit2x < 1.5   { req.TakeProfit2x = 1.5 }
		if req.TakeProfit3x < 2.0   { req.TakeProfit3x = 2.0 }
		if req.TimeoutMinutes < 1   { req.TimeoutMinutes = 1 }
		if req.TimeoutMinutes > 60  { req.TimeoutMinutes = 60 }
		if err := database.SetUserConfig(nexusID, &req); err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		// Restart bot instance with new config
		apiKey, apiSecret, cErr := database.GetSignetCredentials(nexusID)
		if cErr == nil {
			user, _ := database.GetUser(nexusID)
			chatID := ""; walletIDr := ""
			if user != nil { chatID = user.TelegramChatID; walletIDr = user.MainWalletID }
			mgr.Stop(nexusID)
			if walletIDr != "" {
				mgr.StartWithWallet(nexusID, apiKey, apiSecret, walletIDr, chatID, &req)
			} else {
				mgr.Start(nexusID, apiKey, apiSecret, chatID, &req)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"saved"}`)
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
				user, _ := database.GetUser(nexusID)
				chatID := ""
				if user != nil {
					chatID = user.TelegramChatID
				}
				userCfg, _ := database.GetUserConfig(nexusID)
				mgr.Start(nexusID, apiKey, apiSecret, chatID, userCfg)
			}
		} else {
			inst.Port.Resume()
		}
		fmt.Fprint(w, `{"status":"resumed"}`)
	})

	// DELETE /auth/signet — remove stored Signet credentials and stop bot
	mux.HandleFunc("DELETE /auth/signet", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		mgr.Stop(nexusID)
		if err := database.ClearSignetCredentials(nexusID); err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, `{"status":"removed"}`)
	})

	// POST /auth/telegram/token — generate a deep-link token for Telegram account linking
	mux.HandleFunc("POST /auth/telegram/token", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if tgBot == nil {
			http.Error(w, `{"error":"telegram not configured"}`, http.StatusServiceUnavailable)
			return
		}
		token := tgBot.GenerateLinkToken(nexusID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"token":        token,
			"bot_username": tgBot.Username(),
		})
	})

	// POST /auth/cli-token — mint a 7-day token for CLI use
	mux.HandleFunc("POST /auth/cli-token", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		tok, err := auth.IssueCLIToken(nexusID, cfg.JWTSecret)
		if err != nil {
			http.Error(w, `{"error":"could not issue token"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"token": tok})
	})

	// POST /wallets/:id/set-main — mark a wallet as the trading wallet
	mux.HandleFunc("POST /wallets/{id}/set-main", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		walletID := r.PathValue("id")
		// Restart bot with the new wallet
		apiKey, apiSecret, err := database.GetSignetCredentials(nexusID)
		if err == nil {
			user, _ := database.GetUser(nexusID)
			chatID := ""
			if user != nil {
				chatID = user.TelegramChatID
			}
			userCfg, _ := database.GetUserConfig(nexusID)
			mgr.Stop(nexusID)
			database.SetMainWallet(nexusID, walletID)
			mgr.StartWithWallet(nexusID, apiKey, apiSecret, walletID, chatID, userCfg)
		} else {
			database.SetMainWallet(nexusID, walletID)
		}
		fmt.Fprint(w, `{"status":"ok"}`)
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
			bal := fetchSOLBalance(cfg.SolanaRPC, wal.Address)
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

	// POST /wallets/:id/withdraw — transfer SOL from a wallet
	mux.HandleFunc("POST /wallets/{id}/withdraw", func(w http.ResponseWriter, r *http.Request) {
		nexusID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		walletID := r.PathValue("id")
		var req struct {
			To     string `json:"to"`
			Amount string `json:"amount"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.To == "" || req.Amount == "" {
			http.Error(w, `{"error":"to and amount required"}`, http.StatusBadRequest)
			return
		}
		apiKey, apiSecret, err := database.GetSignetCredentials(nexusID)
		if err != nil {
			http.Error(w, `{"error":"no signet credentials"}`, http.StatusBadRequest)
			return
		}
		client := signet.NewClient(apiKey, apiSecret).WithBaseURL(cfg.SignetBaseURL)
		result, err := client.Wallets.Transfer(walletID, signet.TransferParams{
			To:     req.To,
			Amount: req.Amount,
		})
		if err != nil {
			log.Printf("[wallets] transfer failed: %v", err)
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
			return
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

// fetchSOLBalance fetches a wallet's SOL balance directly from Helius RPC.
// Uses getBalance with the wallet's Solana public address — no Signet request needed.
func fetchSOLBalance(rpcURL, address string) float64 {
	if rpcURL == "" || address == "" {
		return 0
	}
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getBalance",
		"params":  []any{address, map[string]string{"commitment": "confirmed"}},
	})
	resp, err := http.Post(rpcURL, "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		return 0
	}
	defer resp.Body.Close()
	var result struct {
		Result struct {
			Value int64 `json:"value"` // lamports
		} `json:"result"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return float64(result.Result.Value) / 1e9 // lamports → SOL
}

// ── Cricket scoring ───────────────────────────────────────────────────────────

// cricketSem limits concurrent Cricket API calls to avoid hammering the endpoint.
// pump.fun fires ~10-20 tokens/min; each scan = 2 Cricket calls. Cap at 3 concurrent scans = 6 max in-flight.
var cricketSem = make(chan struct{}, 3)

// scoreAndTrade runs a Cricket risk analysis on a freshly-detected token and,
// if the score passes, calls dispatch to send it to the active trader(s).
// Runs in a goroutine — the listener is never blocked.
func scoreAndTrade(cc *cricket.Client, dispatch func(*models.ScoreResult), tgToken, channelID string, token cricket.TokenDetected) {
	cricketSem <- struct{}{}
	defer func() { <-cricketSem }()

	// Wait for the launch tx to confirm and propagate to Helius before scanning.
	// Scanning immediately (~ms after detection) causes "account not found" on Cricket's end.
	time.Sleep(3 * time.Second)

	// Basic sanity check: pump.fun mints are 44 chars; shorter = likely wrong account extracted
	if len(token.Mint) < 32 {
		log.Printf("[scorer] skipping %s — mint address too short, likely wrong account", safeShort(token.Mint))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Run mantis scan + firefly wallet check concurrently
	type scanRes struct {
		data *cricket.MantisScanResponse
		err  error
	}
	type walletRes struct {
		data *cricket.FireflyWalletResponse
		err  error
	}

	scanCh := make(chan scanRes, 1)
	walletCh := make(chan walletRes, 1)

	go func() {
		d, err := cc.MantisScan(ctx, token.Mint, token.DevWallet, token.BondingCurve)
		scanCh <- scanRes{d, err}
	}()
	go func() {
		d, err := cc.FireflyWallet(ctx, token.DevWallet)
		walletCh <- walletRes{d, err}
	}()

	sr := <-scanCh
	wr := <-walletCh

	if sr.err != nil {
		if errors.Is(sr.err, cricket.ErrTokenNotFound) {
			log.Printf("[scorer] skip %s — not on-chain yet or wrong address", safeShort(token.Mint))
		} else {
			log.Printf("[scorer] mantis scan failed for %s: %v", safeShort(token.Mint), sr.err)
		}
		return
	}

	score, decision, posSOL := scoreFromCricket(sr.data, wr.data)

	entered := decision != "skip"
	sym := map[bool]string{true: "✅", false: "⏭ "}[entered]
	log.Printf("[scorer] %s %s…  score=%d  decision=%s  pos=%.2f SOL  (rating=%s)",
		sym, safeShort(token.Mint), score, decision, posSOL, sr.data.Data.RiskScore.Rating)

	// Broadcast to public channel for every scanned token
	if channelID != "" && tgToken != "" {
		go broadcastScan(tgToken, channelID, token, sr.data, wr.data, score, decision, posSOL)
	}

	if !entered {
		return
	}

	dispatch(&models.ScoreResult{
		Mint:        token.Mint,
		DevWallet:   token.DevWallet,
		Platform:    token.Platform,
		Total:       score,
		Decision:    decision,
		PositionSOL: posSOL,
		Checks: map[string]models.CheckResult{
			"mantis": {
				Score:    sr.data.Data.RiskScore.Score,
				MaxScore: 100,
				Reason:   sr.data.Data.RiskScore.Rating,
			},
		},
		ScoredAtMs: time.Now().UnixMilli(),
	})
}

// scoreFromCricket maps Cricket's risk data to a Hummingbird score and position size.
//
// Cricket mantis rating → rug risk (high = bad). We invert for Hummingbird (high = good).
//   critical / high → skip entirely
//   moderate        → small position (0.05 SOL)
//   low             → larger position, adjusted by dev wallet profile
//
// Dev wallet adjustments (Firefly):
//   smart_contract_deployer with >75% win rate → seasoned rugger, reduce score
//   very low firefly score (<20)               → bad actor, reduce score
func scoreFromCricket(scan *cricket.MantisScanResponse, devWallet *cricket.FireflyWalletResponse) (score int, decision string, posSOL float64) {
	r := scan.Data.RiskScore
	s := scan.Data.Scan

	// Skip if Cricket hit mock data — score is meaningless
	if scan.Data.Confidence != "high" {
		return 0, "skip", 0
	}

	// Hard skips — these are unrecoverable regardless of other signals
	if r.Rating == "critical" {
		return 0, "skip", 0
	}

	// Low liquidity hard skip — bonding curve has < 1 SOL, can't trade profitably or exit cleanly.
	// Cricket sets this flag when the curve is active but holds less than 1 SOL.
	// The EyDQ7Tyf rug would have been blocked by this.
	for _, f := range s.Flags {
		if f.Check == "low_liquidity" && (f.Severity == "high" || f.Severity == "critical") {
			return 0, "skip", 0
		}
	}
	// Belt + suspenders: also check raw reserve value if Cricket returns it
	if s.BondingCurveSolReserve != nil && *s.BondingCurveSolReserve < 0.5 {
		return 0, "skip", 0
	}

	// Start from Cricket's numeric score as the base
	score = r.Score

	// ── Security flags ────────────────────────────────────────────────────────
	if !s.MintAuthorityRevoked {
		score -= 20 // dev can print unlimited tokens — serious but not auto-skip on pump.fun
	}
	if !s.FreezeAuthorityRevoked {
		score -= 10 // dev can freeze wallets
	}
	if s.MetadataMutable {
		score -= 5 // name/image can be changed post-launch
	}

	// ── Liquidity ─────────────────────────────────────────────────────────────
	// Pre-graduation bonding curve tokens have no LP pool yet — LP lock is meaningless here.
	// Only apply LP checks for graduated tokens (Raydium / post-bonding).
	bondingActive := s.BondingCurveComplete == nil || !*s.BondingCurveComplete
	if !bondingActive {
		if s.LPLocked {
			score += 10
			if s.LPLockDurationDays != nil && *s.LPLockDurationDays >= 30 {
				score += 5 // meaningful lock duration
			}
		} else {
			score -= 10 // graduated but LP not locked — liquidity can be pulled
		}
	}

	// ── Bonding curve timing (pump.fun sweet spot) ────────────────────────────
	if s.BondingCurveFillPct != nil {
		pct := *s.BondingCurveFillPct
		switch {
		case pct >= 5 && pct <= 25:
			score += 10 // sweet spot — enough interest, not too late
		case pct > 25 && pct <= 60:
			score += 5 // still reasonable
		case pct > 80:
			score -= 10 // near graduation — pump already happened
		}
	}
	if s.BondingCurveComplete != nil && *s.BondingCurveComplete {
		score -= 15 // graduated to Raydium — different dynamics, less pump.fun edge
	}

	// ── Holder distribution ───────────────────────────────────────────────────
	if s.Top10HolderPct > 80 {
		score -= 20 // extreme whale concentration
	} else if s.Top10HolderPct > 60 {
		score -= 10
	} else if s.Top10HolderPct > 0 && s.Top10HolderPct < 30 {
		score += 5 // well distributed
	}

	if s.DevSupplyPct != nil {
		devPct := *s.DevSupplyPct
		switch {
		case devPct > 50:
			score -= 25
		case devPct > 30:
			score -= 15
		case devPct > 15:
			score -= 5
		case devPct < 5 && devPct > 0:
			score += 5 // fair distribution
		}
	}

	// ── Deployer history ──────────────────────────────────────────────────────
	// Most pump.fun tokens are launched by fresh wallets — a 0-day wallet is extremely common
	// and doesn't reliably predict a rug. Keep the penalty mild; serial launchers are the real flag.
	if s.DeployerAgeKnown {
		switch {
		case s.DeployerWalletAgeDays == 0:
			score -= 5 // new wallet: mild penalty, very common on pump.fun
		case s.DeployerWalletAgeDays < 7:
			score -= 3
		case s.DeployerWalletAgeDays > 90:
			score += 8 // seasoned wallet — more accountability
		case s.DeployerWalletAgeDays > 30:
			score += 4
		}
	}
	if s.DeployerPriorLaunches != nil {
		launches := *s.DeployerPriorLaunches
		if launches > 10 {
			score -= 10 // serial launcher — likely farming
		} else if launches > 3 {
			score -= 5
		}
	}

	// ── Firefly smart-money signals ───────────────────────────────────────────
	if devWallet != nil && devWallet.Success {
		dw := devWallet.Data
		switch {
		case dw.Score >= 70:
			score += 8 // high smart-money score = good actor
		case dw.Score >= 50:
			score += 3
		case dw.Score < 10:
			score -= 25 // flagged bad actor
		case dw.Score < 20:
			score -= 15
		}
		if dw.Style == "smart_contract_deployer" && dw.WinRate > 75 {
			score -= 20 // professional serial deployer with high win rate = likely rugger
		}
		if dw.AvgReturnPct > 100 && dw.TotalTrades > 5 {
			score += 5 // genuinely profitable deployer history
		}
	}

	// Clamp to 0-100
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}

	// Entry thresholds — calibrated for pump.fun token score distribution (typically 20-65).
	// Pre-graduation tokens can't lock LP so scores are structurally lower than post-launch.
	switch {
	case score < 40:
		return score, "skip", 0
	case score < 55:
		return score, "small", 0.05
	case score < 70:
		return score, "medium", 0.10
	default:
		return score, "full", 0.20
	}
}

// channelRateLimit throttles channel broadcasts to Telegram's ~20/min limit.
var (
	channelLastSent time.Time
	channelMu       sync.Mutex
)

const channelMinInterval = 4 * time.Second // safe under Telegram's 20/min cap

func channelAllowed() bool {
	channelMu.Lock()
	defer channelMu.Unlock()
	if time.Since(channelLastSent) < channelMinInterval {
		return false
	}
	channelLastSent = time.Now()
	return true
}

// broadcastTradeResult posts a styled scan card to the channel from a Python scorer ScoreResult.
// Used when the Rust listener → Python scorer → orchestrator path is active.
func broadcastTradeResult(tgToken, channelID string, result *models.ScoreResult) {
	// Always send entries; throttle skips to avoid Telegram rate limits (~20/min)
	if result.Decision == "skip" && !channelAllowed() {
		return
	}
	mintShort := result.Mint
	if len(mintShort) > 12 {
		mintShort = result.Mint[:8] + "…" + result.Mint[len(result.Mint)-4:]
	}

	// Build check lines from the checks map
	// cleanReason strips raw error messages from check reasons (e.g. Helius 429 errors)
	cleanReason := func(r string) string {
		if strings.Contains(r, "RPC error") || strings.Contains(r, "Too Many Requests") || strings.Contains(r, "http") {
			return "RPC unavailable"
		}
		return r
	}

	checkOrder := []string{"dev_wallet", "supply", "bonding", "contract", "social"}
	checkEmoji := func(score, max int) string {
		if max == 0 {
			return "⚪"
		}
		pct := float64(score) / float64(max)
		switch {
		case pct >= 0.75:
			return "🟢"
		case pct >= 0.4:
			return "🟡"
		default:
			return "🔴"
		}
	}
	checkLabel := map[string]string{
		"dev_wallet": "Dev Wallet",
		"supply":     "Supply",
		"bonding":    "Bonding",
		"contract":   "Contract",
		"social":     "Social",
	}

	var checkLines []string
	for _, key := range checkOrder {
		c, ok := result.Checks[key]
		if !ok {
			continue
		}
		em := checkEmoji(c.Score, c.MaxScore)
		label := checkLabel[key]
		if label == "" {
			label = key
		}
		line := fmt.Sprintf("%s %s %d/%d", em, label, c.Score, c.MaxScore)
		if r := cleanReason(c.Reason); r != "" {
			line += "  " + r
		}
		checkLines = append(checkLines, line)
	}

	var header string
	if result.Decision == "skip" {
		header = fmt.Sprintf("⏭ *Scanned — Skip*   Score: %d/100\n%s", result.Total, mintShort)
	} else {
		label := strings.ToUpper(result.Decision)
		header = fmt.Sprintf("🐦 *Sniped — %s*   Score: %d/100\n%.3f SOL  ·  %s", label, result.Total, result.PositionSOL, mintShort)
	}

	parts := []string{header}
	if len(checkLines) > 0 {
		parts = append(parts, strings.Join(checkLines, "\n"))
	}
	parts = append(parts, "\n⚡ [hummingbird.vylth.com](https://hummingbird.vylth.com)")

	msg := strings.Join(parts, "\n")

	body, _ := json.Marshal(map[string]any{
		"chat_id":                  channelID,
		"text":                     msg,
		"parse_mode":               "Markdown",
		"disable_web_page_preview": true,
	})
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", tgToken)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[scanner] channel broadcast failed: %v", err)
		return
	}
	resp.Body.Close()
}

// broadcastScan posts a styled scan summary to the public Telegram channel.
// Called for every token — skipped ones show risk flags, entered ones celebrate the snipe.
func broadcastScan(tgToken, channelID string, token cricket.TokenDetected, scan *cricket.MantisScanResponse, wallet *cricket.FireflyWalletResponse, score int, decision string, posSOL float64) {
	if decision == "skip" && !channelAllowed() {
		return
	}
	s := scan.Data.Scan
	r := scan.Data.RiskScore

	// Risk rating emoji
	ratingEmoji := map[string]string{
		"low":      "🟢",
		"moderate": "🟡",
		"high":     "🔴",
		"critical": "🔴",
	}
	rEmoji := ratingEmoji[r.Rating]
	if rEmoji == "" {
		rEmoji = "⚪"
	}

	// Mint display: first 8 + last 4
	mintShort := token.Mint
	if len(mintShort) > 12 {
		mintShort = token.Mint[:8] + "…" + token.Mint[len(token.Mint)-4:]
	}

	// Platform label
	platform := strings.ToUpper(strings.ReplaceAll(token.Platform, "_", "."))

	var header string
	if decision == "skip" {
		header = fmt.Sprintf("⏭ *Scanned — Skip*   %s %s (%d/100)\n%s  ·  %s", rEmoji, strings.ToUpper(r.Rating), r.Score, platform, mintShort)
	} else {
		posLabel := strings.ToUpper(decision)
		header = fmt.Sprintf("🐦 *Sniped — %s*   %s %s (%d/100)\n%s  ·  %.3f SOL  ·  %s", posLabel, rEmoji, strings.ToUpper(r.Rating), r.Score, platform, posSOL, mintShort)
	}

	// Security line
	mintOk := "✅ Mint revoked"
	if !s.MintAuthorityRevoked {
		mintOk = "⚠️ Mint active"
	}
	freezeOk := "✅ Freeze revoked"
	if !s.FreezeAuthorityRevoked {
		freezeOk = "⚠️ Freeze active"
	}
	security := mintOk + "   " + freezeOk

	// Supply line
	var supplyParts []string
	if s.DevSupplyPct != nil {
		supplyParts = append(supplyParts, fmt.Sprintf("Dev: %.1f%%", *s.DevSupplyPct))
	}
	if s.Top10HolderPct > 0 {
		supplyParts = append(supplyParts, fmt.Sprintf("Top 10: %.1f%%", s.Top10HolderPct))
	}
	if s.BondingCurveFillPct != nil {
		supplyParts = append(supplyParts, fmt.Sprintf("Bonding: %.1f%%", *s.BondingCurveFillPct))
	}
	supply := "📊 " + strings.Join(supplyParts, "   ")
	if len(supplyParts) == 0 {
		supply = ""
	}

	// Dev wallet line
	devLine := fmt.Sprintf("👤 Dev: %dd old", s.DeployerWalletAgeDays)
	if s.DeployerPriorLaunches != nil && *s.DeployerPriorLaunches > 0 {
		devLine += fmt.Sprintf("   %d prior launches", *s.DeployerPriorLaunches)
	}
	if wallet != nil && wallet.Success {
		dw := wallet.Data
		devLine += fmt.Sprintf("   Firefly: %d/100", dw.Score)
		if dw.TotalTrades > 0 {
			devLine += fmt.Sprintf(" (%.0f%% win)", dw.WinRate)
		}
	}

	// Flags
	var flagLines []string
	for _, f := range s.Flags {
		if f.Severity == "high" || f.Severity == "critical" {
			flagLines = append(flagLines, "🚩 "+f.Detail)
		}
	}

	parts := []string{header, security}
	if supply != "" {
		parts = append(parts, supply)
	}
	parts = append(parts, devLine)
	if len(flagLines) > 0 {
		parts = append(parts, strings.Join(flagLines, "\n"))
	}
	parts = append(parts, fmt.Sprintf("\n⚡ [hummingbird.vylth.com](https://hummingbird.vylth.com)"))

	msg := strings.Join(parts, "\n")

	body, _ := json.Marshal(map[string]any{
		"chat_id":                  channelID,
		"text":                     msg,
		"parse_mode":               "Markdown",
		"disable_web_page_preview": true,
	})
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", tgToken)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[scanner] channel broadcast failed: %v", err)
		return
	}
	resp.Body.Close()
}

// ── Config conversion helpers ─────────────────────────────────────────────────

func dbCfgToBotCfg(database *db.DB, nexusID string) bot.BotConfig {
	uc, _ := database.GetUserConfig(nexusID)
	if uc == nil {
		uc = db.DefaultUserConfig()
	}
	return bot.BotConfig{
		SniperEnabled:   uc.SniperEnabled,
		ScalperEnabled:  uc.ScalperEnabled,
		MaxPositionSOL:  uc.MaxPositionSOL,
		MaxPositions:    uc.MaxPositions,
		StopLossPercent: uc.StopLossPercent,
		DailyLossLimit:  uc.DailyLossLimit,
		TakeProfit1x:    uc.TakeProfit1x,
		TakeProfit2x:    uc.TakeProfit2x,
		TakeProfit3x:    uc.TakeProfit3x,
		TimeoutMinutes:  uc.TimeoutMinutes,
		MinBalanceSOL:   uc.MinBalanceSOL,
	}
}

func botCfgToDBCfg(bcfg bot.BotConfig) *db.UserConfig {
	return &db.UserConfig{
		SniperEnabled:   bcfg.SniperEnabled,
		ScalperEnabled:  bcfg.ScalperEnabled,
		MaxPositionSOL:  bcfg.MaxPositionSOL,
		MaxPositions:    bcfg.MaxPositions,
		StopLossPercent: bcfg.StopLossPercent,
		DailyLossLimit:  bcfg.DailyLossLimit,
		TakeProfit1x:    bcfg.TakeProfit1x,
		TakeProfit2x:    bcfg.TakeProfit2x,
		TakeProfit3x:    bcfg.TakeProfit3x,
		TimeoutMinutes:  bcfg.TimeoutMinutes,
		MinBalanceSOL:   bcfg.MinBalanceSOL,
	}
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

// ── Rate limiting ─────────────────────────────────────────────────────────────

type rateBucket struct {
	mu          sync.Mutex
	count       int
	windowStart time.Time
}

func (b *rateBucket) allow(limit int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if now.Sub(b.windowStart) >= time.Minute {
		b.count = 0
		b.windowStart = now
	}
	if b.count >= limit {
		return false
	}
	b.count++
	return true
}

// withRateLimit wraps the handler with per-user (300/min) and score-endpoint (200/min) rate limiting.
// 300/min = 5/sec average — enough for dashboard polling 3 endpoints every 3s with room to spare.
func withRateLimit(next http.Handler, jwtSecret string) http.Handler {
	var userBuckets sync.Map
	var scoreBuckets sync.Map
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Public routes — no limit
		if path == "/health" || path == "/mode" || strings.HasPrefix(path, "/auth/") {
			next.ServeHTTP(w, r)
			return
		}
		// Score/trade ingest — IP-based 100/min
		if path == "/score" || path == "/trade" {
			ip := r.RemoteAddr
			if idx := strings.LastIndex(ip, ":"); idx >= 0 {
				ip = ip[:idx]
			}
			v, _ := scoreBuckets.LoadOrStore(ip, &rateBucket{windowStart: time.Now()})
			if !v.(*rateBucket).allow(200) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				log.Printf("[ratelimit] WARN score endpoint throttled for %s", ip)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		// All other routes — JWT-based 60/min per user
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			if nexusID, err := auth.ParseToken(strings.TrimPrefix(h, "Bearer "), jwtSecret); err == nil {
				v, _ := userBuckets.LoadOrStore(nexusID, &rateBucket{windowStart: time.Now()})
				if !v.(*rateBucket).allow(300) {
					w.Header().Set("Retry-After", "60")
					http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
					log.Printf("[ratelimit] WARN user %s throttled", safeShort(nexusID))
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
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
func (noopNotifier) Alert(text string)  { log.Printf("[notify] %s", text) }
func (noopNotifier) Notify(text string) { log.Printf("[notify] %s", text) }
