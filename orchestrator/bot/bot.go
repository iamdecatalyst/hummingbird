package bot

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
	"github.com/iamdecatalyst/hummingbird/orchestrator/pnl"
	"github.com/iamdecatalyst/hummingbird/orchestrator/portfolio"
)

// Executor is implemented by trader.Trader.
type Executor interface {
	ExitAll(reason models.ExitReason)
	Close(mint string, reason models.ExitReason) // close a single position by mint
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
	resolve     func(chatID int64) (nexusID string, port *portfolio.Portfolio, exec Executor, ok bool)
	onLink      func(nexusID string, chatID int64)
	onStop      func(nexusID string)
	onGetConfig func(nexusID string) BotConfig
	onSetConfig func(nexusID string, cfg BotConfig)

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
// resolve:      given a Telegram chat_id, return the user's portfolio + executor
// onLink:       called when a deep-link token is validated (save the chat_id to DB)
// onStop:       called when a user stops their bot via Telegram (remove instance)
// onGetConfig:  load per-user config from persistent storage
// onSetConfig:  save per-user config to persistent storage
func NewMultiTenant(
	token string,
	resolve func(int64) (string, *portfolio.Portfolio, Executor, bool),
	onLink func(string, int64),
	onStop func(string),
	onGetConfig func(string) BotConfig,
	onSetConfig func(string, BotConfig),
) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return &Bot{
		api:         api,
		resolve:     resolve,
		onLink:      onLink,
		onStop:      onStop,
		onGetConfig: onGetConfig,
		onSetConfig: onSetConfig,
		linkTokens:  make(map[string]linkEntry),
		config: BotConfig{
			SniperEnabled:   true,
			ScalperEnabled:  true,
			MaxPositionSOL:  0.10,
			MaxPositions:    5,
			StopLossPercent: 0.25,
			DailyLossLimit:  0.30,
			TakeProfit1x:    2.0,
			TakeProfit2x:    5.0,
			TakeProfit3x:    10.0,
			TimeoutMinutes:  8,
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

// getConfig returns the config for the user associated with chatID.
// In single-tenant mode, returns b.config. In multi-tenant mode, loads from DB.
func (b *Bot) getConfig(chatID int64) BotConfig {
	if b.onGetConfig != nil {
		nexusID, _, _, ok := b.ctx(chatID)
		if ok && nexusID != "" {
			return b.onGetConfig(nexusID)
		}
	}
	return b.config
}

// setConfig saves the config for the user associated with chatID.
func (b *Bot) setConfig(chatID int64, cfg BotConfig) {
	if b.onSetConfig != nil {
		nexusID, _, _, ok := b.ctx(chatID)
		if ok && nexusID != "" {
			b.onSetConfig(nexusID, cfg)
			return
		}
	}
	b.config = cfg
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
		cfg := b.getConfig(cid)
		cfg.SniperEnabled = !cfg.SniperEnabled
		b.setConfig(cid, cfg)
		b.editConfig(cid, mid)
	case "scalper_toggle":
		cfg := b.getConfig(cid)
		cfg.ScalperEnabled = !cfg.ScalperEnabled
		b.setConfig(cid, cfg)
		b.editConfig(cid, mid)
	case "noop":
		// display-only button in config keyboard — do nothing
	case "refresh_main":
		b.editMain(cid, mid)
	case "refresh_stats":
		b.editStats(cid, mid)
	case "refresh_positions":
		b.editPositions(cid, mid)
	default:
		switch {
		case strings.HasPrefix(cq.Data, "cfg:"):
			b.handleCfgCallback(cid, mid, cq.Data)
		case strings.HasPrefix(cq.Data, "close_now:"):
			mint := strings.TrimPrefix(cq.Data, "close_now:")
			_, _, exec, ok := b.ctx(cid)
			if ok && exec != nil {
				exec.Close(mint, models.ExitManual)
				b.editText(cid, mid, "⏹ Closing position <code>"+mint[:8]+"...</code>", nil)
			}
		case strings.HasPrefix(cq.Data, "view_pos:"):
			mint := strings.TrimPrefix(cq.Data, "view_pos:")
			_, port, _, ok := b.ctx(cid)
			if !ok || port == nil {
				b.send(cid, "No active bot.", nil)
				return
			}
			pos := port.OpenPositions()
			for _, p := range pos {
				if p.Mint == mint {
					held := time.Since(p.OpenedAt).Round(time.Second)
					txt := fmt.Sprintf(
						"📍 <b>POSITION</b>\n<code>%s</code>\nEntry: <b>%.4f SOL</b>\nScore: <b>%d</b>/100\nHeld: <b>%s</b>",
						mint[:8]+"...", p.EntryAmountSOL, p.Score, held,
					)
					b.send(cid, txt, nil)
					return
				}
			}
			b.send(cid, "Position not found (may have already closed).", nil)
		case strings.HasPrefix(cq.Data, "share:"):
			mint := strings.TrimPrefix(cq.Data, "share:")
			_, port, _, ok := b.ctx(cid)
			if !ok || port == nil {
				b.send(cid, "No active bot.", nil)
				return
			}
			closed, found := port.GetClosedByMint(mint)
			if !found {
				b.send(cid, "Trade not found — it may have been too long ago.", nil)
				return
			}
			go b.sendShareCard(cid, closed)
		}
	}
}

// ── Config +/- handler ────────────────────────────────────────────────────────

// handleCfgCallback handles "cfg:{field}:{dn|up}" callback data.
func (b *Bot) handleCfgCallback(cid int64, mid int, data string) {
	parts := strings.SplitN(data, ":", 3)
	if len(parts) != 3 {
		return
	}
	field, dir := parts[1], parts[2]
	cfg := b.getConfig(cid)

	switch field {
	case "pos":
		step := 0.05
		if dir == "up" {
			cfg.MaxPositionSOL = round2(min64(cfg.MaxPositionSOL+step, 5.0))
		} else {
			cfg.MaxPositionSOL = round2(max64(cfg.MaxPositionSOL-step, 0.01))
		}
	case "maxpos":
		if dir == "up" {
			if cfg.MaxPositions < 20 { cfg.MaxPositions++ }
		} else {
			if cfg.MaxPositions > 1 { cfg.MaxPositions-- }
		}
	case "sl":
		step := 0.05
		if dir == "up" {
			cfg.StopLossPercent = round2(min64(cfg.StopLossPercent+step, 0.90))
		} else {
			cfg.StopLossPercent = round2(max64(cfg.StopLossPercent-step, 0.05))
		}
	case "tp1":
		step := 0.5
		if dir == "up" {
			cfg.TakeProfit1x = round1(min64(cfg.TakeProfit1x+step, 10.0))
		} else {
			cfg.TakeProfit1x = round1(max64(cfg.TakeProfit1x-step, 1.2))
		}
	case "tp2":
		step := 0.5
		if dir == "up" {
			cfg.TakeProfit2x = round1(min64(cfg.TakeProfit2x+step, 20.0))
		} else {
			cfg.TakeProfit2x = round1(max64(cfg.TakeProfit2x-step, 1.5))
		}
	case "tp3":
		step := 1.0
		if dir == "up" {
			cfg.TakeProfit3x = round1(min64(cfg.TakeProfit3x+step, 50.0))
		} else {
			cfg.TakeProfit3x = round1(max64(cfg.TakeProfit3x-step, 2.0))
		}
	case "timeout":
		if dir == "up" {
			if cfg.TimeoutMinutes < 60 { cfg.TimeoutMinutes++ }
		} else {
			if cfg.TimeoutMinutes > 1 { cfg.TimeoutMinutes-- }
		}
	default:
		return
	}

	b.setConfig(cid, cfg)
	b.editConfig(cid, mid)
}

func min64(a, b float64) float64 { if a < b { return a }; return b }
func max64(a, b float64) float64 { if a > b { return a }; return b }
func round2(v float64) float64   { return float64(int(v*100+0.5)) / 100 }
func round1(v float64) float64   { return float64(int(v*10+0.5)) / 10 }

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
	cfg := b.getConfig(chatID)
	b.send(chatID, renderConfig(cfg), configKB(cfg))
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
	cfg := b.getConfig(cid)
	b.editText(cid, mid, renderConfig(cfg), configKB(cfg))
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
	sniperBtn := "Sniper ✅ ON"
	if !cfg.SniperEnabled {
		sniperBtn = "Sniper ❌ OFF"
	}
	scalperBtn := "Scalper ✅ ON"
	if !cfg.ScalperEnabled {
		scalperBtn = "Scalper ❌ OFF"
	}
	tp1 := cfg.TakeProfit1x; if tp1 <= 0 { tp1 = 2.0 }
	tp2 := cfg.TakeProfit2x; if tp2 <= 0 { tp2 = 5.0 }
	tp3 := cfg.TakeProfit3x; if tp3 <= 0 { tp3 = 10.0 }
	timeout := cfg.TimeoutMinutes; if timeout <= 0 { timeout = 8 }

	kb := tgbotapi.NewInlineKeyboardMarkup(
		// Toggles
		[]tgbotapi.InlineKeyboardButton{btn(sniperBtn, "sniper_toggle"), btn(scalperBtn, "scalper_toggle")},
		// Position size
		[]tgbotapi.InlineKeyboardButton{
			btn("─ Position", "cfg:pos:dn"),
			btn(fmt.Sprintf("%.2f SOL", cfg.MaxPositionSOL), "noop"),
			btn("Position +", "cfg:pos:up"),
		},
		// Max positions
		[]tgbotapi.InlineKeyboardButton{
			btn("─ Max pos", "cfg:maxpos:dn"),
			btn(fmt.Sprintf("%d slots", cfg.MaxPositions), "noop"),
			btn("Max pos +", "cfg:maxpos:up"),
		},
		// Stop loss
		[]tgbotapi.InlineKeyboardButton{
			btn("─ Stop loss", "cfg:sl:dn"),
			btn(fmt.Sprintf("%.0f%% SL", cfg.StopLossPercent*100), "noop"),
			btn("Stop loss +", "cfg:sl:up"),
		},
		// Take profit 1
		[]tgbotapi.InlineKeyboardButton{
			btn("─ TP1", "cfg:tp1:dn"),
			btn(fmt.Sprintf("%.1fx TP1", tp1), "noop"),
			btn("TP1 +", "cfg:tp1:up"),
		},
		// Take profit 2
		[]tgbotapi.InlineKeyboardButton{
			btn("─ TP2", "cfg:tp2:dn"),
			btn(fmt.Sprintf("%.1fx TP2", tp2), "noop"),
			btn("TP2 +", "cfg:tp2:up"),
		},
		// Take profit 3
		[]tgbotapi.InlineKeyboardButton{
			btn("─ TP3", "cfg:tp3:dn"),
			btn(fmt.Sprintf("%.1fx TP3", tp3), "noop"),
			btn("TP3 +", "cfg:tp3:up"),
		},
		// Timeout
		[]tgbotapi.InlineKeyboardButton{
			btn("─ Timeout", "cfg:timeout:dn"),
			btn(fmt.Sprintf("%dmin", timeout), "noop"),
			btn("Timeout +", "cfg:timeout:up"),
		},
		// Back
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

func (b *Bot) Notify(text string) {
	b.send(b.chatID, text, nil)
}

// sendShareCard generates a PnL card PNG and sends it to the chat with a tweet link.
// Runs in a goroutine — card generation may take 1-3 seconds.
func (b *Bot) sendShareCard(chatID int64, c *models.ClosedPosition) {
	pngBytes, err := pnl.GenerateCard(c)
	if err != nil {
		log.Printf("[bot] card generation failed: %v", err)
		b.send(chatID, "⚠️ Card generation failed — wkhtmltoimage may not be installed on the server.", nil)
		return
	}

	held := c.ClosedAt.Sub(c.OpenedAt).Round(time.Second)
	pnlSign := "+"
	if c.PnLSOL < 0 {
		pnlSign = ""
	}
	shortMint := c.Mint
	if len(shortMint) > 8 {
		shortMint = shortMint[:8]
	}

	tweetText := fmt.Sprintf("Just caught %s%.0f%% on $%s with @hummingbird_vylth 🐦\n\nTrade autonomously on Solana. No code. No babysitting.\nhummingbird.vylth.com",
		pnlSign, c.PnLPercent, shortMint)
	tweetURL := "https://twitter.com/intent/tweet?text=" + url.QueryEscape(tweetText)

	caption := fmt.Sprintf("🐦 <b>%s%.0f%%</b> on <code>%s</code>\n%s%.4f SOL · held %s\n\n<a href=\"%s\">Post to X →</a>",
		pnlSign, c.PnLPercent, shortMint,
		pnlSign, c.PnLSOL, held,
		tweetURL,
	)

	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileReader{
		Name:   "hb-trade.png",
		Reader: bytes.NewReader(pngBytes),
	})
	photo.Caption = caption
	photo.ParseMode = "HTML"
	if _, err := b.api.Send(photo); err != nil {
		log.Printf("[bot] sendPhoto: %v", err)
	}
}

// ── unused import guard ───────────────────────────────────────────────────────
var _ = fmt.Sprintf
