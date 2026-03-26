package alerts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
)

type Telegram struct {
	token  string
	chatID string
	client *http.Client
}

func NewTelegram(token, chatID string) *Telegram {
	return &Telegram{
		token:  token,
		chatID: chatID,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (t *Telegram) Entered(p *models.Position) {
	msg := fmt.Sprintf(
		"🐦 *ENTERED*\n`%s`\nScore: %d/100 | Position: %.3f SOL\nTime: %s",
		p.Mint, p.Score, p.EntryAmountSOL,
		p.OpenedAt.Format("15:04:05"),
	)
	t.send(msg)
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
