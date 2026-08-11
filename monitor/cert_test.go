package monitor_test

import (
	"testing"

	"github.com/baydogan/termup/monitor"
)

func TestCertTrackerFiresOnceAndReArms(t *testing.T) {
	c := monitor.NewCertTracker()
	const below = monitor.CertWarnDays - 1
	const above = monitor.CertWarnDays + 1

	// Above threshold: no alert.
	if c.Observe("x", above, true) {
		t.Fatal("above threshold should not fire")
	}
	// Cross below: fire once.
	if !c.Observe("x", below, true) {
		t.Fatal("crossing below threshold should fire")
	}
	// Still below on later probes: no repeat.
	if c.Observe("x", below, true) {
		t.Fatal("staying below should not re-fire")
	}
	// Renewed above: re-arm (no alert on the re-arm itself).
	if c.Observe("x", above, true) {
		t.Fatal("re-arm should not fire")
	}
	// Cross below again: fire once more.
	if !c.Observe("x", below, true) {
		t.Fatal("second crossing should fire")
	}
}

func TestCertTrackerNoCertLeavesArmed(t *testing.T) {
	c := monitor.NewCertTracker()

	// hasCert=false carries no expiry info and must not fire.
	if c.Observe("x", 0, false) {
		t.Fatal("no cert should not fire")
	}
	// A real below-threshold reading still fires afterwards.
	if !c.Observe("x", monitor.CertWarnDays-1, true) {
		t.Fatal("below threshold should fire after a no-cert round")
	}
}

func TestCertTrackerBoundary(t *testing.T) {
	c := monitor.NewCertTracker()
	// Exactly at the threshold is not "below" (< comparison): no alert.
	if c.Observe("x", monitor.CertWarnDays, true) {
		t.Fatal("exactly at threshold should not fire")
	}
}
