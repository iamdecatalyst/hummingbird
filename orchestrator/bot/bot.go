package bot

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
	"github.com/iamdecatalyst/hummingbird/orchestrator/portfolio"
)

// Executor is implemented by trader.Trader.
type Executor interface {
	ExitAll(reason models.ExitReason)
}

// linkEntry is a one-time deep-link token.
type linkEntry struct {
	nexusID string
	exp     time.Time
}

// Bot is the interactive Telegram command + inline keyboard handler.
type Bot struct {
	api      *tgbotapi.BotAPI
	username string // resolved at Run()

	// single-tenant fields
	chatID    int64
	portfolio *portfolio.Portfolio
	executor  Executor
	config    BotConfig

	// multi-tenant callbacks (nil = single-tenant)
	resolve func(chatID int64) (nexusID string, port *portfolio.Portfolio, exec Executor, ok bool)
	onLink  func(nexusID string, chatID int64)
	onStop  func(nexusID string)

	// deep-link token store (both modes)
	mu         sync.Mutex
	linkTokens map[string]linkEntry
}

func New(token string, chatID int64, port *portfolio.Portfolio, exec Executor) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return &Bot{
		api:        api,
		chatID:     chatID,
		portfolio:  port,
		executor:   exec,
		linkTokens: make(map[string]linkEntry),
		config: BotConfig{
			SniperEnabled:   true,
			ScalperEnabled:  true,
			MaxPositionSOL:  0.10,
			MaxPositions:    5,
			StopLossPercent: 0.25,
			DailyLossLimit:  0.30,
		},
	}, nil
}

// NewMultiTenant creates a bot for multi-tenant mode.
// resolve: given a Telegram chat_id, return the user's portfolio + executor
// onLink:  called when a deep-link token is validated (save the chat_id to DB)
// onStop:  called when a user stops their bot via Telegram (remove instance)
func NewMultiTenant(
	token string,
	resolve func(int64) (string, *portfolio.Portfolio, Executor, bool),
	onLink func(string, int64),
	onStop func(string),
) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return &Bot{
		api:        api,
		resolve:    resolve,
		onLink:     onLink,
		onStop:     onStop,
		linkTokens: make(map[string]linkEntry),
		config: BotConfig{
			SniperEnabled:   true,
			ScalperEnabled:  true,
			MaxPositionSOL:  0.10,
			MaxPositions:    5,
			StopLossPercent: 0.25,
			DailyLossLimit:  0.30,
		},
	}, nil
}

// SetExecutor wires in the trader after both are constructed (single-tenant).
func (b *Bot) SetExecutor(exec Executor) {
	b.executor = exec
}

// Username returns the bot's Telegram username (available after Run starts).
func (b *Bot) Username() string {
	return b.username
}

// GenerateLinkToken creates a one-time token valid for 10 minutes.
func (b *Bot) GenerateLinkToken(nexusID string) string {
	raw := make([]byte, 16)
	rand.Read(raw)
	token := hex.EncodeToString(raw)

	b.mu.Lock()
	// purge expired tokens
	for k, v := range b.linkTokens {
		if time.Now().After(v.exp) {
			delete(b.linkTokens, k)
		}
	}
	b.linkTokens[token] = linkEntry{nexusID: nexusID, exp: time.Now().Add(10 * time.Minute)}
	b.mu.Unlock()
	return token
}

// Run starts long-polling. Blocks until the process exits.
func (b *Bot) Run() {
	b.username = b.api.Self.UserName
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)
	log.Printf("[bot] polling — @%s (multi_tenant=%v)", b.username, b.resolve != nil)
	for update := range updates {
		switch {
		case update.Message != nil && update.Message.IsCommand():
			b.handleCommand(update.Message)
		case update.CallbackQuery != nil:
			b.handleCallback(update.CallbackQuery)
		}
	}
}

// ── Context helper ────────────────────────────────────────────────────────────

