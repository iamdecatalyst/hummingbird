package main

import (
	"context"
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
	"github.com/iamdecatalyst/hummingbird/orchestrator/cricket"
	"github.com/iamdecatalyst/hummingbird/orchestrator/db"
	"github.com/iamdecatalyst/hummingbird/orchestrator/eventlog"
	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
	"github.com/iamdecatalyst/hummingbird/orchestrator/monitor"
	"github.com/iamdecatalyst/hummingbird/orchestrator/portfolio"
	"github.com/iamdecatalyst/hummingbird/orchestrator/scalper"
	"github.com/iamdecatalyst/hummingbird/orchestrator/trader"
	"github.com/iamdecatalyst/hummingbird/orchestrator/userbot"
)

func main() {
	cfg := config.Load()
	mux := http.NewServeMux()

	// Cricket client — powers all scoring, signal detection, and rug monitoring.
	// Users need a Cricket account: https://cricket.vylth.com
	cc := cricket.New(cfg.CricketURL, cfg.CricketKey)
	if cfg.CricketKey == "" {
		log.Printf("[main] WARNING: CRICKET_API_KEY not set — scoring will fail. Get your key at https://cricket.vylth.com")
	} else {
		log.Printf("[main] Cricket Protocol connected (%s)", cfg.CricketURL)
	}

	// Always public — lets the frontend know which mode to render
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
	log.Printf("[main] Hummingbird listening on %s (multi_tenant=%v)", addr, cfg.MultiTenant)
	if err := http.ListenAndServe(addr, withCORS(mux)); err != nil {
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
	tr = trader.New(signetClient, walletID, port, notifier, cc, sc, monitor.DefaultMonitorConfig())

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
		go scoreAndTrade(cc, tr.Execute, token)
		fmt.Fprintf(w, `{"status":"queued","mint":"%s"}`, token.Mint)
	})

	// POST /trade — legacy internal endpoint kept for compatibility.
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
		}, token)
		fmt.Fprintf(w, `{"status":"queued","mint":"%s"}`, token.Mint)
	})

	// POST /trade — legacy internal endpoint kept for compatibility.
	mux.HandleFunc("POST /trade", func(w http.ResponseWriter, r *http.Request) {
		var result models.ScoreResult
		if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
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
		inst := mgr.Get(nexusID)
		w.Header().Set("Content-Type", "application/json")
		if inst == nil {
			json.NewEncoder(w).Encode([]*models.ClosedPosition{})
			return
		}
		json.NewEncoder(w).Encode(inst.Port.RecentClosed(50))
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

// ── Cricket scoring ───────────────────────────────────────────────────────────

// scoreAndTrade runs a Cricket risk analysis on a freshly-detected token and,
// if the score passes, calls dispatch to send it to the active trader(s).
// Runs in a goroutine — the listener is never blocked.
func scoreAndTrade(cc *cricket.Client, dispatch func(*models.ScoreResult), token cricket.TokenDetected) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
		d, err := cc.MantisScan(ctx, token.Mint)
		scanCh <- scanRes{d, err}
	}()
	go func() {
		d, err := cc.FireflyWallet(ctx, token.DevWallet)
		walletCh <- walletRes{d, err}
	}()

	sr := <-scanCh
	wr := <-walletCh

	if sr.err != nil {
		log.Printf("[scorer] mantis scan failed for %s: %v — need Cricket API key? https://cricket.vylth.com", safeShort(token.Mint), sr.err)
		return
	}

	score, decision, posSOL := scoreFromCricket(sr.data, wr.data)

	entered := decision != "skip"
	sym := map[bool]string{true: "✅", false: "⏭ "}[entered]
	log.Printf("[scorer] %s %s…  score=%d  decision=%s  pos=%.2f SOL  (rating=%s)",
		sym, safeShort(token.Mint), score, decision, posSOL, sr.data.Data.RiskScore.Rating)

	if !entered {
		return
	}

	dispatch(&models.ScoreResult{
		Mint:        token.Mint,
		DevWallet:   token.DevWallet,
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
	switch scan.Data.RiskScore.Rating {
	case "critical", "high":
		return 0, "skip", 0
	case "moderate":
		score = 70
	case "low":
		score = 90
	default:
		return 0, "skip", 0 // unknown rating — skip to be safe
	}

	if devWallet != nil && devWallet.Success {
		dw := devWallet.Data
		if dw.Style == "smart_contract_deployer" && dw.WinRate > 75 {
			score -= 25 // experienced deployer with high win rate = likely professional rugger
		}
		if dw.Score < 20 {
			score -= 15 // very low smart-money score = flagged bad actor
		}
	}

	switch {
	case score < 60:
		return score, "skip", 0
	case score < 75:
		return score, "small", 0.05
	case score < 90:
		return score, "medium", 0.10
	default:
		return score, "full", 0.20
	}
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
func (noopNotifier) Alert(text string) { log.Printf("[notify] %s", text) }
