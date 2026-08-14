package main

import (
	"github.com/baydogan/termup/api"
	"github.com/charmbracelet/lipgloss"
)

// The grid is drawn from these fixed dimensions; checkAt hit-tests against the
// same numbers so a mouse cell maps back to a specific check.
const (
	cardWidth = 40 // card content width (inside the border)
	barLen    = 34 // max bars shown per card (fits cardWidth)

	cardTotalW   = cardWidth + 2 // + left/right border
	cardTotalH   = 6             // top border + 4 content lines + bottom border
	barRowInCard = 3             // border + title + url, then the bar row
	barColStart  = 2             // border + left padding before the first bar cell

	// headerRows is the fixed number of rows above the grid: the title line, a
	// blank, the 3-row filter box, and a blank. Constant (the filter box is
	// always drawn) so checkAt's y-math stays valid.
	headerRows = 6
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

// gridCols is how many cards fit side by side. Shared by the renderer and the
// hit-test so they cannot disagree.
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

// checkAt maps a mouse cell (x, y) to the check under it, mirroring the grid
// layout. Returns ok=false when the cell is not over a status bar.
func (m model) checkAt(x, y int) (string, api.CheckDTO, bool) {
	vis := m.visible()
	if len(vis) == 0 {
		return "", api.CheckDTO{}, false
	}
	cols := gridCols(m.width)
	by := y - headerRows
	if by < 0 || by%cardTotalH != barRowInCard {
		return "", api.CheckDTO{}, false // not on a bar row
	}
	gridCol := x / cardTotalW
	if gridCol >= cols {
		return "", api.CheckDTO{}, false
	}
	barIdx := x%cardTotalW - barColStart
	if barIdx < 0 {
		return "", api.CheckDTO{}, false // in the border/padding, not a bar cell
	}
	mi := (by/cardTotalH)*cols + gridCol
	if mi >= len(vis) {
		return "", api.CheckDTO{}, false
	}

	h := vis[mi]
	if barIdx >= shownCount(h) {
		return "", api.CheckDTO{}, false
	}
	return h.Name, h.Recent[barStart(h)+barIdx], true
}
