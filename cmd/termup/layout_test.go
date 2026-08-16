package main

import (
	"strings"
	"testing"
	"time"

	"github.com/baydogan/termup/api"
	"github.com/charmbracelet/bubbles/textinput"
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

	// bar row y is derived from the layout itself so it tracks the real header.
	row0 := m.headerRows() + barRowInCard              // grid row 0 bar line
	row1 := m.headerRows() + cardTotalH + barRowInCard // grid row 1 bar line
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

// TestHeaderRowsMatchesRenderedView is the guard that was missing: it pins
// headerRows to what View actually draws, so adding a line to the header (or
// changing the filter box) can no longer shift the grid out from under checkAt.
func TestHeaderRowsMatchesRenderedView(t *testing.T) {
	base := model{
		width:  100,
		filter: textinput.New(),
		health: []api.MonitorHealthDTO{{Name: "m0", URL: "http://m0", State: "up", Recent: recentN(4)}},
	}

	cases := []struct {
		name string
		m    model
	}{
		{"idle", base},
		{"with timestamp", func() model { m := base; m.updated = time.Unix(1000, 0); return m }()},
		{"filter holds a query", func() model {
			m := base
			m.filter = textinput.New()
			m.filter.SetValue("m0")
			return m
		}()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := strings.Split(tc.m.View(), "\n")
			hr := tc.m.headerRows()
			if hr >= len(lines) {
				t.Fatalf("headerRows = %d but View drew only %d lines", hr, len(lines))
			}
			// The grid starts exactly at headerRows: that row carries a card's top
			// border, and the row before it is the blank separator.
			if !strings.ContainsAny(lines[hr], "╭┏") {
				t.Errorf("row %d is not the first card row:\n%s", hr, tc.m.View())
			}
			if strings.TrimSpace(lines[hr-1]) != "" {
				t.Errorf("row %d should be blank, got %q", hr-1, lines[hr-1])
			}
		})
	}
}

// TestCheckAtUsesTheRenderedHeader ties the two together end to end: the bar row
// found in the rendered output is the row checkAt resolves to a check.
func TestCheckAtUsesTheRenderedHeader(t *testing.T) {
	m := model{
		width:  100,
		filter: textinput.New(),
		health: []api.MonitorHealthDTO{{Name: "m0", URL: "http://m0", State: "up", Recent: recentN(4)}},
	}

	lines := strings.Split(m.View(), "\n")
	barRow := -1
	for i, l := range lines {
		if strings.Contains(l, "▌") {
			barRow = i
			break
		}
	}
	if barRow < 0 {
		t.Fatalf("no bar row in the rendered view:\n%s", m.View())
	}

	if _, _, ok := m.checkAt(barColStart, barRow); !ok {
		t.Errorf("checkAt missed the bar row at y=%d (headerRows=%d)", barRow, m.headerRows())
	}
}
