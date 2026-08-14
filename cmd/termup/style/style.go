// Package style holds the dashboard's visual vocabulary: colors, text styles and
// badges. It knows nothing about layout or data — geometry (card sizes, the bar
// window) and the render functions live with the TUI itself, so widths are
// applied at the call site (e.g. style.Card.Width(w).Render(...)).
package style

import "github.com/charmbracelet/lipgloss"

// Palette. Everything below is built from these five.
var (
	Green  = lipgloss.Color("42")
	Red    = lipgloss.Color("196")
	Gray   = lipgloss.Color("240")
	Fg     = lipgloss.Color("252")
	Accent = lipgloss.Color("39") // selection highlight
)

// Text styles.
var (
	Title   = lipgloss.NewStyle().Bold(true).Foreground(Fg)
	Name    = lipgloss.NewStyle().Bold(true).Foreground(Fg)
	SelName = lipgloss.NewStyle().Bold(true).Foreground(Accent)
	Subtle  = lipgloss.NewStyle().Foreground(Gray)
	Help    = lipgloss.NewStyle().Foreground(Gray)
	Err     = lipgloss.NewStyle().Bold(true).Foreground(Red)
)

// Badges: the monitor's state as one glyph plus a word. unknown is gray, not
// red — "not confirmed yet" is not the same as "down".
var (
	HealthyBadge   = lipgloss.NewStyle().Foreground(Green).Render("● healthy")
	UnhealthyBadge = lipgloss.NewStyle().Foreground(Red).Render("● unhealthy")
	UnknownBadge   = lipgloss.NewStyle().Foreground(Gray).Render("● unknown")
)

// Filter box: gray when idle, accent when focused or holding a query.
var (
	FilterLabel = lipgloss.NewStyle().Bold(true).Foreground(Accent)

	FilterBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Gray).
			Padding(0, 1)

	FilterBoxActive = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Accent).
			Padding(0, 1)
)

// Cards. SelectedCard marks the active card with a thick accent border; it is
// still a single-cell border, so the grid layout (and its hit-test) is unchanged.
var (
	Card = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Gray).
		Padding(0, 1)

	SelectedCard = lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(Accent).
			Padding(0, 1)
)
