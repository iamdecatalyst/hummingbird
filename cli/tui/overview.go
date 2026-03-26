package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/iamdecatalyst/hummingbird/cli/client"
)

// overviewMsg is sent when the stats fetch completes.
type overviewMsg struct {
	stats *client.Stats
	err   error
}

// OverviewModel is the overview tab view.
type OverviewModel struct {
	spinner spinner.Model
	client  *client.Client
	stats   *client.Stats
	err     error
	loading bool
	fetched bool
}

func NewOverviewModel(c *client.Client) OverviewModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(ColorAccent)
	return OverviewModel{spinner: sp, client: c}
}

func (m OverviewModel) Init() tea.Cmd { return nil }

func (m OverviewModel) Fetch() (OverviewModel, tea.Cmd) {
	m.loading = true
	m.err = nil
	c := m.client
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		stats, err := c.GetStats()
		return overviewMsg{stats: stats, err: err}
	})
}

func (m OverviewModel) Update(msg tea.Msg) (OverviewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "r" && !m.loading {
			return m.Fetch()
		}
	case overviewMsg:
		m.loading = false
		m.fetched = true
		m.stats = msg.stats
		m.err = msg.err
		return m, nil
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m OverviewModel) View() string {
	var b strings.Builder

	if m.loading && !m.fetched {
		b.WriteString("  " + m.spinner.View() + StyleMuted.Render("  connecting..."))
		return b.String()
	}

	if m.err != nil {
		b.WriteString("  " + StyleError.Render("✗  " + m.err.Error()))
		return b.String()
	}

	if m.stats != nil {
		if m.loading {
			b.WriteString("  " + m.spinner.View() + "\n\n")
		}
		b.WriteString(renderOverview(m.stats))
	} else {
		b.WriteString("  " + StyleMuted.Render("loading..."))
	}

	return b.String()
}

func renderOverview(s *client.Stats) string {
	var b strings.Builder

	// ── Status ──────────────────────────────────────────
	var dot, statusText string
	if !s.Configured {
		dot = StyleRed.Render("●")
		statusText = StyleRed.Bold(true).Render("NOT CONFIGURED")
	} else if s.Paused {
		dot = StyleYellow.Render("●")
		statusText = StyleYellow.Bold(true).Render("PAUSED")
		if s.PauseReason != "" {
			statusText += "  " + StyleMuted.Render("· " + s.PauseReason)
		}
	} else {
		dot = StyleGreen.Render("●")
		statusText = StyleGreen.Bold(true).Render("LIVE")
	}
	balanceStr := fmt.Sprintf("%.4f SOL", s.WalletBalanceSOL)
	b.WriteString(fmt.Sprintf("  %s  %s    %s\n\n", dot, statusText,
		StyleMuted.Render("wallet  ")+StyleValue.Bold(true).Render(balanceStr),
	))

	// ── Metrics grid ────────────────────────────────────
	w := 22

	todayStr := fmt.Sprintf("%.4f SOL", s.TodayPnL)
	totalStr := fmt.Sprintf("%.4f SOL", s.TotalPnL)
	winRateStr := fmt.Sprintf("%.1f%%", s.WinRate)
	openStr := fmt.Sprintf("%d", s.OpenPositions)

	labels := fmt.Sprintf("  %s%s%s%s",
		StyleMuted.Width(w).Render("TODAY P&L"),
		StyleMuted.Width(w).Render("TOTAL P&L"),
		StyleMuted.Width(w).Render("WIN RATE"),
		StyleMuted.Width(w).Render("POSITIONS"),
	)
	values := fmt.Sprintf("  %s%s%s%s",
		PnLStyle(s.TodayPnL).Width(w).Bold(true).Render(todayStr),
		PnLStyle(s.TotalPnL).Width(w).Bold(true).Render(totalStr),
		StyleValue.Width(w).Bold(true).Render(winRateStr),
		StyleValue.Width(w).Bold(true).Render(openStr),
	)

	b.WriteString(labels + "\n")
	b.WriteString(values + "\n")
	b.WriteString("\n  " + StyleDivider.Render(strings.Repeat("─", w*4-2)) + "\n\n")

	// ── Trade counters ───────────────────────────────────
	b.WriteString(fmt.Sprintf("  %s%s  %s%s  %s%s",
		StyleMuted.Render("wins "),   StyleGreen.Bold(true).Render(fmt.Sprintf("%d", s.Wins)),
		StyleMuted.Render("losses "), StyleRed.Bold(true).Render(fmt.Sprintf("%d", s.Losses)),
		StyleMuted.Render("total "),  StyleValue.Bold(true).Render(fmt.Sprintf("%d", s.TotalTrades)),
	))
	b.WriteString("\n")

	return b.String()
}

// OverviewOnce runs a one-shot stats fetch and returns a formatted string.
func OverviewOnce(c *client.Client) (string, error) {
	stats, err := c.GetStats()
	if err != nil {
		return "", err
	}
	return renderOverview(stats), nil
}

// suppress unused
var _ = lipgloss.NewStyle
