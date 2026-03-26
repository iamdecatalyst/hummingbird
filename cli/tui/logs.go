package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/iamdecatalyst/hummingbird/cli/client"
)

type logsMsg struct {
	logs []client.LogEntry
	err  error
}

type LogsModel struct {
	spinner   spinner.Model
	client    *client.Client
	logs      []client.LogEntry
	err       error
	loading   bool
	fetched   bool
	scrollOff int
}

func NewLogsModel(c *client.Client) LogsModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(ColorAccent)
	return LogsModel{spinner: sp, client: c}
}

func (m LogsModel) Init() tea.Cmd { return nil }

func (m LogsModel) Fetch() (LogsModel, tea.Cmd) {
	m.loading = true
	m.err = nil
	c := m.client
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		logs, err := c.GetLogs()
		return logsMsg{logs: logs, err: err}
	})
}

func (m LogsModel) Update(msg tea.Msg) (LogsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			if !m.loading {
				return m.Fetch()
			}
		case "up", "k":
			if m.scrollOff < len(m.logs)-1 {
				m.scrollOff++
			}
		case "down", "j":
			if m.scrollOff > 0 {
				m.scrollOff--
			}
		case "g":
			m.scrollOff = 0
		case "G":
			if len(m.logs) > 50 {
				m.scrollOff = len(m.logs) - 50
			}
		}
	case logsMsg:
		m.loading = false
		m.fetched = true
		m.logs = msg.logs
		m.err = msg.err
		m.scrollOff = 0
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

func (m LogsModel) View() string {
	var b strings.Builder

	if m.loading && !m.fetched {
		b.WriteString("  " + m.spinner.View() + StyleMuted.Render("  loading event log..."))
		return b.String()
	}

	if m.err != nil {
		b.WriteString("  " + StyleError.Render("✗  " + m.err.Error()))
		return b.String()
	}

	b.WriteString(renderLogs(m.logs, m.scrollOff))

	return b.String()
}

var logTypeGlyph = map[string]string{
	"ENTER": "▶",
	"EXIT":  "◀",
	"START": "▲",
	"STOP":  "▼",
	"ALERT": "!",
	"INFO":  "·",
}

func logEntryStyle(t string) lipgloss.Style {
	switch strings.ToUpper(t) {
	case "ENTER", "START":
		return StyleGreen
	case "EXIT":
		return StyleAccent
	case "STOP":
		return StyleYellow
	case "ALERT":
		return StyleRed
	default:
		return StyleMuted
	}
}

func renderLogEntry(e client.LogEntry) string {
	upper := strings.ToUpper(e.Type)
	style := logEntryStyle(upper)

	glyph, ok := logTypeGlyph[upper]
	if !ok {
		glyph = "·"
	}

	tag := style.Bold(true).Render(fmt.Sprintf("%s %-5s", glyph, upper))
	ts  := StyleMuted.Render(e.Time)

	var extra strings.Builder
	if e.Token != "" {
		extra.WriteString("  " + StyleMuted.Render("token=") + StyleValue.Render(shortMint(e.Token)))
	}
	if e.AmtSOL != 0 {
		extra.WriteString("  " + StyleMuted.Render("amt=") + StyleValue.Render(fmt.Sprintf("%.4f◎", e.AmtSOL)))
	}
	if e.PnLSOL != 0 {
		extra.WriteString("  " + StyleMuted.Render("pnl=") + PnLStyle(e.PnLSOL).Bold(true).Render(fmt.Sprintf("%+.4f◎", e.PnLSOL)))
	}
	if e.PnLPct != 0 {
		extra.WriteString(PnLStyle(e.PnLPct).Render(fmt.Sprintf(" (%+.1f%%)", e.PnLPct)))
	}
	if e.Reason != "" {
		extra.WriteString("  " + StyleMuted.Render("·") + " " + StyleValue.Render(e.Reason))
	}

	msg := ""
	if e.Message != "" {
		msg = "  " + StyleMuted.Render(e.Message)
	}

	return fmt.Sprintf("  %s  %s%s%s", ts, tag, extra.String(), msg)
}

func renderLogs(logs []client.LogEntry, scrollOff int) string {
	if len(logs) == 0 {
		return StyleBox.Render("  " + StyleMuted.Render("no events yet"))
	}

	const maxDisplay = 50

	// Reverse so newest is at top
	reversed := make([]client.LogEntry, len(logs))
	for i, e := range logs {
		reversed[len(logs)-1-i] = e
	}

	start := scrollOff
	end := scrollOff + maxDisplay
	if end > len(reversed) {
		end = len(reversed)
	}
	if start > len(reversed) {
		start = len(reversed)
	}

	var b strings.Builder

	// Header row
	b.WriteString(fmt.Sprintf("  %s  %s\n",
		StyleMuted.Width(19).Render("TIME"),
		StyleMuted.Render("EVENT"),
	))
	b.WriteString("  " + StyleDivider.Render(strings.Repeat("─", 72)) + "\n")

	for _, e := range reversed[start:end] {
		b.WriteString(renderLogEntry(e) + "\n")
	}

	if len(logs) > maxDisplay {
		b.WriteString("\n  " + StyleMuted.Render(fmt.Sprintf(
			"%d–%d of %d  ·  ↑↓ to scroll", start+1, end, len(logs),
		)))
	}

	return StyleBox.Render(b.String())
}

// LogsOnce runs a one-shot logs fetch and returns a formatted string.
func LogsOnce(c *client.Client) (string, error) {
	logs, err := c.GetLogs()
	if err != nil {
		return "", err
	}
	return renderLogs(logs, 0), nil
}
