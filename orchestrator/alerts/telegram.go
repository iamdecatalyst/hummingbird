package alerts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/iamdecatalyst/hummingbird/orchestrator/eventlog"
	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
)

// Notifier is implemented by anything that can push trade notifications.
type Notifier interface {
	Entered(p *models.Position)
	Exited(c *models.ClosedPosition)
	Alert(text string)
	Notify(text string) // like Alert but without the ⚠️ prefix
}

type Telegram struct {
	token     string
	chatID    string
	channelID string // public broadcast channel (optional)
	client    *http.Client
	log       *eventlog.Log
}

func NewTelegram(token, chatID string) *Telegram {
	return &Telegram{
		token:  token,
		chatID: chatID,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (t *Telegram) WithChannel(channelID string) *Telegram {
	t.channelID = channelID
	return t
}

// WithLog attaches a per-user event log so trade events appear on the dashboard.
func (t *Telegram) WithLog(l *eventlog.Log) *Telegram {
	t.log = l
	return t
}

const div = "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

func shortMint(mint string) string {
	if len(mint) <= 12 {
		return mint
	}
	return mint[:6] + "..." + mint[len(mint)-4:]
}

func (t *Telegram) Entered(p *models.Position) {
	mode := "SNIPER"
	if p.Score < 60 {
		mode = "SCALPER"
	}

	msg := fmt.Sprintf(
		"⚡ <b>ENTERED</b> · <code>%s</code>\n%s\nToken:    <code>%s</code>\nScore:    <b>%d</b>/100 · %s\nPosition: <b>%.3f SOL</b>\n\n🕐 %s UTC",
		mode, div,
		shortMint(p.Mint),
		p.Score, p.Decision,
		p.EntryAmountSOL,
		p.OpenedAt.UTC().Format("15:04:05"),
	)

	kb := inlineKB([][]kbButton{
		{
			{Text: "📊 View Position", Data: "view_pos:" + p.Mint},
			{Text: "❌ Close Now", Data: "close_now:" + p.Mint},
		},
	})
	t.sendKB(t.chatID, msg, kb)

	// Public channel — no inline buttons
	if t.channelID != "" {
		pub := fmt.Sprintf(
			"⚡ <b>SNIPED</b> · <code>%s</code>\n%s\nToken: <code>%s</code>\nScore: <b>%d</b>/100 · %s\nEntry: <b>%.3f SOL</b>\n\n<a href=\"https://hummingbird.vylth.com\">hummingbird.vylth.com</a>",
			mode, div, shortMint(p.Mint), p.Score, p.Decision, p.EntryAmountSOL,
		)
		t.sendKB(t.channelID, pub, nil)
	}

	if t.log != nil {
		short := p.Mint
		if len(short) > 8 {
			short = short[:8]
		}
		t.log.Emit(eventlog.Event{
			Type:    "ENTER",
			Token:   p.Mint,
			AmtSOL:  p.EntryAmountSOL,
			Message: fmt.Sprintf("Entered %s…  %.3f SOL", short, p.EntryAmountSOL),
		})
	}
}

func (t *Telegram) Exited(c *models.ClosedPosition) {
	held := c.ClosedAt.Sub(c.OpenedAt).Round(time.Second)
	pnlSign := "+"
	if c.PnLSOL < 0 {
		pnlSign = ""
	}

	var header string
	switch {
	case c.PnLSOL > 0:
		header = fmt.Sprintf("✅ <b>EXITED</b> · <code>%s</code> · <b>%s%.4f SOL</b>", reasonLabel(c.Reason), pnlSign, c.PnLSOL)
	case c.Reason == models.ExitRugDetected:
		header = fmt.Sprintf("🪦 <b>EXITED</b> · <code>RUG DETECTED</code> · <b>%.4f SOL</b>", c.PnLSOL)
	default:
		header = fmt.Sprintf("🔴 <b>EXITED</b> · <code>%s</code> · <b>%.4f SOL</b>", reasonLabel(c.Reason), c.PnLSOL)
	}

	msg := fmt.Sprintf(
		"%s\n%s\nToken:    <code>%s</code>\nEntry:    <b>%.4f SOL</b>  →  Exit: <b>%.4f SOL</b>\nResult:   <b>%s%.4f SOL</b>  (<b>%s%.1f%%</b>)\nDuration: <b>%s</b>",
		header, div,
		shortMint(c.Mint),
		c.EntryAmountSOL, c.ExitAmountSOL,
		pnlSign, c.PnLSOL, pnlSign, c.PnLPercent,
		held,
	)

	var kb [][]kbButton
	if c.PnLSOL > 0 {
		kb = [][]kbButton{
			{
				{Text: "📤 Share Trade", Data: "share:" + c.TxHash},
				{Text: "📊 Dashboard", URL: "https://hummingbird.vylth.com/dashboard"},
			},
		}
	} else {
		kb = [][]kbButton{
			{
				{Text: "📊 Dashboard", URL: "https://hummingbird.vylth.com/dashboard"},
			},
		}
	}
	t.sendKB(t.chatID, msg, inlineKB(kb))

	// Public channel
	if t.channelID != "" {
		winEmoji := "🟢"
		if c.PnLSOL < 0 {
			winEmoji = "🔴"
		}
		pub := fmt.Sprintf(
			"%s <b>CLOSED %s%.0f%%</b>\n%s\nToken: <code>%s</code>\nP&amp;L: <b>%s%.4f SOL</b>\nHeld: %s\n\n<a href=\"https://hummingbird.vylth.com\">hummingbird.vylth.com</a>",
			winEmoji, pnlSign, c.PnLPercent, div,
			shortMint(c.Mint),
			pnlSign, c.PnLSOL,
			held,
		)
		t.sendKB(t.channelID, pub, nil)
	}

	if t.log != nil {
		short := c.Mint
		if len(short) > 8 {
			short = short[:8]
		}
		t.log.Emit(eventlog.Event{
			Type:    "EXIT",
			Token:   c.Mint,
			PnLSOL:  c.PnLSOL,
			PnLPct:  c.PnLPercent,
			Reason:  string(c.Reason),
			Message: fmt.Sprintf("Exited %s…  %+.4f SOL (%+.1f%%)", short, c.PnLSOL, c.PnLPercent),
		})
	}
}

func (t *Telegram) DailyStats(wins, losses int, pnl float64, winRate float64) {
	emoji := "📈"
	if pnl < 0 {
		emoji = "📉"
	}
	sign := "+"
	if pnl < 0 {
		sign = ""
	}
	msg := fmt.Sprintf(
		"%s <b>Daily Summary</b>\n%s\nP&amp;L:    <b>%s%.4f SOL</b>\nTrades:  <b>%d</b>  (W: %d  L: %d)\nWin rate: <b>%.0f%%</b>",
		emoji, div, sign, pnl, wins+losses, wins, losses, winRate,
	)
	t.sendKB(t.chatID, msg, nil)
}

func (t *Telegram) Alert(text string) {
	t.sendKB(t.chatID, "⚠️ "+text, nil)
	if t.log != nil {
		t.log.Emit(eventlog.Event{Type: "ALERT", Message: text})
	}
}

func (t *Telegram) Notify(text string) {
	t.sendKB(t.chatID, text, nil)
}

// ── Keyboard helpers ──────────────────────────────────────────────────────────

type kbButton struct {
	Text string
	Data string // callback_data (mutually exclusive with URL)
	URL  string // url button
}

func inlineKB(rows [][]kbButton) any {
	var result [][]map[string]string
	for _, row := range rows {
		var r []map[string]string
		for _, btn := range row {
			b := map[string]string{"text": btn.Text}
			if btn.URL != "" {
				b["url"] = btn.URL
			} else {
				b["callback_data"] = btn.Data
			}
			r = append(r, b)
		}
		result = append(result, r)
	}
	return map[string]any{"inline_keyboard": result}
}

// ── Send helpers ──────────────────────────────────────────────────────────────

func (t *Telegram) sendKB(chatID, text string, replyMarkup any) {
	if t.token == "" || chatID == "" {
		log.Printf("[telegram] %s", text)
		return
	}
	payload := map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
	if replyMarkup != nil {
		payload["reply_markup"] = replyMarkup
	}
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
	resp, err := t.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[telegram] send error: %v", err)
		return
	}
	defer resp.Body.Close()
}

func reasonLabel(r models.ExitReason) string {
	switch r {
	case models.ExitTakeProfit:
		return "Take Profit"
	case models.ExitStopLoss:
		return "Stop Loss"
	case models.ExitRugDetected:
		return "Rug Detected"
	case models.ExitTimeout:
		return "Timeout"
	case models.ExitManual:
		return "Manual"
	default:
		return string(r)
	}
}
