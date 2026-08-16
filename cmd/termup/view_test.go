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

func TestBarColor(t *testing.T) {
	if got := barColor(true); got != style.Green {
		t.Errorf("up bar colour = %v, want %v", got, style.Green)
	}
	if got := barColor(false); got != style.Red {
		t.Errorf("down bar colour = %v, want %v", got, style.Red)
	}
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// TestRenderBarIsUniform pins the decision behind the marker row: bars are never
// restyled per check, so nothing about the selection can change their glyphs or
// width (the earlier ▌→█ swap made the selected bar look like it had moved).
func TestRenderBarIsUniform(t *testing.T) {
	recent := []api.CheckDTO{{Up: true}, {Up: false}, {Up: true}}
	plain := ansiRe.ReplaceAllString(renderBar(recent), "")

	if got := strings.Count(plain, barGlyph); got != len(recent) {
		t.Errorf("bar has %d glyphs, want %d (%q)", got, len(recent), plain)
	}
	if strings.Trim(plain, barGlyph) != "" {
		t.Errorf("bar mixes glyphs: %q", plain)
	}
	if got := lipgloss.Width(renderBar(recent)); got != len(recent) {
		t.Errorf("bar width = %d, want %d", got, len(recent))
	}

	// The window keeps only the newest barLen checks.
	if got := lipgloss.Width(renderBar(recentN(barLen + 5))); got != barLen {
		t.Errorf("long history bar width = %d, want %d", got, barLen)
	}
	if got := renderBar(nil); !strings.Contains(got, "no data yet") {
		t.Errorf("empty history bar = %q", got)
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
