package bot

// render.go — all ASCII/HTML message templates
// Follows Raft style: box drawing chars, ━ separators, <pre> tables, HTML parse mode

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
	"github.com/iamdecatalyst/hummingbird/orchestrator/portfolio"
)

const (
	divider = "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	boxTop  = "┌─────────────────────────────────────┐"
	boxMid  = "├─────────────────────────────────────┤"
	boxBot  = "└─────────────────────────────────────┘"
)

func row(label, value string) string {
	return fmt.Sprintf("│ <b>%-18s</b> <code>%s</code>\n", label, value)
}

// ── Main Menu ─────────────────────────────────────────────────────────────────

func renderMain(stats portfolio.Stats, running bool) string {
	status := "🟢 <b>LIVE</b>   Watching 4 platforms"
	if !running {
		status = "🔴 <b>PAUSED</b>"
	}
	if stats.Paused {
		status = fmt.Sprintf("⏸  <b>PAUSED</b>  %s", stats.PauseReason)
	}

	pnlSign := "+"
	if stats.TodayPnL < 0 {
		pnlSign = ""
	}
	totalSign := "+"
	if stats.TotalPnL < 0 {
		totalSign = ""
	}

	return strings.Join([]string{
		"🐦 <b>HUMMINGBIRD</b>",
		divider,
		"",
		status,
		"",
		boxTop,
		row("Today P&L", fmt.Sprintf("%s%.4f SOL", pnlSign, stats.TodayPnL)),
		row("Total P&L", fmt.Sprintf("%s%.4f SOL", totalSign, stats.TotalPnL)),
		row("Open positions", fmt.Sprintf("%d", stats.OpenPositions)),
		row("Win rate", fmt.Sprintf("%.0f%%  (%d trades)", stats.WinRate, stats.TotalTrades)),
		boxBot,
		"",
		fmt.Sprintf("<i>%s</i>", time.Now().Format("2006-01-02 15:04:05 UTC")),
	}, "\n")
}

// ── Stats ─────────────────────────────────────────────────────────────────────

func renderStats(stats portfolio.Stats, recent []*models.ClosedPosition) string {
	todaySign := "+"
	if stats.TodayPnL < 0 {
		todaySign = ""
	}
	totalSign := "+"
	if stats.TotalPnL < 0 {
		totalSign = ""
	}

	header := strings.Join([]string{
		"📊 <b>PERFORMANCE</b>",
		divider,
		"",
		boxTop,
		row("TODAY P&L", fmt.Sprintf("%s%.4f SOL", todaySign, stats.TodayPnL)),
		row("Today trades", fmt.Sprintf("%d  (W:%d  L:%d)", stats.TotalTrades, stats.Wins, stats.Losses)),
		row("Win rate", fmt.Sprintf("%.0f%%", stats.WinRate)),
		boxMid,
		row("TOTAL P&L", fmt.Sprintf("%s%.4f SOL", totalSign, stats.TotalPnL)),
		row("All trades", fmt.Sprintf("%d  (W:%d  L:%d)", stats.TotalTrades, stats.Wins, stats.Losses)),
		boxBot,
	}, "\n")

	if len(recent) == 0 {
		return header + "\n\n<i>No trades yet.</i>"
	}

	table := "<pre>\nMODE   TOKEN    ENTRY    EXIT     P&L\n" + strings.Repeat("─", 38) + "\n"
	for _, c := range recent {
		mode := "snipe"
		if c.Reason == "scalp" {
			mode = "scalp"
		}
		sign := "+"
		if c.PnLPercent < 0 {
			sign = ""
		}
		mint := c.Mint
		if len(mint) > 8 {
			mint = mint[:8]
		}
		table += fmt.Sprintf(
			"%-6s %-8s %-8s %-8s %s%.0f%%\n",
			mode, mint,
			fmt.Sprintf("%.3f", c.EntryAmountSOL),
			fmt.Sprintf("%.3f", c.ExitAmountSOL),
			sign, c.PnLPercent,
		)
	}
	table += "</pre>"

	return header + "\n\n🗂 <b>RECENT TRADES</b>\n" + table
}

// ── Positions ─────────────────────────────────────────────────────────────────

func renderPositions(positions []*models.Position) string {
	if len(positions) == 0 {
		return strings.Join([]string{
			"📍 <b>OPEN POSITIONS</b>",
			divider,
			"",
			"<i>No open positions.</i>",
			"",
			"Bot is scanning for entries...",
		}, "\n")
	}

	header := fmt.Sprintf(
		"📍 <b>OPEN POSITIONS (%d)</b>\n%s\n",
		len(positions), divider,
	)

	table := "<pre>\nTOKEN    MODE   ENTRY    P&L\n" + strings.Repeat("─", 36) + "\n"
	for _, p := range positions {
		held := time.Since(p.OpenedAt).Round(time.Second)
		mint := p.Mint
		if len(mint) > 8 {
			mint = mint[:8]
		}
		table += fmt.Sprintf(
			"%-8s %-6s %-8s %s\n",
			mint, "snipe",
			fmt.Sprintf("%.3f", p.EntryAmountSOL),
			held,
		)
	}
	table += "</pre>"

	return header + table
}

// ── Config ────────────────────────────────────────────────────────────────────

