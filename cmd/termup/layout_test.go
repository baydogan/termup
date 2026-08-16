package main

import (
	"strings"
	"testing"

	"github.com/baydogan/termup/api"
	"github.com/charmbracelet/lipgloss"
)

// recentN builds n checks whose StatusCode equals their index, so a test can
// identify exactly which check a bar or marker refers to.
func recentN(n int) []api.CheckDTO {
	out := make([]api.CheckDTO, n)
	for i := range out {
		out[i] = api.CheckDTO{StatusCode: i, Up: true}
	}
	return out
}

// TestCardFootprintIsStable pins what the grid depends on: every card is
// cardTotalW wide and the same height whether or not it holds the selection —
// the marker row is drawn either way, so cards in a row cannot drift apart.
func TestCardFootprintIsStable(t *testing.T) {
	h := api.MonitorHealthDTO{Name: "m", URL: "http://m.example", State: "up", Recent: recentN(5)}

	plain := renderCard(h, -1)
	selected := renderCard(h, 2)

	if w := lipgloss.Width(plain); w != cardTotalW {
		t.Errorf("card width = %d, want cardTotalW = %d", w, cardTotalW)
	}
	if got, want := lipgloss.Width(selected), lipgloss.Width(plain); got != want {
		t.Errorf("selected card width = %d, want %d", got, want)
	}
	if got, want := lipgloss.Height(selected), lipgloss.Height(plain); got != want {
		t.Errorf("selected card height = %d, want %d", got, want)
	}
}

func TestGridColsMatchesArrangeGrid(t *testing.T) {
	cards := []string{"a", "b", "c"}
	for _, width := range []int{0, 1, cardTotalW - 1, cardTotalW, 3 * cardTotalW, 500} {
		cols := gridCols(width)
		if cols < 1 {
			t.Errorf("gridCols(%d) = %d, want at least 1", width, cols)
		}
		rows := strings.Count(arrangeGrid(cards, width), "\n") + 1
		wantRows := (len(cards) + cols - 1) / cols
		if rows != wantRows {
			t.Errorf("width %d: grid has %d rows, want %d (cols=%d)", width, rows, wantRows, cols)
		}
	}
}

func TestBarWindowShowsTheNewestChecks(t *testing.T) {
	long := api.MonitorHealthDTO{Recent: recentN(barLen + 10)}
	if got := barStart(long); got != 10 {
		t.Errorf("barStart = %d, want 10 (the oldest 10 scroll off)", got)
	}
	if got := shownCount(long); got != barLen {
		t.Errorf("shownCount = %d, want %d", got, barLen)
	}
	if got := lastCheck(long); got != barLen-1 {
		t.Errorf("lastCheck = %d, want %d", got, barLen-1)
	}

	short := api.MonitorHealthDTO{Recent: recentN(3)}
	if barStart(short) != 0 || shownCount(short) != 3 || lastCheck(short) != 2 {
		t.Errorf("short history: start=%d shown=%d last=%d, want 0/3/2",
			barStart(short), shownCount(short), lastCheck(short))
	}

	empty := api.MonitorHealthDTO{}
	if barStart(empty) != 0 || shownCount(empty) != 0 || lastCheck(empty) != 0 {
		t.Error("empty history should be all zeroes")
	}
}
