package main

import (
	"strings"
	"testing"

	"github.com/baydogan/termup/api"
	"github.com/baydogan/termup/cmd/termup/style"
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
