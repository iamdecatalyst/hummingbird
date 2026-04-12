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
	"github.com/iamdecatalyst/hummingbird/orchestrator/cricket"
	"github.com/iamdecatalyst/hummingbird/orchestrator/db"
	"github.com/iamdecatalyst/hummingbird/orchestrator/eventlog"
	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
	"github.com/iamdecatalyst/hummingbird/orchestrator/monitor"
	"github.com/iamdecatalyst/hummingbird/orchestrator/portfolio"
	"github.com/iamdecatalyst/hummingbird/orchestrator/trader"
)

// Instance holds a user's isolated bot state.
type Instance struct {
	UserID   string
	WalletID string
	Port     *portfolio.Portfolio
	Trader   *trader.Trader
	Log      *eventlog.Log
}

// Manager owns all active instances, keyed by user ID.
type Manager struct {
	mu          sync.RWMutex
	instances   map[string]*Instance
	chatToUser  map[string]string // telegram chat_id → nexus user ID
	cfg         *config.Config
	db          *db.DB
	cricket     *cricket.Client
	scalper     trader.ScalperCloser
}

func NewManager(cfg *config.Config, database *db.DB, cc *cricket.Client, sc trader.ScalperCloser) *Manager {
	return &Manager{
		instances:  make(map[string]*Instance),
		chatToUser: make(map[string]string),
		cfg:        cfg,
		db:         database,
		cricket:    cc,
		scalper:    sc,
	}
}

func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// Start creates (or replaces) a bot instance, ensuring a default wallet exists.
func (m *Manager) Start(userID, apiKey, apiSecret, telegramChatID string, userCfg *db.UserConfig) error {
	client := signet.NewClient(apiKey, apiSecret).WithBaseURL(m.cfg.SignetBaseURL)

	walletName := fmt.Sprintf("hummingbird-%s", short(userID))
	walletID, err := trader.EnsureWallet(client, walletName)
	if err != nil {
		return fmt.Errorf("wallet setup: %w", err)
	}
	return m.startInstance(userID, apiKey, apiSecret, walletID, telegramChatID, userCfg)
}

// StartWithWallet creates (or replaces) a bot instance using a specific wallet ID.
func (m *Manager) StartWithWallet(userID, apiKey, apiSecret, walletID, telegramChatID string, userCfg *db.UserConfig) error {
	return m.startInstance(userID, apiKey, apiSecret, walletID, telegramChatID, userCfg)
}

func (m *Manager) startInstance(userID, apiKey, apiSecret, walletID, telegramChatID string, userCfg *db.UserConfig) error {
	if userCfg == nil {
		userCfg = db.DefaultUserConfig()
	}
	client := signet.NewClient(apiKey, apiSecret).WithBaseURL(m.cfg.SignetBaseURL)

	maxPos := userCfg.MaxPositions
	if maxPos <= 0 {
		maxPos = m.cfg.MaxConcurrentPositions
	}
	dailyLoss := userCfg.DailyLossLimit
	if dailyLoss <= 0 {
		dailyLoss = m.cfg.MaxDailyLossPercent
	}
	port := portfolio.New(1.0, maxPos, dailyLoss)

	// Per-user event log — persisted to Postgres asynchronously.
	userLog := eventlog.New(func(e eventlog.Event) {
		if m.db != nil {
			go m.db.InsertEvent(userID, e)
		}
	})
	if m.db != nil {
		if recent, err := m.db.RecentEvents(userID, 500); err == nil {
			userLog.Load(recent)
		}
	}

	var n alerts.Notifier
	if telegramChatID != "" && m.cfg.TelegramToken != "" {
		n = alerts.NewTelegram(m.cfg.TelegramToken, telegramChatID).WithLog(userLog)
	} else {
		n = noopNotifier{userID, userLog}
	}

	// Persist hooks — fire-and-forget DB writes for position lifecycle.
	if m.db != nil {
		port.SetPersistHooks(
			func(pos *models.Position) {
				if err := m.db.SavePosition(userID, pos); err != nil {
					log.Printf("[userbot] SavePosition failed for user %s: %v", short(userID), err)
				}
			},
			func(closed *models.ClosedPosition) {
				if err := m.db.ClosePosition(userID, closed); err != nil {
					log.Printf("[userbot] ClosePosition failed for user %s: %v", short(userID), err)
				}
			},
		)
	}

	monCfg := monitor.MonitorConfig{
		StopLossPercent: userCfg.StopLossPercent,
		TakeProfit1x:    userCfg.TakeProfit1x,
		TakeProfit2x:    userCfg.TakeProfit2x,
		TakeProfit3x:    userCfg.TakeProfit3x,
		TimeoutMinutes:  userCfg.TimeoutMinutes,
	}

	tr := trader.New(client, walletID, port, n, m.cricket, m.scalper, monCfg, userCfg.MinBalanceSOL)

	// Restore open positions from DB so monitors resume after a restart.
	if m.db != nil {
		if openPositions, err := m.db.OpenPositionsByUser(userID); err == nil && len(openPositions) > 0 {
			for _, pos := range openPositions {
				tr.Restore(pos)
			}
			log.Printf("[userbot] restored %d open position(s) for user %s", len(openPositions), short(userID))
		}
	}

	inst := &Instance{
		UserID:   userID,
		WalletID: walletID,
		Port:     port,
		Trader:   tr,
		Log:      userLog,
	}

	m.mu.Lock()
	m.instances[userID] = inst
	if telegramChatID != "" {
		m.chatToUser[telegramChatID] = userID
	}
	m.mu.Unlock()

	log.Printf("[userbot] started instance for user %s | wallet %s | tg=%v",
		short(userID), walletID, telegramChatID != "")
	userLog.Emit(eventlog.Event{
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

// All returns a snapshot of all active instances.
func (m *Manager) All() []*Instance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Instance, 0, len(m.instances))
	for _, inst := range m.instances {
		out = append(out, inst)
	}
	return out
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
		inst.Log.Emit(eventlog.Event{Type: "STOP", Message: "Bot stopped manually"})
	}
}

// noopNotifier logs trades to stdout and emits to the per-user event log.
type noopNotifier struct {
	userID string
	log    *eventlog.Log
}

func (n noopNotifier) Entered(p *models.Position) {
	log.Printf("[user:%s] entered %s | %.3f SOL", short(n.userID), short(p.Mint), p.EntryAmountSOL)
	n.log.Emit(eventlog.Event{
		Type:    "ENTER",
		Token:   p.Mint,
		AmtSOL:  p.EntryAmountSOL,
		Message: fmt.Sprintf("Entered %s…  %.3f SOL", short(p.Mint), p.EntryAmountSOL),
	})
}
func (n noopNotifier) Exited(c *models.ClosedPosition) {
	log.Printf("[user:%s] exited %s | P&L %+.4f SOL", short(n.userID), short(c.Mint), c.PnLSOL)
	n.log.Emit(eventlog.Event{
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
	n.log.Emit(eventlog.Event{Type: "ALERT", Message: text})
}
