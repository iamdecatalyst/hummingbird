package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/iamdecatalyst/hummingbird/cli/client"
)

// Version is the CLI version shown in the banner.
const Version = "v1.0.0"

const banner = `
  ██╗  ██╗██╗   ██╗███╗   ███╗███╗   ███╗██╗███╗   ██╗ ██████╗ ██████╗ ██╗██████╗ ██████╗
  ██║  ██║██║   ██║████╗ ████║████╗ ████║██║████╗  ██║██╔════╝ ██╔══██╗██║██╔══██╗██╔══██╗
  ███████║██║   ██║██╔████╔██║██╔████╔██║██║██╔██╗ ██║██║  ███╗██████╔╝██║██████╔╝██║  ██║
  ██╔══██║██║   ██║██║╚██╔╝██║██║╚██╔╝██║██║██║╚██╗██║██║   ██║██╔══██╗██║██╔══██╗██║  ██║
  ██║  ██║╚██████╔╝██║ ╚═╝ ██║██║ ╚═╝ ██║██║██║ ╚████║╚██████╔╝██████╔╝██║██║  ██║██████╔╝
  ╚═╝  ╚═╝ ╚═════╝ ╚═╝     ╚═╝╚═╝     ╚═╝╚═╝╚═╝  ╚═══╝ ╚═════╝ ╚═════╝ ╚═╝╚═╝  ╚═╝╚═════╝ `

const bird = `
    .
   /|\     Pump.fun Trading Agent
  / | \    by Vylth · VYLTH Strategies
 /__|__\
    |`

// Tab represents a view tab.
type Tab int

const (
	TabOverview Tab = iota
	TabTrades
	TabLogs
)

const tabCount = 3

// Model is the main application model.
type Model struct {
	activeTab Tab
	overview  OverviewModel
	trades    TradesModel
	logs      LogsModel
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
	}
}

func (m Model) Init() tea.Cmd {
	// Fetch overview immediately on launch
	var cmd tea.Cmd
	m.overview, cmd = m.overview.Fetch()
	return cmd
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "right":
			return m.switchTab((m.activeTab + 1) % tabCount)
		case "shift+tab", "left":
			return m.switchTab((m.activeTab + tabCount - 1) % tabCount)
		case "1":
			return m.switchTab(TabOverview)
		case "2":
			return m.switchTab(TabTrades)
		case "3":
			return m.switchTab(TabLogs)
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
	}

	return m, cmd
}

func (m Model) View() string {
	var b strings.Builder

	// Banner + bird side by side
	bannerLines := strings.Split(StyleBanner.Render(banner), "\n")
	birdLines := strings.Split(StyleBannerSub.Render(bird), "\n")

	// Pad to same height
	for len(birdLines) < len(bannerLines) {
		birdLines = append(birdLines, "")
	}

	for i, line := range bannerLines {
		b.WriteString(line)
		if i < len(birdLines) {
			b.WriteString("   " + birdLines[i])
		}
		b.WriteString("\n")
	}

	// Version
	b.WriteString("  " + StyleBannerVersion.Render(Version) + "\n")
	b.WriteString("\n")

	// Tab bar
	sep := StyleTabSep.Render("│")
	tabs := []string{
		renderTab(TabOverview, m.activeTab, "1", "Overview"),
		renderTab(TabTrades, m.activeTab, "2", "Trades"),
		renderTab(TabLogs, m.activeTab, "3", "Logs"),
	}
	b.WriteString("  " + strings.Join(tabs, " "+sep+" "))
	b.WriteString("\n")
	barWidth := 50
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
	}

	// Footer
	b.WriteString("\n\n")
	b.WriteString("  " + StyleHelp.Render("← → tab: switch tabs   1 overview · 2 trades · 3 logs   q: quit"))

	return b.String()
}

func renderTab(tab, active Tab, num, name string) string {
	label := "[" + num + "] " + name
	if tab == active {
		return StyleActiveTab.Render(label)
	}
	return StyleTab.Render(label)
}
