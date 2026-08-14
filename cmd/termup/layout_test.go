package main

import (
	"strings"
	"testing"

	"github.com/baydogan/termup/api"
	"github.com/charmbracelet/lipgloss"
)

// recentN builds n checks whose StatusCode equals their index, so a test can
// identify exactly which check a coordinate resolved to.
func recentN(n int) []api.CheckDTO {
	out := make([]api.CheckDTO, n)
	for i := range out {
		out[i] = api.CheckDTO{StatusCode: i, Up: true}
	}
	return out
}

func TestCheckAt(t *testing.T) {
	// width 100 -> cols = 100/42 = 2. Three monitors: (row0,col0),(row0,col1),(row1,col0).
	m := model{
		width: 100,
		health: []api.MonitorHealthDTO{
			{Name: "m0", Recent: recentN(5)},
			{Name: "m1", Recent: recentN(5)},
			{Name: "m2", Recent: recentN(50)}, // > barLen, exercises the tail window
		},
	}

	// bar row y is derived from the layout constants so it tracks headerRows.
	row0 := headerRows + barRowInCard              // grid row 0 bar line
	row1 := headerRows + cardTotalH + barRowInCard // grid row 1 bar line
	// bar first cell x: col0 = barColStart = 2; col1 = cardTotalW+2 = 44.
	cases := []struct {
		name       string
		x, y       int
		wantOK     bool
		wantMon    string
		wantStatus int // == index into that monitor's Recent
	}{
		{"m0 first bar", 2, row0, true, "m0", 0},
		{"m0 fifth bar", 6, row0, true, "m0", 4},
		{"m1 first bar", 44, row0, true, "m1", 0},
		{"m2 first bar (row1)", 2, row1, true, "m2", 50 - barLen}, // tail window start
		{"m2 last bar", 2 + barLen - 1, row1, true, "m2", 49},
		{"border cell", 0, row0, false, "", 0},
		{"padding cell", 1, row0, false, "", 0},
		{"not a bar row", 2, row0 - 1, false, "", 0},
		{"below bar row", 2, row0 + 1, false, "", 0},
		{"past available bars", 8, row0, false, "", 0},      // m0 has only 5 checks
		{"empty area (no monitor)", 44, row1, false, "", 0}, // row1,col1 has no monitor
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, c, ok := m.checkAt(tc.x, tc.y)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (name=%q status=%d)", ok, tc.wantOK, name, c.StatusCode)
			}
			if !ok {
				return
			}
			if name != tc.wantMon || c.StatusCode != tc.wantStatus {
				t.Errorf("got (%q, status %d), want (%q, status %d)", name, c.StatusCode, tc.wantMon, tc.wantStatus)
			}
		})
	}
}

// TestCardDimensionsMatchLayoutConstants pins the contract between the renderer
// and checkAt: the hit-test derives a check from a mouse cell using cardTotalW /
// cardTotalH / barRowInCard, so a card whose real size drifts from them would
// silently misroute clicks.
func TestCardDimensionsMatchLayoutConstants(t *testing.T) {
	h := api.MonitorHealthDTO{Name: "m", URL: "http://m.example", State: "up", Recent: recentN(5)}

	for _, tc := range []struct {
		name   string
		selIdx int
	}{
		{"plain", -1},
		{"selected", 0}, // thick border, must keep the same footprint
	} {
		t.Run(tc.name, func(t *testing.T) {
			card := renderCard(h, tc.selIdx)
			if w := lipgloss.Width(card); w != cardTotalW {
				t.Errorf("card width = %d, want cardTotalW = %d", w, cardTotalW)
			}
			if hh := lipgloss.Height(card); hh != cardTotalH {
				t.Errorf("card height = %d, want cardTotalH = %d", hh, cardTotalH)
			}
			lines := strings.Split(card, "\n")
			if !strings.Contains(lines[barRowInCard], "▌") && !strings.Contains(lines[barRowInCard], "█") {
				t.Errorf("row %d is not the bar row:\n%s", barRowInCard, card)
			}
		})
	}
}

// TestGridColsIsSharedByRendererAndHitTest guards the other half of that
// contract: both sides must compute the same column count.
func TestGridColsIsSharedByRendererAndHitTest(t *testing.T) {
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
