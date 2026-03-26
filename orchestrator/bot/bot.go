package bot

import (
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
	"github.com/iamdecatalyst/hummingbird/orchestrator/portfolio"
)

// Executor is implemented by trader.Trader.
type Executor interface {
	ExitAll(reason models.ExitReason)
}

// Bot is the interactive Telegram command + inline keyboard handler.
type Bot struct {
	api       *tgbotapi.BotAPI
	chatID    int64
	portfolio *portfolio.Portfolio
	executor  Executor
	config    BotConfig
}

func New(token string, chatID int64, port *portfolio.Portfolio, exec Executor) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return &Bot{
		api:       api,
		chatID:    chatID,
		portfolio: port,
		executor:  exec,
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

// SetExecutor wires in the trader after both are constructed.
func (b *Bot) SetExecutor(exec Executor) {
	b.executor = exec
}

// Run starts long-polling. Blocks until the process exits.
func (b *Bot) Run() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)
	log.Printf("[bot] polling — @%s", b.api.Self.UserName)
	for update := range updates {
		switch {
		case update.Message != nil && update.Message.IsCommand():
			b.handleCommand(update.Message)
		case update.CallbackQuery != nil:
			b.handleCallback(update.CallbackQuery)
		}
	}
}

// ── Commands ──────────────────────────────────────────────────────────────────

func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	switch msg.Command() {
	case "start", "menu":
		b.sendMain(msg.Chat.ID)
	case "stats":
		b.sendStats(msg.Chat.ID)
	case "positions":
		b.sendPositions(msg.Chat.ID)
	case "config":
		b.sendConfig(msg.Chat.ID)
	case "stop":
		b.sendStopConfirm(msg.Chat.ID)
	case "pause":
		b.portfolio.Pause("manual pause")
		b.send(msg.Chat.ID,
			"⏸ <b>Bot paused.</b>\nNew entries blocked. Open positions still monitored.",
			backToMenuKB())
	case "resume":
		b.portfolio.Resume()
		b.send(msg.Chat.ID, "▶️ <b>Bot resumed.</b>", backToMenuKB())
	}
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
		b.executor.ExitAll(models.ExitManual)
		b.portfolio.Pause("stopped via Telegram")
		b.editText(cid, mid,
			"⏹ <b>Bot stopped.</b>\nAll positions closed. Use /resume to restart.",
			backToMenuKB())
	case "stop_cancel":
		b.editMain(cid, mid)
	case "pause":
		b.portfolio.Pause("manual pause")
		b.editMain(cid, mid)
	case "resume":
		b.portfolio.Resume()
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
	stats := b.portfolio.Stats()
	b.send(chatID, renderMain(stats, !stats.Paused), mainKB(stats))
}

func (b *Bot) sendStats(chatID int64) {
	stats := b.portfolio.Stats()
	recent := b.portfolio.RecentClosed(10)
	b.send(chatID, renderStats(stats, recent), statsKB())
}

func (b *Bot) sendPositions(chatID int64) {
	b.send(chatID, renderPositions(b.portfolio.OpenPositions()), positionsKB())
}

func (b *Bot) sendConfig(chatID int64) {
	b.send(chatID, renderConfig(b.config), configKB(b.config))
}

func (b *Bot) sendStopConfirm(chatID int64) {
	b.send(chatID, stopConfirmText(), stopConfirmKB())
}

// ── In-place editors ──────────────────────────────────────────────────────────

func (b *Bot) editMain(cid int64, mid int) {
	stats := b.portfolio.Stats()
	b.editText(cid, mid, renderMain(stats, !stats.Paused), mainKB(stats))
}

func (b *Bot) editStats(cid int64, mid int) {
	stats := b.portfolio.Stats()
	recent := b.portfolio.RecentClosed(10)
	b.editText(cid, mid, renderStats(stats, recent), statsKB())
}

func (b *Bot) editPositions(cid int64, mid int) {
	b.editText(cid, mid, renderPositions(b.portfolio.OpenPositions()), positionsKB())
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
// These replace alerts.Telegram — same chat, full HTML+ASCII box formatting.

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
