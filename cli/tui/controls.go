package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/iamdecatalyst/hummingbird/cli/client"
)

type controlsMsg struct {
	stats *client.Stats
	err   error
}

type actionMsg struct {
	action string
	err    error
}

type confirmState int

const (
	confirmNone confirmState = iota
	confirmStop
	confirmExitAll
)

// ControlsModel is the bot controls tab.
type ControlsModel struct {
	spinner  spinner.Model
	client   *client.Client
	stats    *client.Stats
	err      error
	loading  bool
	fetched  bool
	busy     bool
	confirm  confirmState
	lastMsg  string
	lastOk   bool
}

func NewControlsModel(c *client.Client) ControlsModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(ColorAccent)
	return ControlsModel{spinner: sp, client: c}
}

func (m ControlsModel) Init() tea.Cmd { return nil }

func (m ControlsModel) Fetch() (ControlsModel, tea.Cmd) {
	m.loading = true
	m.err = nil
	c := m.client
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		stats, err := c.GetStats()
		return controlsMsg{stats: stats, err: err}
	})
}

func (m ControlsModel) doAction(action string) (ControlsModel, tea.Cmd) {
	m.busy = true
	m.confirm = confirmNone
	m.lastMsg = ""
	c := m.client
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		var err error
		switch action {
		case "stop":
			err = c.Stop()
		case "resume":
			err = c.Resume()
		case "start":
			err = c.Start()
		}
		return actionMsg{action: action, err: err}
	})
}

func (m ControlsModel) Update(msg tea.Msg) (ControlsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Escape cancels confirmation
		if msg.String() == "esc" {
			m.confirm = confirmNone
			return m, nil
		}

		if m.busy {
			return m, nil
		}

		switch m.confirm {
		case confirmStop:
			if msg.String() == "y" {
				return m.doAction("stop")
			}
			m.confirm = confirmNone
			return m, nil

		case confirmExitAll:
			if msg.String() == "y" {
				// Exit all = stop bot
				return m.doAction("stop")
			}
			m.confirm = confirmNone
			return m, nil
		}

		// Normal keys
		switch msg.String() {
		case "r":
			return m.Fetch()
		case "s":
			// Stop / pause — ask confirmation
			m.confirm = confirmStop
			return m, nil
		case "R":
			return m.doAction("resume")
		case "S":
			return m.doAction("start")
		case "e":
			m.confirm = confirmExitAll
			return m, nil
		}

	case controlsMsg:
		m.loading = false
		m.fetched = true
		m.stats = msg.stats
		m.err = msg.err
		return m, nil

	case actionMsg:
		m.busy = false
		m.fetched = false // reload status after action
		if msg.err != nil {
			m.lastOk = false
			m.lastMsg = msg.err.Error()
		} else {
			m.lastOk = true
			switch msg.action {
			case "stop":
				m.lastMsg = "bot paused"
			case "resume":
				m.lastMsg = "bot resumed"
			case "start":
				m.lastMsg = "bot started"
			}
		}
		// Re-fetch stats after action
		return m.Fetch()

	case spinner.TickMsg:
		if m.loading || m.busy {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m ControlsModel) View() string {
	var b strings.Builder

	// ── Status card ─────────────────────────────────────
	if m.loading && !m.fetched {
		b.WriteString("  " + m.spinner.View() + StyleMuted.Render("  loading status..."))
		return b.String()
	}

	if m.err != nil {
		b.WriteString("  " + StyleError.Render("✗  "+m.err.Error()))
		b.WriteString("\n\n  " + StyleHelp.Render("r  retry"))
		return b.String()
	}

	if m.stats != nil {
		b.WriteString(renderStatusCard(m.stats))
		b.WriteString("\n\n")
	}

	// ── Busy spinner ─────────────────────────────────────
	if m.busy {
		b.WriteString("  " + m.spinner.View() + StyleMuted.Render("  executing..."))
		return b.String()
	}

	// ── Last action result ───────────────────────────────
	if m.lastMsg != "" {
		if m.lastOk {
			b.WriteString("  " + StyleSuccess.Render("✓  "+m.lastMsg))
		} else {
			b.WriteString("  " + StyleError.Render("✗  "+m.lastMsg))
		}
		b.WriteString("\n\n")
	}

	// ── Confirmation prompts ─────────────────────────────
	switch m.confirm {
	case confirmStop:
		b.WriteString(StyleBox.Render(
			"  " + StyleConfirmWarning.Render("▼ STOP BOT") + "\n\n" +
			"  " + StyleMuted.Render("This pauses trading. Open positions stay open.") + "\n\n" +
			"  " + StyleYellow.Render("y") + StyleMuted.Render("  confirm    ") +
			StyleMuted.Render("esc  cancel"),
		))
		return b.String()

	case confirmExitAll:
		b.WriteString(StyleBox.Render(
			"  " + StyleRed.Bold(true).Render("! EXIT ALL POSITIONS") + "\n\n" +
			"  " + StyleMuted.Render("Immediately market-sells all open positions.") + "\n" +
			"  " + StyleRed.Render("This cannot be undone.") + "\n\n" +
			"  " + StyleRed.Bold(true).Render("y") + StyleMuted.Render("  confirm    ") +
			StyleMuted.Render("esc  cancel"),
		))
		return b.String()
	}

	// ── Action menu ──────────────────────────────────────
	paused := m.stats == nil || m.stats.Paused
	running := m.stats != nil && !m.stats.Paused && m.stats.Configured

	b.WriteString(renderControls(running, paused))
	b.WriteString("\n\n  " + StyleHelp.Render("r  refresh status"))

	return b.String()
}

func renderStatusCard(s *client.Stats) string {
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
		statusText = StyleGreen.Bold(true).Render("LIVE  ·  TRADING")
	}

	openStr := fmt.Sprintf("%d open position", s.OpenPositions)
	if s.OpenPositions != 1 {
		openStr += "s"
	}

	inner := fmt.Sprintf("  %s  %s\n\n  %s",
		dot, statusText,
		StyleMuted.Render(openStr),
	)
	return StyleBox.Render(inner)
}

func renderControls(running, paused bool) string {
	type action struct {
		key  string
		name string
		desc string
		live bool
	}

	actions := []action{
		{"S", "Start Bot",         "activate the trading engine",            !running && paused},
		{"R", "Resume Bot",        "unpause — continue trading",             paused},
		{"s", "Stop / Pause Bot",  "pause trading, keep positions open",     running},
		{"e", "Exit All Positions","market-sell everything immediately",      running || !paused},
	}

	var b strings.Builder
	b.WriteString("  " + StyleSection.Render("ACTIONS") + "\n\n")

	for _, a := range actions {
		keyStyle := StyleAccent.Bold(true)
		nameStyle := StyleValue
		descStyle := StyleMuted
		if !a.live {
			keyStyle = StyleMuted
			nameStyle = StyleMuted
		}
		line := fmt.Sprintf("  %s  %s  %s",
			keyStyle.Width(4).Render("["+a.key+"]"),
			nameStyle.Width(24).Render(a.name),
			descStyle.Render(a.desc),
		)
		b.WriteString(line + "\n")
	}

	return StyleBox.Render(b.String())
}

// suppress unused import
var _ = strings.TrimSpace
