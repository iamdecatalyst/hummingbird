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
}

type Telegram struct {
	token  string
	chatID string
	client *http.Client
	log    *eventlog.Log
}

func NewTelegram(token, chatID string) *Telegram {
	return &Telegram{
		token:  token,
		chatID: chatID,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// WithLog attaches a per-user event log so trade events appear on the dashboard.
func (t *Telegram) WithLog(l *eventlog.Log) *Telegram {
	t.log = l
	return t
}

func (t *Telegram) Entered(p *models.Position) {
	msg := fmt.Sprintf(
		"🐦 *ENTERED*\n`%s`\nScore: %d/100 | Position: %.3f SOL\nTime: %s",
		p.Mint, p.Score, p.EntryAmountSOL,
		p.OpenedAt.Format("15:04:05"),
	)
	t.send(msg)
	if t.log != nil {
		short := p.Mint
		if len(short) > 8 {
			short = short[:8]
		}
		t.log.Emit(eventlog.Event{
			Type:   "ENTER",
			Token:  p.Mint,
			AmtSOL: p.EntryAmountSOL,
			Message: fmt.Sprintf("Entered %s…  %.3f SOL", short, p.EntryAmountSOL),
		})
	}
}

func (t *Telegram) Exited(c *models.ClosedPosition) {
	emoji := "✅"
	if c.PnLSOL < 0 {
		emoji = "❌"
	}
	msg := fmt.Sprintf(
		"%s *EXITED* — %s\n`%s`\nEntry: %.4f SOL → Exit: %.4f SOL\nP&L: %+.4f SOL (%+.1f%%)\nReason: %s | Held: %s",
		emoji,
		reasonLabel(c.Reason),
		c.Mint,
		c.EntryAmountSOL, c.ExitAmountSOL,
		c.PnLSOL, c.PnLPercent,
		c.Reason,
		time.Since(c.OpenedAt).Round(time.Second),
	)
	t.send(msg)
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
	msg := fmt.Sprintf(
		"%s *Daily Summary*\nP&L: %+.4f SOL\nTrades: %d (W:%d L:%d) | Win rate: %.0f%%",
		emoji, pnl, wins+losses, wins, losses, winRate,
	)
	t.send(msg)
}

func (t *Telegram) Alert(text string) {
	t.send("⚠️ " + text)
	if t.log != nil {
		t.log.Emit(eventlog.Event{Type: "ALERT", Message: text})
	}
}

func (t *Telegram) send(text string) {
	if t.token == "" || t.chatID == "" {
		log.Printf("[telegram] %s", text)
		return
	}

	body, _ := json.Marshal(map[string]any{
		"chat_id":    t.chatID,
		"text":       text,
		"parse_mode": "Markdown",
	})

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
		return "RUG DETECTED"
	case models.ExitTimeout:
		return "Timeout"
	case models.ExitManual:
		return "Manual"
	default:
		return string(r)
	}
}
