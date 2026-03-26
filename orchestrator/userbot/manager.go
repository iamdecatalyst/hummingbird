// Package userbot manages per-user trader instances in multi-tenant mode.
// Each user gets their own Portfolio + Trader goroutine, isolated from others.
package userbot

import (
	"fmt"
	"log"
	"sync"

	signet "github.com/VYLTH/signet-sdk-go/signet"
	"github.com/iamdecatalyst/hummingbird/orchestrator/alerts"
	"github.com/iamdecatalyst/hummingbird/orchestrator/config"
	"github.com/iamdecatalyst/hummingbird/orchestrator/eventlog"
	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
	"github.com/iamdecatalyst/hummingbird/orchestrator/portfolio"
	"github.com/iamdecatalyst/hummingbird/orchestrator/trader"
)

// Instance holds a user's isolated bot state.
type Instance struct {
	UserID   string
	WalletID string
	Port     *portfolio.Portfolio
	Trader   *trader.Trader
}

// Manager owns all active instances, keyed by user ID.
type Manager struct {
	mu        sync.RWMutex
	instances map[string]*Instance
	cfg       *config.Config
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		instances: make(map[string]*Instance),
		cfg:       cfg,
	}
}

// Start creates (or replaces) a bot instance for the user with the given credentials.
func (m *Manager) Start(userID, apiKey, apiSecret string) error {
	client := signet.NewClient(apiKey, apiSecret).WithBaseURL(m.cfg.SignetBaseURL)

	walletName := fmt.Sprintf("hummingbird-%s", userID[:8])
	walletID, err := trader.EnsureWallet(client, walletName)
	if err != nil {
		return fmt.Errorf("wallet setup: %w", err)
	}

	port := portfolio.New(1.0, m.cfg.MaxConcurrentPositions, m.cfg.MaxDailyLossPercent)
	var n alerts.Notifier = noopNotifier{userID}
	tr := trader.New(client, walletID, port, n, m.cfg.SolanaRPC, "http://localhost:8001")

	inst := &Instance{
		UserID:   userID,
		WalletID: walletID,
		Port:     port,
		Trader:   tr,
	}

	m.mu.Lock()
	m.instances[userID] = inst
	m.mu.Unlock()

	log.Printf("[userbot] started instance for user %s | wallet %s", userID[:8], walletID)
	eventlog.Emit(eventlog.Event{
		Type:    "START",
		Message: fmt.Sprintf("Bot started — wallet %s", walletID),
	})
	return nil
}

// Get returns the instance for a user, or nil if not started.
func (m *Manager) Get(userID string) *Instance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.instances[userID]
}

// Stop removes the instance and closes all positions.
func (m *Manager) Stop(userID string) {
	m.mu.Lock()
	inst, ok := m.instances[userID]
	if ok {
		delete(m.instances, userID)
	}
	m.mu.Unlock()
	if ok {
		inst.Trader.ExitAll(models.ExitManual)
		log.Printf("[userbot] stopped instance for user %s", userID[:8])
		eventlog.Emit(eventlog.Event{Type: "STOP", Message: "Bot stopped manually"})
	}
}

// noopNotifier logs trades to stdout tagged with the user ID.
type noopNotifier struct{ userID string }

func (n noopNotifier) Entered(p *models.Position) {
	log.Printf("[user:%s] entered %s | %.3f SOL", n.userID[:8], p.Mint[:8], p.EntryAmountSOL)
	eventlog.Emit(eventlog.Event{
		Type:    "ENTER",
		Token:   p.Mint,
		AmtSOL:  p.EntryAmountSOL,
		Message: fmt.Sprintf("Entered %s…  %.3f SOL", p.Mint[:8], p.EntryAmountSOL),
	})
}
func (n noopNotifier) Exited(c *models.ClosedPosition) {
	log.Printf("[user:%s] exited %s | P&L %+.4f SOL", n.userID[:8], c.Mint[:8], c.PnLSOL)
	eventlog.Emit(eventlog.Event{
		Type:    "EXIT",
		Token:   c.Mint,
		PnLSOL:  c.PnLSOL,
		PnLPct:  c.PnLPercent,
		Reason:  c.Reason,
		Message: fmt.Sprintf("Exited %s…  %+.4f SOL (%+.1f%%)", c.Mint[:8], c.PnLSOL, c.PnLPercent),
	})
}
func (n noopNotifier) Alert(text string) {
	log.Printf("[user:%s] %s", n.userID[:8], text)
	eventlog.Emit(eventlog.Event{Type: "ALERT", Message: text})
}
