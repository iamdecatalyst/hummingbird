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

// NewOverviewModel creates a new overview view.
func NewOverviewModel(c *client.Client) OverviewModel {
	sp := spinner.New()
	sp.Spinner = spinner.Points
	sp.Style = lipgloss.NewStyle().Foreground(ColorAccent)

	return OverviewModel{
		spinner: sp,
		client:  c,
	}
}

func (m OverviewModel) Init() tea.Cmd {
	return nil
}

// Fetch starts a stats fetch. Called when the tab is activated.
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

	b.WriteString("  " + StyleTitle.Render("◎ Overview"))
	b.WriteString("\n\n")

	if m.loading {
		b.WriteString("  " + m.spinner.View() + StyleMuted.Render("  Fetching stats..."))
		return b.String()
	}

	if m.err != nil {
		b.WriteString("  " + StyleError.Render("✗ "+m.err.Error()))
		b.WriteString("\n\n  " + StyleMuted.Render("Press r to retry"))
		return b.String()
	}

	if m.stats != nil {
		b.WriteString(renderOverview(m.stats))
		b.WriteString("\n\n  " + StyleHelp.Render("Press r to refresh"))
	} else {
		b.WriteString("  " + StyleMuted.Render("Loading..."))
	}

	return b.String()
}

func renderOverview(s *client.Stats) string {
	// Status line
	var statusLabel, statusDot string
	if s.Paused {
		statusDot = StyleYellow.Render("●")
		statusLabel = StyleYellow.Bold(true).Render("PAUSED")
		if s.PauseReason != "" {
			statusLabel += "  " + StyleMuted.Render(s.PauseReason)
		}
	} else if !s.Configured {
		statusDot = StyleRed.Render("●")
		statusLabel = StyleRed.Bold(true).Render("NOT CONFIGURED")
	} else {
		statusDot = StyleGreen.Render("●")
		statusLabel = StyleGreen.Bold(true).Render("LIVE")
	}
	statusLine := fmt.Sprintf("  %s  %s", statusDot, statusLabel)

	// Stats grid
	colW := 20

	todayPnL := fmt.Sprintf("%.4f SOL", s.TodayPnL)
	totalPnL := fmt.Sprintf("%.4f SOL", s.TotalPnL)
	winRate := fmt.Sprintf("%.1f%%", s.WinRate)
	openPos := fmt.Sprintf("%d", s.OpenPositions)

	todayStyle := PnLStyle(s.TodayPnL)
	totalStyle := PnLStyle(s.TotalPnL)

	row1Labels := fmt.Sprintf("  %s%s%s%s",
		StyleMuted.Width(colW).Render("TODAY P&L"),
		StyleMuted.Width(colW).Render("TOTAL P&L"),
		StyleMuted.Width(colW).Render("WIN RATE"),
		StyleMuted.Width(colW).Render("OPEN POSITIONS"),
	)
	row1Values := fmt.Sprintf("  %s%s%s%s",
		todayStyle.Width(colW).Render(todayPnL),
		totalStyle.Width(colW).Render(totalPnL),
		StyleValue.Width(colW).Render(winRate),
		StyleValue.Width(colW).Render(openPos),
	)

	divider := "  " + StyleDivider.Render(strings.Repeat("─", colW*4))

	// Trade counts
	winsLabel := StyleMuted.Render("Wins: ")
	winsVal := StyleGreen.Render(fmt.Sprintf("%d", s.Wins))
	lossLabel := StyleMuted.Render("  Losses: ")
	lossVal := StyleRed.Render(fmt.Sprintf("%d", s.Losses))
	totalLabel := StyleMuted.Render("  Total Trades: ")
	totalVal := StyleValue.Render(fmt.Sprintf("%d", s.TotalTrades))

	tradeLine := "  " + winsLabel + winsVal + lossLabel + lossVal + totalLabel + totalVal

	grid := row1Labels + "\n" + row1Values + "\n\n" + divider + "\n" + tradeLine

	content := statusLine + "\n\n" + grid

	return StyleBox.Render(content)
}

// OverviewOnce runs a one-shot stats fetch and returns a formatted string.
func OverviewOnce(c *client.Client) (string, error) {
	stats, err := c.GetStats()
	if err != nil {
		return "", err
	}
	return renderOverview(stats), nil
}

// statsStyle returns appropriate style for a lipgloss width-formatted value.
func statsStyle(val float64) lipgloss.Style {
	return PnLStyle(val)
}
