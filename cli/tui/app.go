package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/iamdecatalyst/hummingbird/cli/client"
)

// Version is the CLI version shown in the banner.
const Version = "v1.0.0"

// tickMsg fires on the auto-refresh interval.
type tickMsg struct{}

const refreshInterval = 5 * time.Second

// Tab represents a view tab.
type Tab int

const (
	TabOverview Tab = iota
	TabTrades
	TabLogs
	TabControls
)

const tabCount = 4

// Model is the main application model.
type Model struct {
	activeTab Tab
	overview  OverviewModel
	trades    TradesModel
	logs      LogsModel
	controls  ControlsModel
	width     int
	height    int
}

// NewModel creates the main app model.
func NewModel(c *client.Client) Model {
	return Model{
		activeTab: TabOverview,
		overview:  NewOverviewModel(c),
		trades:    NewTradesModel(c),
		logs:      NewLogsModel(c),
		controls:  NewControlsModel(c),
	}
}

func (m Model) Init() tea.Cmd {
	var cmd tea.Cmd
	m.overview, cmd = m.overview.Fetch()
	return tea.Batch(cmd, tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		// Auto-refresh overview stats in background; schedule next tick
		var cmd tea.Cmd
		if m.activeTab == TabOverview {
			m.overview, cmd = m.overview.Fetch()
		}
		return m, tea.Batch(cmd, tickCmd())

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "right", "l":
			return m.switchTab((m.activeTab + 1) % tabCount)
		case "shift+tab", "left", "h":
			return m.switchTab((m.activeTab + tabCount - 1) % tabCount)
		case "1":
			return m.switchTab(TabOverview)
		case "2":
			return m.switchTab(TabTrades)
		case "3":
			return m.switchTab(TabLogs)
		case "4":
			return m.switchTab(TabControls)
		}
	}

	// Forward to active tab
	var cmd tea.Cmd
	switch m.activeTab {
	case TabOverview:
		m.overview, cmd = m.overview.Update(msg)
	case TabTrades:
		m.trades, cmd = m.trades.Update(msg)
	case TabLogs:
		m.logs, cmd = m.logs.Update(msg)
	case TabControls:
		m.controls, cmd = m.controls.Update(msg)
	}

	return m, cmd
}

func (m Model) switchTab(tab Tab) (Model, tea.Cmd) {
	m.activeTab = tab

	var cmd tea.Cmd
	switch tab {
	case TabOverview:
		if !m.overview.fetched {
			m.overview, cmd = m.overview.Fetch()
		}
	case TabTrades:
		if !m.trades.fetched {
			m.trades, cmd = m.trades.Fetch()
		}
	case TabLogs:
		if !m.logs.fetched {
			m.logs, cmd = m.logs.Fetch()
		}
	case TabControls:
		if !m.controls.fetched {
			m.controls, cmd = m.controls.Fetch()
		}
	}

	return m, cmd
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(renderBanner(m.width))
	b.WriteString("\n")

	// Tab bar
	sep := StyleTabSep.Render("│")
	tabs := []string{
		renderTab(TabOverview,  m.activeTab, "1", "overview"),
		renderTab(TabTrades,    m.activeTab, "2", "trades"),
		renderTab(TabLogs,      m.activeTab, "3", "logs"),
		renderTab(TabControls,  m.activeTab, "4", "controls"),
	}
	b.WriteString("  " + strings.Join(tabs, " "+sep+" "))
	b.WriteString("\n")
	barWidth := 60
	if m.width > 0 {
		barWidth = m.width - 4
	}
	b.WriteString("  " + StyleTabSep.Render(strings.Repeat("─", barWidth)))
	b.WriteString("\n\n")

	// Active view
	switch m.activeTab {
	case TabOverview:
		b.WriteString(m.overview.View())
	case TabTrades:
		b.WriteString(m.trades.View())
	case TabLogs:
		b.WriteString(m.logs.View())
	case TabControls:
		b.WriteString(m.controls.View())
	}

	// Footer
	b.WriteString("\n\n")
	hint := "← → / h l / tab: switch   1-4: jump to tab   r: refresh   q: quit"
	b.WriteString("  " + StyleHelp.Render(hint))

	return b.String()
}

func renderBanner(termWidth int) string {
	bird := `   ──.
  /  ◉ \──
  \    /   >────
   '──'`

	title := lipgloss.NewStyle().
		Foreground(ColorAccent).
		Bold(true).
		Render("hummingbird")

	sub := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render("autonomous pump.fun trading agent")

	ver := lipgloss.NewStyle().
		Foreground(ColorDim).
		Render(Version + "  ·  by VYLTH Strategies · @iamdecatalyst")

	birdLines   := strings.Split(lipgloss.NewStyle().Foreground(ColorAccent).Render(bird), "\n")
	detailLines := []string{
		"",
		"  " + title,
		"  " + sub,
		"  " + ver,
	}

	width := termWidth - 2
	if width < 60 {
		width = 60
	}
	divider := lipgloss.NewStyle().Foreground(ColorDim).Render(strings.Repeat("─", width))

	var b strings.Builder
	b.WriteString("\n  " + divider + "\n")

	maxLines := len(birdLines)
	if len(detailLines) > maxLines {
		maxLines = len(detailLines)
	}
	for i := 0; i < maxLines; i++ {
		bl := ""
		if i < len(birdLines) {
			bl = birdLines[i]
		}
		dl := ""
		if i < len(detailLines) {
			dl = detailLines[i]
		}
		b.WriteString("  " + bl + dl + "\n")
	}

	b.WriteString("  " + divider)
	return b.String()
}

func renderTab(tab, active Tab, num, name string) string {
	label := num + "  " + name
	if tab == active {
		return StyleActiveTab.Render("▸ " + label)
	}
	return StyleTab.Render("  " + label)
}
