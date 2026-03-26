package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/iamdecatalyst/hummingbird/cli/client"
)

// logsMsg is sent when the logs fetch completes.
type logsMsg struct {
	logs []client.LogEntry
	err  error
}

// LogsModel is the logs tab view.
type LogsModel struct {
	spinner  spinner.Model
	client   *client.Client
	logs     []client.LogEntry
	err      error
	loading  bool
	fetched  bool
	scrollOff int
}

// NewLogsModel creates a new logs view.
func NewLogsModel(c *client.Client) LogsModel {
	sp := spinner.New()
	sp.Spinner = spinner.Points
	sp.Style = lipgloss.NewStyle().Foreground(ColorAccent)

	return LogsModel{
		spinner: sp,
		client:  c,
	}
}

func (m LogsModel) Init() tea.Cmd {
	return nil
}

// Fetch starts a logs fetch. Called when the tab is activated.
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

	b.WriteString("  " + StyleTitle.Render("◎ Event Log"))
	b.WriteString("\n\n")

	if m.loading {
		b.WriteString("  " + m.spinner.View() + StyleMuted.Render("  Fetching logs..."))
		return b.String()
	}

	if m.err != nil {
		b.WriteString("  " + StyleError.Render("✗ "+m.err.Error()))
		b.WriteString("\n\n  " + StyleMuted.Render("Press r to retry"))
		return b.String()
	}

	if !m.fetched {
		b.WriteString("  " + StyleMuted.Render("Loading..."))
		return b.String()
	}

	b.WriteString(renderLogs(m.logs, m.scrollOff))
	b.WriteString("\n\n  " + StyleHelp.Render("↑↓ / j k: scroll   r: refresh"))

	return b.String()
}

// logEntryStyle returns the appropriate style for a log entry type.
func logEntryStyle(entryType string) lipgloss.Style {
	switch strings.ToUpper(entryType) {
	case "ENTER", "START":
		return StyleGreen
	case "EXIT":
		return lipgloss.NewStyle().Foreground(ColorAccent)
	case "STOP":
		return StyleYellow
	case "ALERT":
		return StyleRed
	default:
		return StyleMuted
	}
}

func renderLogEntry(e client.LogEntry) string {
	typeStyle := logEntryStyle(e.Type)
	typeTag := typeStyle.Bold(true).Width(8).Render(e.Type)

	timeStr := StyleMuted.Render(e.Time)

	var details strings.Builder
	if e.Token != "" {
		details.WriteString(StyleMuted.Render(" token=") + StyleValue.Render(shortMint(e.Token)))
	}
	if e.AmtSOL != 0 {
		details.WriteString(StyleMuted.Render(" amt=") + StyleValue.Render(fmt.Sprintf("%.4f SOL", e.AmtSOL)))
	}
	if e.PnLSOL != 0 {
		pnlStyle := PnLStyle(e.PnLSOL)
		details.WriteString(StyleMuted.Render(" pnl=") + pnlStyle.Render(fmt.Sprintf("%+.4f SOL", e.PnLSOL)))
	}
	if e.PnLPct != 0 {
		pnlStyle := PnLStyle(e.PnLPct)
		details.WriteString(pnlStyle.Render(fmt.Sprintf(" (%+.1f%%)", e.PnLPct)))
	}
	if e.Reason != "" {
		details.WriteString(StyleMuted.Render(" reason=") + StyleValue.Render(e.Reason))
	}

	msg := ""
	if e.Message != "" {
		msg = "  " + StyleValue.Render(e.Message)
	}

	return fmt.Sprintf("  %s  %s%s%s", timeStr, typeTag, details.String(), msg)
}

func renderLogs(logs []client.LogEntry, scrollOff int) string {
	if len(logs) == 0 {
		return StyleBox.Render("  " + StyleMuted.Render("No log entries"))
	}

	maxDisplay := 50
	// Newest first
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
	visible := reversed[start:end]

	var b strings.Builder
	for _, e := range visible {
		b.WriteString(renderLogEntry(e))
		b.WriteString("\n")
	}

	if len(logs) > maxDisplay {
		b.WriteString("\n  " + StyleMuted.Render(fmt.Sprintf("Showing %d–%d of %d entries", start+1, end, len(logs))))
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