// ctx resolves the portfolio and executor for the given chat ID.
// In single-tenant mode, always returns the configured values.
// In multi-tenant mode, looks up by chat ID.
func (b *Bot) ctx(chatID int64) (nexusID string, port *portfolio.Portfolio, exec Executor, ok bool) {
	if b.resolve == nil {
		return "", b.portfolio, b.executor, b.portfolio != nil
	}
	return b.resolve(chatID)
}

// ── Commands ──────────────────────────────────────────────────────────────────

func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	cid := msg.Chat.ID

	// Deep-link token handling — must come before other commands
	if msg.Command() == "start" {
		token := msg.CommandArguments()
		if token != "" {
			b.handleLinkToken(cid, token)
			return
		}
		if b.resolve != nil {
			// Multi-tenant: no token, no linked account
			_, _, _, ok := b.ctx(cid)
			if !ok {
				b.send(cid, strings.Join([]string{
					"🐦 <b>HUMMINGBIRD</b>",
					"",
					"To receive trade alerts here, connect your Telegram from the dashboard:",
					"<b>hummingbird.vylth.com</b> → Key icon → Connect Telegram",
				}, "\n"), nil)
				return
			}
		}
		b.sendMain(cid)
		return
	}

	switch msg.Command() {
	case "menu":
		b.sendMain(cid)
	case "stats":
		b.sendStats(cid)
	case "positions":
		b.sendPositions(cid)
	case "config":
		b.sendConfig(cid)
	case "stop":
		b.sendStopConfirm(cid)
	case "pause":
		_, port, _, ok := b.ctx(cid)
		if !ok {
			b.send(cid, "No active bot.", nil)
			return
		}
		port.Pause("manual pause")
		b.send(cid,
			"⏸ <b>Bot paused.</b>\nNew entries blocked. Open positions still monitored.",
			backToMenuKB())
	case "resume":
		_, port, _, ok := b.ctx(cid)
		if !ok {
			b.send(cid, "No active bot.", nil)
			return
		}
		port.Resume()
		b.send(cid, "▶️ <b>Bot resumed.</b>", backToMenuKB())
	}
}

// handleLinkToken validates a deep-link token and links the chat ID to an account.
func (b *Bot) handleLinkToken(chatID int64, token string) {
	b.mu.Lock()
	entry, ok := b.linkTokens[token]
	if ok {
		delete(b.linkTokens, token)
	}
	b.mu.Unlock()

	if !ok || time.Now().After(entry.exp) {
		b.send(chatID, "⚠️ This link has expired. Generate a new one from the dashboard.", nil)
		return
	}

	if b.onLink != nil {
		b.onLink(entry.nexusID, chatID)
	}

	b.send(chatID, strings.Join([]string{
		"✅ <b>Telegram connected!</b>",
		"",
		"You'll now receive real-time trade alerts here.",
		"Use /menu to see your bot status.",
	}, "\n"), backToMenuKB())
}

// ── Callback dispatcher ───────────────────────────────────────────────────────

func (b *Bot) handleCallback(cq *tgbotapi.CallbackQuery) {
	b.api.Request(tgbotapi.NewCallback(cq.ID, "")) //nolint:errcheck

	cid := cq.Message.Chat.ID
	mid := cq.Message.MessageID

	switch cq.Data {
	case "menu":
		b.editMain(cid, mid)
	case "stats":
		b.editStats(cid, mid)
	case "positions":
		b.editPositions(cid, mid)
	case "config":
		b.editConfig(cid, mid)
	case "stop":
		b.editStopConfirm(cid, mid)
	case "stop_confirm":
		nexusID, port, exec, ok := b.ctx(cid)
		if ok {
			if exec != nil {
				exec.ExitAll(models.ExitManual)
			}
			port.Pause("stopped via Telegram")
			if b.onStop != nil && nexusID != "" {
				b.onStop(nexusID)
			}
		}
		b.editText(cid, mid,
			"⏹ <b>Bot stopped.</b>\nAll positions closed. Use /resume to restart.",
			backToMenuKB())
	case "stop_cancel":
		b.editMain(cid, mid)
	case "pause":
		_, port, _, ok := b.ctx(cid)
		if ok {
			port.Pause("manual pause")
		}
		b.editMain(cid, mid)
	case "resume":
		_, port, _, ok := b.ctx(cid)
		if ok {
			port.Resume()
		}
		b.editMain(cid, mid)
	case "sniper_toggle":
		b.config.SniperEnabled = !b.config.SniperEnabled
		b.editConfig(cid, mid)
	case "scalper_toggle":
		b.config.ScalperEnabled = !b.config.ScalperEnabled
		b.editConfig(cid, mid)
	case "refresh_main":
		b.editMain(cid, mid)
	case "refresh_stats":
		b.editStats(cid, mid)
	case "refresh_positions":
		b.editPositions(cid, mid)
	}
}

