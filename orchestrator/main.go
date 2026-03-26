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

	// Signet client
	signetClient := signet.NewClient(cfg.SignetAPIKey, cfg.SignetAPISecret).
		WithBaseURL(cfg.SignetBaseURL)

	// Ensure trading wallet exists
	walletID, err := trader.EnsureWallet(signetClient, "hummingbird-trader")
	if err != nil {
		log.Fatalf("[main] wallet setup failed: %v", err)
	}

	port := portfolio.New(1.0, cfg.MaxConcurrentPositions, cfg.MaxDailyLossPercent)
	scorerURL := "http://localhost:8001"

	// Init Telegram bot — it is both the interactive handler and the notifier.
	// Falls back to a no-op notifier if token/chatID are not set.
	var notifier alerts.Notifier = noopNotifier{}
	var tgBot *bot.Bot

	if cfg.TelegramToken != "" && cfg.TelegramChatID != "" {
		chatID, parseErr := strconv.ParseInt(cfg.TelegramChatID, 10, 64)
		if parseErr != nil {
			log.Printf("[main] invalid TELEGRAM_CHAT_ID: %v", parseErr)
		} else {
			tgBot, err = bot.New(cfg.TelegramToken, chatID, port, nil) // executor wired below
			if err != nil {
				log.Printf("[main] bot init failed: %v", err)
			} else {
				notifier = tgBot
			}
		}
	}

	tr := trader.New(signetClient, walletID, port, notifier, cfg.SolanaRPC, scorerURL)

	// Now that trader exists, wire it into the bot as executor
	if tgBot != nil {
		tgBot.SetExecutor(tr)
		go tgBot.Run()
	}

	// Daily stats ticker
	go dailyStats(tgBot, port)

	// HTTP server
	mux := http.NewServeMux()

	// POST /trade — receives ScoreResult from Python scorer
	mux.HandleFunc("POST /trade", func(w http.ResponseWriter, r *http.Request) {
		var result models.ScoreResult
		if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		tr.Execute(&result)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"queued"}`)
	})

	// GET /stats — current portfolio stats
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		stats := port.Stats()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	})

	// GET /positions — open positions
	mux.HandleFunc("GET /positions", func(w http.ResponseWriter, r *http.Request) {
		positions := port.OpenPositions()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(positions)
	})

	// POST /stop — close all positions + pause
	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, r *http.Request) {
		log.Println("[main] /stop called — closing all positions")
		tr.ExitAll(models.ExitManual)
		notifier.Alert("Bot stopped manually. All positions closed.")
		fmt.Fprint(w, `{"status":"stopped"}`)
	})

	// POST /resume — unpause after daily loss limit
	mux.HandleFunc("POST /resume", func(w http.ResponseWriter, r *http.Request) {
		port.Resume()
		notifier.Alert("Bot resumed.")
		fmt.Fprint(w, `{"status":"resumed"}`)
	})

	// GET /health
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	addr := ":" + cfg.Port
	log.Printf("🐦 Hummingbird Orchestrator running on %s", addr)
	log.Printf("   Wallet: %s", walletID)
	log.Printf("   Max positions: %d", cfg.MaxConcurrentPositions)
	log.Printf("   Daily loss limit: %.0f%%", cfg.MaxDailyLossPercent*100)

	notifier.Alert("🐦 Hummingbird is online and watching pump.fun")

	if err := http.ListenAndServe(addr, mux); err != nil {
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

// noopNotifier is used when Telegram is not configured.
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
