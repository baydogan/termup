package main

import (
	"testing"

	"github.com/baydogan/termup/api"
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