// ── New message senders ───────────────────────────────────────────────────────

func (b *Bot) sendMain(chatID int64) {
	_, port, _, ok := b.ctx(chatID)
	if !ok {
		b.send(chatID, "No active bot. Connect Telegram from your dashboard first.", nil)
		return
	}
	stats := port.Stats()
	b.send(chatID, renderMain(stats, !stats.Paused), mainKB(stats))
}

func (b *Bot) sendStats(chatID int64) {
	_, port, _, ok := b.ctx(chatID)
	if !ok {
		b.send(chatID, "No active bot.", nil)
		return
	}
	stats := port.Stats()
	recent := port.RecentClosed(10)
	b.send(chatID, renderStats(stats, recent), statsKB())
}

func (b *Bot) sendPositions(chatID int64) {
	_, port, _, ok := b.ctx(chatID)
	if !ok {
		b.send(chatID, "No active bot.", nil)
		return
	}
	b.send(chatID, renderPositions(port.OpenPositions()), positionsKB())
}

func (b *Bot) sendConfig(chatID int64) {
	b.send(chatID, renderConfig(b.config), configKB(b.config))
}

func (b *Bot) sendStopConfirm(chatID int64) {
	b.send(chatID, stopConfirmText(), stopConfirmKB())
}

// ── In-place editors ──────────────────────────────────────────────────────────

func (b *Bot) editMain(cid int64, mid int) {
	_, port, _, ok := b.ctx(cid)
	if !ok {
		b.editText(cid, mid, "No active bot.", nil)
		return
	}
	stats := port.Stats()
	b.editText(cid, mid, renderMain(stats, !stats.Paused), mainKB(stats))
}

func (b *Bot) editStats(cid int64, mid int) {
	_, port, _, ok := b.ctx(cid)
	if !ok {
		b.editText(cid, mid, "No active bot.", nil)
		return
	}
	stats := port.Stats()
	recent := port.RecentClosed(10)
	b.editText(cid, mid, renderStats(stats, recent), statsKB())
}

func (b *Bot) editPositions(cid int64, mid int) {
	_, port, _, ok := b.ctx(cid)
	if !ok {
		b.editText(cid, mid, "No active bot.", nil)
		return
	}
	b.editText(cid, mid, renderPositions(port.OpenPositions()), positionsKB())
}

func (b *Bot) editConfig(cid int64, mid int) {
	b.editText(cid, mid, renderConfig(b.config), configKB(b.config))
}

func (b *Bot) editStopConfirm(cid int64, mid int) {
	b.editText(cid, mid, stopConfirmText(), stopConfirmKB())
}

// ── Low-level Telegram helpers ────────────────────────────────────────────────

func (b *Bot) send(chatID int64, text string, kb *tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	if kb != nil {
		msg.ReplyMarkup = *kb
	}
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("[bot] send: %v", err)
	}
}

func (b *Bot) editText(chatID int64, msgID int, text string, kb *tgbotapi.InlineKeyboardMarkup) {
	edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
	edit.ParseMode = "HTML"
	if kb != nil {
		edit.ReplyMarkup = kb
	}
	if _, err := b.api.Request(edit); err != nil {
		if !strings.Contains(err.Error(), "message is not modified") {
			log.Printf("[bot] edit: %v", err)
		}
	}
}

