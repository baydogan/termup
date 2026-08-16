package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/baydogan/termup/api"
	"github.com/baydogan/termup/cmd/termup/style"
	"github.com/charmbracelet/lipgloss"
)

func TestBadgeFor(t *testing.T) {
	cases := []struct {
		state string
		want  string
	}{
		{"up", style.HealthyBadge},
		{"down", style.UnhealthyBadge},
		{"unknown", style.UnknownBadge},
		{"", style.UnknownBadge}, // no state on the wire is not a healthy target either
	}
	for _, tc := range cases {
		t.Run(tc.state, func(t *testing.T) {
			if got := badgeFor(tc.state); got != tc.want {
				t.Errorf("badgeFor(%q) = %q, want %q", tc.state, got, tc.want)
			}
		})
	}
}

// TestRenderCardUnknownIsNotUnhealthy guards the regression: an unconfirmed
// target used to render with the red "unhealthy" badge.
func TestRenderCardUnknownIsNotUnhealthy(t *testing.T) {
	card := renderCard(api.MonitorHealthDTO{Name: "m", URL: "http://m", State: "unknown"}, -1)
	if strings.Contains(card, "unhealthy") {
		t.Errorf("unknown monitor rendered as unhealthy:\n%s", card)
	}
	if !strings.Contains(card, "unknown") {
		t.Errorf("unknown badge missing from card:\n%s", card)
	}
}

func TestBarColorMarksSelectionWithLight(t *testing.T) {
	cases := []struct {
		name         string
		up, selected bool
		want         lipgloss.Color
	}{
		{"up", true, false, style.Green},
		{"up selected", true, true, style.GreenBright},
		{"down", false, false, style.Red},
		{"down selected", false, true, style.RedBright},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := barColor(tc.up, tc.selected); got != tc.want {
				t.Errorf("barColor(%v, %v) = %v, want %v", tc.up, tc.selected, got, tc.want)
			}
		})
	}
	// The highlight must be a different shade, otherwise selection is invisible.
	if style.GreenBright == style.Green || style.RedBright == style.Red {
		t.Error("bright shades equal the base colors")
	}
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// TestRenderBarSelectionKeepsGeometry is the regression for the selected bar
// appearing to shift sideways: selection changes color only, so the glyphs and
// the rendered width must be identical with and without a selection.
func TestRenderBarSelectionKeepsGeometry(t *testing.T) {
	recent := []api.CheckDTO{{Up: true}, {Up: false}, {Up: true}, {Up: true}}
	plain := ansiRe.ReplaceAllString(renderBar(recent, -1), "")

	for _, selIdx := range []int{0, 1, len(recent) - 1} {
		got := ansiRe.ReplaceAllString(renderBar(recent, selIdx), "")
		if got != plain {
			t.Errorf("selIdx %d changed the glyphs: %q, want %q", selIdx, got, plain)
		}
		if w, want := lipgloss.Width(renderBar(recent, selIdx)), lipgloss.Width(renderBar(recent, -1)); w != want {
			t.Errorf("selIdx %d changed the width: %d, want %d", selIdx, w, want)
		}
	}
	if strings.Contains(plain, "█") {
		t.Errorf("bar still uses the full block glyph: %q", plain)
	}
	if n := strings.Count(plain, barGlyph); n != len(recent) {
		t.Errorf("bar has %d glyphs, want %d", n, len(recent))
	}
}

// TestBarStyleMarksSelection checks the attributes that carry the emphasis,
// independent of whether the test process has a colour-capable TTY. Both are cell
// attributes, so they cannot shift the bar (the geometry test above pins that);
// the marker row is what identifies the selection.
func TestBarStyleMarksSelection(t *testing.T) {
	sel := barStyle(false, true)
	if got := sel.GetForeground(); got != style.RedBright {
		t.Errorf("selected down bar colour = %v, want %v", got, style.RedBright)
	}
	if got := sel.GetBackground(); got != style.BarSelBg {
		t.Errorf("selected bar background = %v, want %v", got, style.BarSelBg)
	}

	plain := barStyle(true, false)
	if plain.GetBackground() == style.BarSelBg {
		t.Error("unselected bar should not be tinted")
	}
}

// TestMarkerPointsAtTheSelectedBar is what replaced the highlight games: the
// pointer sits in the row under the bar, in the selected bar's own column.
func TestMarkerPointsAtTheSelectedBar(t *testing.T) {
	recent := recentN(6)

	for _, selIdx := range []int{0, 3, len(recent) - 1} {
		marker := ansiRe.ReplaceAllString(renderMarker(recent, selIdx), "")
		if got := strings.Index(marker, markerGlyph); got != selIdx {
			t.Errorf("selIdx %d: marker at column %d, want %d (%q)", selIdx, got, selIdx, marker)
		}
	}

	// Nothing selected in this card: the row is still drawn, but empty.
	blank := ansiRe.ReplaceAllString(renderMarker(recent, -1), "")
	if strings.Contains(blank, markerGlyph) {
		t.Errorf("unselected card shows a marker: %q", blank)
	}
	if blank == "" {
		t.Error("marker row must not be empty, or cards in a grid row lose alignment")
	}
	// Out-of-window selection (history scrolled past it) draws no pointer either.
	if m := ansiRe.ReplaceAllString(renderMarker(recentN(2), 5), ""); strings.Contains(m, markerGlyph) {
		t.Errorf("marker drawn outside the bar window: %q", m)
	}
}

// TestMarkerRowFollowsTheBarRow keeps the pointer directly under the bars inside
// the card, which is the whole reason it reads as a pointer.
func TestMarkerRowFollowsTheBarRow(t *testing.T) {
	card := renderCard(api.MonitorHealthDTO{Name: "m", URL: "http://m", State: "up", Recent: recentN(5)}, 1)
	lines := strings.Split(ansiRe.ReplaceAllString(card, ""), "\n")

	barRow, markerRow := -1, -1
	for i, l := range lines {
		if barRow < 0 && strings.Contains(l, barGlyph) {
			barRow = i
		}
		if strings.Contains(l, markerGlyph) {
			markerRow = i
		}
	}
	if barRow < 0 || markerRow < 0 {
		t.Fatalf("bar row %d, marker row %d in:\n%s", barRow, markerRow, card)
	}
	if markerRow != barRow+1 {
		t.Errorf("marker is on row %d, want %d (directly under the bars)", markerRow, barRow+1)
	}
	// Same column inside the card: bars and marker share the content offset.
	if strings.Index(lines[markerRow], markerGlyph) != strings.Index(lines[barRow], barGlyph)+1 {
		t.Errorf("marker column does not line up with the second bar:\n%s", card)
	}
}
