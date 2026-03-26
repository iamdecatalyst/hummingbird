package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/iamdecatalyst/hummingbird/cli/client"
)

// tradesMsg is sent when the positions fetch completes.
type tradesMsg struct {
	open   []client.Position
	closed []client.ClosedPosition
	err    error
}

// TradesModel is the trades/positions tab view.
type TradesModel struct {
	spinner spinner.Model
	client  *client.Client
	open    []client.Position
	closed  []client.ClosedPosition
	err     error
	loading bool
	fetched bool
}

// NewTradesModel creates a new trades view.
func NewTradesModel(c *client.Client) TradesModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(ColorAccent)

	return TradesModel{
		spinner: sp,
		client:  c,
	}
}

func (m TradesModel) Init() tea.Cmd {
	return nil
}

// Fetch starts a positions fetch. Called when the tab is activated.
func (m TradesModel) Fetch() (TradesModel, tea.Cmd) {
	m.loading = true
	m.err = nil
	c := m.client
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		open, err := c.GetPositions()
		if err != nil {
			return tradesMsg{err: err}
		}
		closed, err := c.GetClosed()
		if err != nil {
			return tradesMsg{err: err}
		}
		return tradesMsg{open: open, closed: closed}
	})
}

func (m TradesModel) Update(msg tea.Msg) (TradesModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "r" && !m.loading {
			return m.Fetch()
		}

	case tradesMsg:
		m.loading = false
		m.fetched = true
		m.open = msg.open
		m.closed = msg.closed
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

func (m TradesModel) View() string {
	var b strings.Builder

	if m.loading {
		b.WriteString("  " + m.spinner.View() + StyleMuted.Render("  fetching positions..."))
		return b.String()
	}

	if m.err != nil {
		b.WriteString("  " + StyleError.Render("✗ "+m.err.Error()))
		return b.String()
	}

	if !m.fetched {
		b.WriteString("  " + StyleMuted.Render("loading..."))
		return b.String()
	}

	b.WriteString(renderTrades(m.open, m.closed))

	return b.String()
}

func shortMint(mint string) string {
	if len(mint) <= 12 {
		return mint
	}
	return mint[:6] + "..." + mint[len(mint)-4:]
}

func heldDuration(openedAt string) string {
	t, err := time.Parse(time.RFC3339, openedAt)
	if err != nil {
		// Try common formats
		t, err = time.Parse("2006-01-02T15:04:05Z", openedAt)
		if err != nil {
			return openedAt
		}
	}
	dur := time.Since(t)
	if dur < time.Minute {
		return fmt.Sprintf("%ds", int(dur.Seconds()))
	}
	if dur < time.Hour {
		return fmt.Sprintf("%dm", int(dur.Minutes()))
	}
	if dur < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(dur.Hours()), int(dur.Minutes())%60)
	}
	return fmt.Sprintf("%dd", int(dur.Hours()/24))
}

func renderTrades(open []client.Position, closed []client.ClosedPosition) string {
	var b strings.Builder

	// Open positions table
	openHeader := fmt.Sprintf("  %s%s%s%s%s",
		StyleMuted.Width(14).Render("MINT"),
		StyleMuted.Width(8).Render("SCORE"),
		StyleMuted.Width(16).Render("ENTRY (SOL)"),
		StyleMuted.Width(14).Render("AMOUNT"),
		StyleMuted.Width(12).Render("HELD"),
	)
	openDivider := "  " + StyleDivider.Render(strings.Repeat("─", 64))

	b.WriteString(StyleTitle.Render(fmt.Sprintf("Open Positions (%d)", len(open))))
	b.WriteString("\n\n")
	b.WriteString(openHeader + "\n")
	b.WriteString(openDivider + "\n")

	if len(open) == 0 {
		b.WriteString("  " + StyleMuted.Render("No open positions") + "\n")
	} else {
		for _, p := range open {
			row := fmt.Sprintf("  %s%s%s%s%s",
				StyleValue.Width(14).Render(shortMint(p.Mint)),
				StyleValue.Width(8).Render(fmt.Sprintf("%d", p.Score)),
				StyleValue.Width(16).Render(fmt.Sprintf("%.6f", p.EntryPriceSOL)),
				StyleValue.Width(14).Render(fmt.Sprintf("%.4f", p.EntryAmountSOL)),
				StyleMuted.Width(12).Render(heldDuration(p.OpenedAt)),
			)
			b.WriteString(row + "\n")
		}
	}

	b.WriteString("\n")

	// Recent closed table
	closedHeader := fmt.Sprintf("  %s%s%s%s",
		StyleMuted.Width(14).Render("MINT"),
		StyleMuted.Width(14).Render("P&L (SOL)"),
		StyleMuted.Width(10).Render("%"),
		StyleMuted.Width(14).Render("REASON"),
	)
	closedDivider := "  " + StyleDivider.Render(strings.Repeat("─", 52))

	limit := 20
	if len(closed) < limit {
		limit = len(closed)
	}
	recentClosed := closed
	if len(recentClosed) > limit {
		recentClosed = recentClosed[len(recentClosed)-limit:]
	}

	b.WriteString(StyleTitle.Render(fmt.Sprintf("Recent Closed (%d shown)", limit)))
	b.WriteString("\n\n")
	b.WriteString(closedHeader + "\n")
	b.WriteString(closedDivider + "\n")

	if len(recentClosed) == 0 {
		b.WriteString("  " + StyleMuted.Render("No closed positions") + "\n")
	} else {
		// Show newest first
		for i := len(recentClosed) - 1; i >= 0; i-- {
			p := recentClosed[i]
			pnlStr := fmt.Sprintf("%+.4f", p.PnLSOL)
			pctStr := fmt.Sprintf("%+.1f%%", p.PnLPercent)
			pnlStyle := PnLStyle(p.PnLSOL)

			row := fmt.Sprintf("  %s%s%s%s",
				StyleValue.Width(14).Render(shortMint(p.Mint)),
				pnlStyle.Width(14).Render(pnlStr),
				pnlStyle.Width(10).Render(pctStr),
				StyleMuted.Width(14).Render(p.Reason),
			)
			b.WriteString(row + "\n")
		}
	}

	return b.String()
}

// PositionsOnce runs a one-shot positions fetch and returns a formatted string.
func PositionsOnce(c *client.Client) (string, error) {
	open, err := c.GetPositions()
	if err != nil {
		return "", err
	}
	closed, err := c.GetClosed()
	if err != nil {
		return "", err
	}
	return renderTrades(open, closed), nil
}

var _ = lipgloss.NewStyle // suppress unused import
