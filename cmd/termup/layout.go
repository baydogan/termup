package main

import (
	"github.com/baydogan/termup/api"
	"github.com/charmbracelet/lipgloss"
)

// Grid geometry. Only the renderer reads these now: navigation is keyboard-only,
// so there is no mouse hit-test to keep in sync with them.
const (
	cardWidth = 40 // card content width (inside the border)
	barLen    = 34 // max bars shown per card (fits cardWidth)

	cardTotalW = cardWidth + 2 // + left/right border
)

// arrangeGrid lays cards out in as many columns as fit the terminal width.
func arrangeGrid(cards []string, width int) string {
	cols := gridCols(width)
	var rows []string
	for i := 0; i < len(cards); i += cols {
		end := i + cols
		if end > len(cards) {
			end = len(cards)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cards[i:end]...))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// gridCols is how many cards fit side by side.
func gridCols(width int) int {
	if cols := width / cardTotalW; cols > 1 {
		return cols
	}
	return 1
}

// barStart is the index of the first displayed bar within Recent (the bar only
// shows the last barLen checks). shownCount is how many bars are displayed.
func barStart(h api.MonitorHealthDTO) int {
	if len(h.Recent) > barLen {
		return len(h.Recent) - barLen
	}
	return 0
}

func shownCount(h api.MonitorHealthDTO) int { return len(h.Recent) - barStart(h) }

func lastCheck(h api.MonitorHealthDTO) int {
	if s := shownCount(h); s > 0 {
		return s - 1
	}
	return 0
}