// btn shorthand
func btn(label, data string) tgbotapi.InlineKeyboardButton {
	return tgbotapi.NewInlineKeyboardButtonData(label, data)
}

// ── Keyboards ─────────────────────────────────────────────────────────────────

func mainKB(stats portfolio.Stats) *tgbotapi.InlineKeyboardMarkup {
	rows := [][]tgbotapi.InlineKeyboardButton{
		{btn("📊 Stats", "stats"), btn("📍 Positions", "positions")},
		{btn("⚙️ Config", "config"), btn("⏹ Stop", "stop")},
		{btn("🔄 Refresh", "refresh_main")},
	}
	if stats.Paused {
		rows = append([][]tgbotapi.InlineKeyboardButton{
			{btn("▶️ Resume", "resume")},
		}, rows...)
	}
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	return &kb
}

func statsKB() *tgbotapi.InlineKeyboardMarkup {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		[]tgbotapi.InlineKeyboardButton{
			btn("🔄 Refresh", "refresh_stats"),
			btn("◀ Menu", "menu"),
		},
	)
	return &kb
}

func positionsKB() *tgbotapi.InlineKeyboardMarkup {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		[]tgbotapi.InlineKeyboardButton{
			btn("🔄 Refresh", "refresh_positions"),
			btn("◀ Menu", "menu"),
		},
	)
	return &kb
}

func configKB(cfg BotConfig) *tgbotapi.InlineKeyboardMarkup {
	sniperBtn := "Sniper  ✅ ON"
	if !cfg.SniperEnabled {
		sniperBtn = "Sniper  ❌ OFF"
	}
	scalperBtn := "Scalper  ✅ ON"
	if !cfg.ScalperEnabled {
		scalperBtn = "Scalper  ❌ OFF"
	}
	kb := tgbotapi.NewInlineKeyboardMarkup(
		[]tgbotapi.InlineKeyboardButton{btn(sniperBtn, "sniper_toggle")},
		[]tgbotapi.InlineKeyboardButton{btn(scalperBtn, "scalper_toggle")},
		[]tgbotapi.InlineKeyboardButton{btn("◀ Menu", "menu")},
	)
	return &kb
}

func stopConfirmKB() *tgbotapi.InlineKeyboardMarkup {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		[]tgbotapi.InlineKeyboardButton{
			btn("✅ Confirm — stop all", "stop_confirm"),
		},
		[]tgbotapi.InlineKeyboardButton{
			btn("❌ Cancel", "stop_cancel"),
		},
	)
	return &kb
}

func backToMenuKB() *tgbotapi.InlineKeyboardMarkup {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		[]tgbotapi.InlineKeyboardButton{btn("◀ Menu", "menu")},
	)
	return &kb
}

func stopConfirmText() string {
	return strings.Join([]string{
		"⏹ <b>STOP BOT</b>",
		divider,
		"",
		"This will:",
		"  • Close <b>all open positions</b> immediately",
		"  • Execute market-sell orders (5% slippage)",
		"  • Pause all new trade entries",
		"",
		"<b>Are you sure?</b>",
	}, "\n")
}

// ── Outbound push notifications ───────────────────────────────────────────────

func (b *Bot) Entered(p *models.Position) {
	b.send(b.chatID, renderEntered(p), nil)
}

func (b *Bot) Exited(c *models.ClosedPosition) {
	b.send(b.chatID, renderExited(c), nil)
}

func (b *Bot) DailyStats(stats portfolio.Stats) {
	b.send(b.chatID, renderDailyStats(stats), nil)
}

func (b *Bot) Alert(text string) {
	b.send(b.chatID, "⚠️  "+text, nil)
}

// ── unused import guard ───────────────────────────────────────────────────────
var _ = fmt.Sprintf
