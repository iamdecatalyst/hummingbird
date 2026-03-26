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
	mu          sync.RWMutex
	instances   map[string]*Instance
	chatToUser  map[string]string // telegram chat_id → nexus user ID
	cfg         *config.Config
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		instances:  make(map[string]*Instance),
		chatToUser: make(map[string]string),
		cfg:        cfg,
	}
}

func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// Start creates (or replaces) a bot instance, ensuring a default wallet exists.
func (m *Manager) Start(userID, apiKey, apiSecret, telegramChatID string) error {
	client := signet.NewClient(apiKey, apiSecret).WithBaseURL(m.cfg.SignetBaseURL)

	walletName := fmt.Sprintf("hummingbird-%s", short(userID))
	walletID, err := trader.EnsureWallet(client, walletName)
	if err != nil {
		return fmt.Errorf("wallet setup: %w", err)
	}
	return m.startInstance(userID, apiKey, apiSecret, walletID, telegramChatID)
}

// StartWithWallet creates (or replaces) a bot instance using a specific wallet ID.
func (m *Manager) StartWithWallet(userID, apiKey, apiSecret, walletID, telegramChatID string) error {
	return m.startInstance(userID, apiKey, apiSecret, walletID, telegramChatID)
}

func (m *Manager) startInstance(userID, apiKey, apiSecret, walletID, telegramChatID string) error {
	client := signet.NewClient(apiKey, apiSecret).WithBaseURL(m.cfg.SignetBaseURL)

	port := portfolio.New(1.0, m.cfg.MaxConcurrentPositions, m.cfg.MaxDailyLossPercent)

	var n alerts.Notifier
	if telegramChatID != "" && m.cfg.TelegramToken != "" {
		n = alerts.NewTelegram(m.cfg.TelegramToken, telegramChatID)
	} else {
		n = noopNotifier{userID}
	}

	tr := trader.New(client, walletID, port, n, m.cfg.SolanaRPC, "http://localhost:8001")

	inst := &Instance{
		UserID:   userID,
		WalletID: walletID,
		Port:     port,
		Trader:   tr,
	}

	m.mu.Lock()
	m.instances[userID] = inst
	if telegramChatID != "" {
		m.chatToUser[telegramChatID] = userID
	}
	m.mu.Unlock()

	log.Printf("[userbot] started instance for user %s | wallet %s | tg=%v",
		short(userID), walletID, telegramChatID != "")
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

// GetByChatID looks up an instance by Telegram chat ID.
func (m *Manager) GetByChatID(chatID string) (string, *Instance) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	nexusID := m.chatToUser[chatID]
	if nexusID == "" {
		return "", nil
	}
	return nexusID, m.instances[nexusID]
}

// Stop removes the instance and closes all positions.
func (m *Manager) Stop(userID string) {
	m.mu.Lock()
	inst, ok := m.instances[userID]
	if ok {
		delete(m.instances, userID)
		// Remove reverse chat mapping
		for chatID, uid := range m.chatToUser {
			if uid == userID {
				delete(m.chatToUser, chatID)
				break
			}
		}
	}
	m.mu.Unlock()
	if ok {
		inst.Trader.ExitAll(models.ExitManual)
		log.Printf("[userbot] stopped instance for user %s", short(userID))
		eventlog.Emit(eventlog.Event{Type: "STOP", Message: "Bot stopped manually"})
	}
}

// noopNotifier logs trades to stdout tagged with the user ID.
type noopNotifier struct{ userID string }

func (n noopNotifier) Entered(p *models.Position) {
	log.Printf("[user:%s] entered %s | %.3f SOL", short(n.userID), short(p.Mint), p.EntryAmountSOL)
	eventlog.Emit(eventlog.Event{
		Type:    "ENTER",
		Token:   p.Mint,
		AmtSOL:  p.EntryAmountSOL,
		Message: fmt.Sprintf("Entered %s…  %.3f SOL", short(p.Mint), p.EntryAmountSOL),
	})
}
func (n noopNotifier) Exited(c *models.ClosedPosition) {
	log.Printf("[user:%s] exited %s | P&L %+.4f SOL", short(n.userID), short(c.Mint), c.PnLSOL)
	eventlog.Emit(eventlog.Event{
		Type:    "EXIT",
		Token:   c.Mint,
		PnLSOL:  c.PnLSOL,
		PnLPct:  c.PnLPercent,
		Reason:  string(c.Reason),
		Message: fmt.Sprintf("Exited %s…  %+.4f SOL (%+.1f%%)", short(c.Mint), c.PnLSOL, c.PnLPercent),
	})
}
func (n noopNotifier) Alert(text string) {
	log.Printf("[user:%s] %s", short(n.userID), text)
	eventlog.Emit(eventlog.Event{Type: "ALERT", Message: text})
}
