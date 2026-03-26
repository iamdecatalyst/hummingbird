package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	signet "github.com/VYLTH/signet-sdk-go/signet"

	"github.com/iamdecatalyst/hummingbird/orchestrator/alerts"
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

	// Init components
	tg := alerts.NewTelegram(cfg.TelegramToken, cfg.TelegramChatID)
	port := portfolio.New(1.0, cfg.MaxConcurrentPositions, cfg.MaxDailyLossPercent)
	tr := trader.New(signetClient, walletID, port, tg, cfg.SolanaRPC)

	// Daily stats ticker
	go dailyStats(tg, port)

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
		tg.Alert("Bot stopped manually. All positions closed.")
		fmt.Fprint(w, `{"status":"stopped"}`)
	})

	// POST /resume — unpause after daily loss limit
	mux.HandleFunc("POST /resume", func(w http.ResponseWriter, r *http.Request) {
		port.Resume()
		tg.Alert("Bot resumed.")
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

	tg.Alert("🐦 Hummingbird is online and watching pump.fun")

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[main] server error: %v", err)
	}
}

func dailyStats(tg *alerts.Telegram, port *portfolio.Portfolio) {
	for {
		now := time.Now()
		// Fire at midnight
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		time.Sleep(time.Until(next))

		stats := port.Stats()
		tg.DailyStats(stats.Wins, stats.Losses, stats.TodayPnL, stats.WinRate)
	}
}
