package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	signet "github.com/VYLTH/signet-sdk-go/signet"

	"github.com/iamdecatalyst/hummingbird/orchestrator/alerts"
	"github.com/iamdecatalyst/hummingbird/orchestrator/bot"
	"github.com/iamdecatalyst/hummingbird/orchestrator/config"
	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
	"github.com/iamdecatalyst/hummingbird/orchestrator/portfolio"
	"github.com/iamdecatalyst/hummingbird/orchestrator/trader"
)

func main() {
	cfg := config.Load()

	port := portfolio.New(1.0, cfg.MaxConcurrentPositions, cfg.MaxDailyLossPercent)
	scorerURL := "http://localhost:8001"

	// Try to set up Signet — non-fatal if creds are missing/wrong.
	// The HTTP server starts regardless so the dashboard can show the setup screen.
	configured := false
	var walletID string

	signetClient := signet.NewClient(cfg.SignetAPIKey, cfg.SignetAPISecret).
		WithBaseURL(cfg.SignetBaseURL)

	wid, err := trader.EnsureWallet(signetClient, "hummingbird-trader")
	if err != nil {
		log.Printf("[main] WARNING: wallet setup failed: %v", err)
		log.Printf("[main] Starting in unconfigured mode. Set SIGNET_API_KEY + SIGNET_API_SECRET in .env and restart.")
	} else {
		walletID = wid
		configured = true
	}

	// Init Telegram bot — falls back to no-op notifier if token/chatID not set.
	var notifier alerts.Notifier = noopNotifier{}
	var tgBot *bot.Bot

	if cfg.TelegramToken != "" && cfg.TelegramChatID != "" {
		chatID, parseErr := strconv.ParseInt(cfg.TelegramChatID, 10, 64)
		if parseErr != nil {
			log.Printf("[main] invalid TELEGRAM_CHAT_ID: %v", parseErr)
		} else {
			tgBot, err = bot.New(cfg.TelegramToken, chatID, port, nil)
			if err != nil {
				log.Printf("[main] bot init failed: %v", err)
			} else {
				notifier = tgBot
			}
		}
	}

	tr := trader.New(signetClient, walletID, port, notifier, cfg.SolanaRPC, scorerURL)

	if tgBot != nil {
		tgBot.SetExecutor(tr)
		go tgBot.Run()
	}

	go dailyStats(tgBot, port)

	// HTTP server
	mux := http.NewServeMux()

	// POST /trade — receives ScoreResult from Python scorer
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
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"queued"}`)
	})

	// GET /stats — current portfolio stats + configured flag
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		stats := port.Stats()
		type statsResp struct {
			portfolio.Stats
			Configured bool `json:"configured"`
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(statsResp{stats, configured})
	})

	// GET /positions — open positions
	mux.HandleFunc("GET /positions", func(w http.ResponseWriter, r *http.Request) {
		positions := port.OpenPositions()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(positions)
	})

	// POST /stop — close all positions + pause
	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, r *http.Request) {
		if !configured {
			fmt.Fprint(w, `{"status":"not configured"}`)
			return
		}
		log.Println("[main] /stop called — closing all positions")
		tr.ExitAll(models.ExitManual)
		notifier.Alert("Bot stopped manually. All positions closed.")
		fmt.Fprint(w, `{"status":"stopped"}`)
	})

	// POST /resume — unpause
	mux.HandleFunc("POST /resume", func(w http.ResponseWriter, r *http.Request) {
		port.Resume()
		if configured {
			notifier.Alert("Bot resumed.")
		}
		fmt.Fprint(w, `{"status":"resumed"}`)
	})

	// GET /health
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		if configured {
			fmt.Fprint(w, `{"status":"ok","configured":true}`)
		} else {
			fmt.Fprint(w, `{"status":"ok","configured":false}`)
		}
	})

	// GET /closed — recent closed positions (for dashboard)
	mux.HandleFunc("GET /closed", func(w http.ResponseWriter, r *http.Request) {
		closed := port.RecentClosed(50)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(closed)
	})

	addr := ":" + cfg.Port
	log.Printf("[main] Hummingbird Orchestrator running on %s", addr)
	if configured {
		log.Printf("[main] Wallet: %s", walletID)
		log.Printf("[main] Max positions: %d | Daily loss limit: %.0f%%",
			cfg.MaxConcurrentPositions, cfg.MaxDailyLossPercent*100)
		notifier.Alert("Hummingbird is online and watching pump.fun")
	} else {
		log.Printf("[main] UNCONFIGURED — trading disabled until credentials are set")
	}

	if err := http.ListenAndServe(addr, withCORS(mux)); err != nil {
		log.Fatalf("[main] server error: %v", err)
	}
}

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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
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
func (noopNotifier) Alert(text string) {
	log.Printf("[notify] %s", text)
}
