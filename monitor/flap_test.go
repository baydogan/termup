package monitor_test

import (
	"testing"

	"github.com/baydogan/termup/monitor"
)

func alternating(n int) []bool {
	out := make([]bool, n)
	for i := range out {
		out[i] = i%2 == 0
	}
	return out
}

func steadyUp(n int) []bool {
	out := make([]bool, n)
	for i := range out {
		out[i] = true
	}
	return out
}

func flapFeed(f *monitor.FlapTracker, name string, ups []bool) (fires int) {
	for _, u := range ups {
		if _, ok := f.Observe(name, u); ok {
			fires++
		}
	}
	return fires
}

func TestFlapTrackerSteadyNeverFires(t *testing.T) {
	f := monitor.NewFlapTracker()
	if fires := flapFeed(f, "x", steadyUp(monitor.FlapWindow)); fires != 0 {
		t.Errorf("steady up fired %d times, want 0", fires)
	}
}

func TestFlapTrackerFiresOnceWhileOscillating(t *testing.T) {
	f := monitor.NewFlapTracker()
	// Alternating for the whole window flips every step; it crosses the
	// threshold once and must not keep re-firing.
	if fires := flapFeed(f, "x", alternating(monitor.FlapWindow)); fires != 1 {
		t.Errorf("oscillating fired %d times, want 1", fires)
	}
}

func TestFlapTrackerReArmsAfterSettling(t *testing.T) {
	f := monitor.NewFlapTracker()

	fires := flapFeed(f, "x", alternating(monitor.FlapWindow)) // fire #1
	fires += flapFeed(f, "x", steadyUp(monitor.FlapWindow))    // settle -> re-arm
	fires += flapFeed(f, "x", alternating(monitor.FlapWindow)) // fire #2

	if fires != 2 {
		t.Errorf("total fires = %d, want 2 (once per flapping episode)", fires)
	}
}
