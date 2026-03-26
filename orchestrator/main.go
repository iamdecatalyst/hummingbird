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
// Identity = Signet wallet ID. The API key+secret IS the login — no passwords.
// Flow: user enters key+secret → we verify against Signet → wallet ID = account → JWT issued.

func startMultiTenant(cfg *config.Config, mux *http.ServeMux) {
	database, err := db.New(cfg.DatabaseURL, cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("[main] db init failed: %v", err)
	}
	log.Printf("[main] Postgres connected, multi-tenant mode active")

	mgr := userbot.NewManager(cfg)

	// Resume bots for all existing users on startup
	go func() {
		ids, err := database.AllWalletIDs()
		if err != nil {
			log.Printf("[main] failed to load users: %v", err)
			return
		}
		for _, wid := range ids {
			apiKey, apiSecret, err := database.GetCredentials(wid)
			if err != nil {
				log.Printf("[main] skip resume for %s: %v", wid[:8], err)
				continue
			}
			if err := mgr.Start(wid, apiKey, apiSecret); err != nil {
				log.Printf("[main] resume failed for %s: %v", wid[:8], err)
			}
		}
	}()

	// requireAuth extracts and validates the JWT, returns wallet ID
	requireAuth := func(r *http.Request) (string, error) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return "", fmt.Errorf("missing token")
		}
		return auth.ParseToken(strings.TrimPrefix(h, "Bearer "), cfg.JWTSecret)
	}

	// POST /auth/signin — Signet key+secret IS the login. No passwords.
	// Verifies credentials against Signet, creates/updates account, returns JWT.
	mux.HandleFunc("POST /auth/signin", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			APIKey    string `json:"api_key"`
			APISecret string `json:"api_secret"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.APIKey == "" || req.APISecret == "" {
			http.Error(w, `{"error":"api_key and api_secret required"}`, http.StatusBadRequest)
			return
		}

		// Verify credentials by actually connecting to Signet
		walletName := "hummingbird-trader"
		client := signet.NewClient(req.APIKey, req.APISecret).WithBaseURL(cfg.SignetBaseURL)
		walletID, err := trader.EnsureWallet(client, walletName)
		if err != nil {
			http.Error(w, `{"error":"invalid Signet credentials"}`, http.StatusUnauthorized)
			return
		}

		// Save encrypted credentials (upsert — same wallet ID = same account)
		if err := database.Upsert(walletID, req.APIKey, req.APISecret); err != nil {
			log.Printf("[signin] db upsert failed for %s: %v", walletID[:8], err)
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}

		// Start the bot for this user
		if mgr.Get(walletID) == nil {
			if err := mgr.Start(walletID, req.APIKey, req.APISecret); err != nil {
				log.Printf("[signin] bot start failed for %s: %v", walletID[:8], err)
			}
		}

		token, _ := auth.IssueToken(walletID, cfg.JWTSecret)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"token":     token,
			"wallet_id": walletID,
		})
	})

	// GET /auth/me — who am I?
	mux.HandleFunc("GET /auth/me", func(w http.ResponseWriter, r *http.Request) {
		walletID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		inst := mgr.Get(walletID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"wallet_id":  walletID,
			"bot_active": inst != nil,
		})
	})

	// Per-user trading endpoints

	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		walletID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		inst := mgr.Get(walletID)
		type statsResp struct {
			portfolio.Stats
			Configured bool `json:"configured"`
		}
		w.Header().Set("Content-Type", "application/json")
		if inst == nil {
			json.NewEncoder(w).Encode(statsResp{portfolio.Stats{}, false})
			return
		}
		json.NewEncoder(w).Encode(statsResp{inst.Port.Stats(), true})
	})

	mux.HandleFunc("GET /positions", func(w http.ResponseWriter, r *http.Request) {
		walletID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		inst := mgr.Get(walletID)
		w.Header().Set("Content-Type", "application/json")
		if inst == nil {
			json.NewEncoder(w).Encode([]*models.Position{})
			return
		}
		json.NewEncoder(w).Encode(inst.Port.OpenPositions())
	})

	mux.HandleFunc("GET /closed", func(w http.ResponseWriter, r *http.Request) {
		walletID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		inst := mgr.Get(walletID)
		w.Header().Set("Content-Type", "application/json")
		if inst == nil {
			json.NewEncoder(w).Encode([]*models.ClosedPosition{})
			return
		}
		json.NewEncoder(w).Encode(inst.Port.RecentClosed(50))
	})

	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, r *http.Request) {
		walletID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		mgr.Stop(walletID)
		fmt.Fprint(w, `{"status":"stopped"}`)
	})

	mux.HandleFunc("POST /resume", func(w http.ResponseWriter, r *http.Request) {
		walletID, err := requireAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		inst := mgr.Get(walletID)
		if inst == nil {
			// Reload from DB and restart
			apiKey, apiSecret, err := database.GetCredentials(walletID)
			if err == nil {
				mgr.Start(walletID, apiKey, apiSecret)
			}
		} else {
			inst.Port.Resume()
		}
		fmt.Fprint(w, `{"status":"resumed"}`)
	})
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

func (noopNotifier) Entered(p *models.Position) {
	log.Printf("[notify] entered %s | %.3f SOL", p.Mint[:8], p.EntryAmountSOL)
}
func (noopNotifier) Exited(c *models.ClosedPosition) {
	log.Printf("[notify] exited %s | P&L %+.4f SOL", c.Mint[:8], c.PnLSOL)
}
func (noopNotifier) Alert(text string) { log.Printf("[notify] %s", text) }