func renderConfig(cfg BotConfig) string {
	sniperStatus := statusToggle(cfg.SniperEnabled)
	scalperStatus := statusToggle(cfg.ScalperEnabled)
	riskLabel, riskEmoji := riskDisplay(cfg.MaxPositionSOL)
	tp1 := cfg.TakeProfit1x
	if tp1 <= 0 { tp1 = 2.0 }
	tp2 := cfg.TakeProfit2x
	if tp2 <= 0 { tp2 = 5.0 }
	tp3 := cfg.TakeProfit3x
	if tp3 <= 0 { tp3 = 10.0 }
	timeout := cfg.TimeoutMinutes
	if timeout <= 0 { timeout = 8 }

	return strings.Join([]string{
		"⚙️ <b>CONFIGURATION</b>",
		divider,
		"",
		boxTop,
		row("Sniper", sniperStatus),
		row("Scalper", scalperStatus),
		boxMid,
		row("Risk level", fmt.Sprintf("%s  %s", riskEmoji, riskLabel)),
		row("Max position", fmt.Sprintf("%.2f SOL", cfg.MaxPositionSOL)),
		row("Max concurrent", fmt.Sprintf("%d positions", cfg.MaxPositions)),
		row("Stop loss", fmt.Sprintf("%.0f%%", cfg.StopLossPercent*100)),
		row("Daily loss limit", fmt.Sprintf("%.0f%%", cfg.DailyLossLimit*100)),
		boxMid,
		row("Take profit 1", fmt.Sprintf("%.1fx  → sell 40%%", tp1)),
		row("Take profit 2", fmt.Sprintf("%.1fx  → sell 40%%", tp2)),
		row("Take profit 3", fmt.Sprintf("%.1fx  → sell rest", tp3)),
		row("Timeout", fmt.Sprintf("%d min", timeout)),
		boxBot,
		"",
		"<i>Use ± buttons below to adjust values.</i>",
	}, "\n")
}

// ── Alert / entry / exit (outbound) ──────────────────────────────────────────

func renderEntered(p *models.Position) string {
	mode := "SNIPER"
	if p.Score < 60 {
		mode = "SCALPER"
	}
	return strings.Join([]string{
		fmt.Sprintf("🐦 <b>ENTERED [%s]</b>", mode),
		divider,
		"",
		boxTop,
		row("Token", truncate(p.Mint, 16)),
		row("Score", fmt.Sprintf("%d / 100", p.Score)),
		row("Position", fmt.Sprintf("%.3f SOL", p.EntryAmountSOL)),
		row("Time", p.OpenedAt.Format("15:04:05")),
		boxBot,
	}, "\n")
}

func renderExited(c *models.ClosedPosition) string {
	emoji := "✅"
	if c.PnLSOL < 0 {
		emoji = "❌"
	}
	sign := "+"
	if c.PnLSOL < 0 {
		sign = ""
	}
	held := c.ClosedAt.Sub(c.OpenedAt).Round(time.Second)

	return strings.Join([]string{
		fmt.Sprintf("%s <b>EXITED — %s</b>", emoji, strings.ToUpper(string(c.Reason))),
		divider,
		"",
		boxTop,
		row("Token", truncate(c.Mint, 16)),
		row("Entry", fmt.Sprintf("%.4f SOL", c.EntryAmountSOL)),
		row("Exit", fmt.Sprintf("%.4f SOL", c.ExitAmountSOL)),
		row("P&L", fmt.Sprintf("%s%.4f SOL  (%s%.1f%%)", sign, c.PnLSOL, sign, c.PnLPercent)),
		row("Held", held.String()),
		boxBot,
	}, "\n")
}

func renderDailyStats(stats portfolio.Stats) string {
	emoji := "📈"
	if stats.TodayPnL < 0 {
		emoji = "📉"
	}
	sign := "+"
	if stats.TodayPnL < 0 {
		sign = ""
	}

	return strings.Join([]string{
		fmt.Sprintf("%s <b>DAILY SUMMARY</b>", emoji),
		divider,
		"",
		boxTop,
		row("P&L", fmt.Sprintf("%s%.4f SOL", sign, stats.TodayPnL)),
		row("Trades", fmt.Sprintf("%d  (W:%d  L:%d)", stats.TotalTrades, stats.Wins, stats.Losses)),
		row("Win rate", fmt.Sprintf("%.0f%%", stats.WinRate)),
		boxBot,
	}, "\n")
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func statusToggle(on bool) string {
	if on {
		return "ON  ✅"
	}
	return "OFF ❌"
}

func riskDisplay(maxPos float64) (string, string) {
	switch {
	case maxPos <= 0.05:
		return "Low", "🟢"
	case maxPos <= 0.10:
		return "Medium", "🟡"
	default:
		return "High", "🔴"
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// BotConfig holds the current user-configurable settings.
type BotConfig struct {
	SniperEnabled   bool
	ScalperEnabled  bool
	MaxPositionSOL  float64
	MaxPositions    int
	StopLossPercent float64 // 0.25 = 25%
	DailyLossLimit  float64 // 0.30 = 30%
	TakeProfit1x    float64 // price multiple, e.g. 2.0
	TakeProfit2x    float64
	TakeProfit3x    float64
	TimeoutMinutes  int
	MinBalanceSOL   float64
}

var _ = math.Abs // avoid unused import
