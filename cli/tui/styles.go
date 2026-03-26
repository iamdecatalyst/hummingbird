package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Hummingbird blue monochrome theme colors
var (
	ColorAccent  = lipgloss.Color("#00A8FF")
	ColorBg      = lipgloss.Color("#0d0d0d")
	ColorSurface = lipgloss.Color("#141414")
	ColorBorder  = lipgloss.Color("#1a1a1a")
	ColorMuted   = lipgloss.Color("#555555")
	ColorText    = lipgloss.Color("#ffffff")
	ColorGreen   = lipgloss.Color("#4ADE80")
	ColorRed     = lipgloss.Color("#EF4444")
	ColorYellow  = lipgloss.Color("#F59E0B")
	ColorDim     = lipgloss.Color("#333333")
)

// Styles
var (
	StyleBanner = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)

	StyleBannerSub = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)

	StyleBannerVersion = lipgloss.NewStyle().
				Foreground(ColorDim)

	StyleTab = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(ColorMuted)

	StyleActiveTab = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(ColorAccent).
			Bold(true)

	StyleTabSep = lipgloss.NewStyle().
			Foreground(ColorBorder)

	StyleBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(1, 2)

	StyleTitle = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)

	StyleLabel = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Width(22)

	StyleValue = lipgloss.NewStyle().
			Foreground(ColorText)

	StyleGreen = lipgloss.NewStyle().
			Foreground(ColorGreen)

	StyleRed = lipgloss.NewStyle().
			Foreground(ColorRed)

	StyleYellow = lipgloss.NewStyle().
			Foreground(ColorYellow)

	StyleMuted = lipgloss.NewStyle().
			Foreground(ColorMuted)

	StyleHelp = lipgloss.NewStyle().
			Foreground(ColorDim).
			Italic(true)

	StyleError = lipgloss.NewStyle().
			Foreground(ColorRed).
			Bold(true)

	StyleDivider = lipgloss.NewStyle().
			Foreground(ColorBorder)
)

// PnLStyle returns the appropriate style for a P&L value.
func PnLStyle(pnl float64) lipgloss.Style {
	if pnl > 0 {
		return StyleGreen
	}
	if pnl < 0 {
		return StyleRed
	}
	return StyleMuted
}

// StatusDot returns a colored status indicator dot.
func StatusDot(paused bool, offline bool) string {
	if offline {
		return StyleRed.Render("●")
	}
	if paused {
		return StyleYellow.Render("●")
	}
	return StyleGreen.Render("●")
}
