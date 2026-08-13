package monitor_test

import (
	"testing"

	"github.com/baydogan/termup/monitor"
)

// result builds an up (2xx) or down (5xx) probe result for a monitor.
func result(name string, up bool) monitor.Result {
	code := 500
	if up {
		code = 200
	}
	return monitor.Result{MonitorName: name, StatusCode: code}
}

type transition struct {
	from monitor.State
	to   monitor.State
}

// feed runs a sequence of up/down probes through the machine and returns every
// alarm-worthy transition it emitted, in order.
func feed(m *monitor.Machine, name string, ups ...bool) []transition {
	var got []transition
	for _, up := range ups {
		if t, ok := m.Observe(result(name, up)); ok {
			got = append(got, transition{t.From, t.To})
		}
	}
	return got
}

func TestMachine(t *testing.T) {
	const (
		up   = true
		down = false
	)

	cases := []struct {
		name  string
		ups   []bool
		want  []transition
		final monitor.State
	}{
		{
			name:  "two fails from unknown do not flip",
			ups:   []bool{down, down},
			want:  nil,
			final: monitor.StateUnknown,
		},
		{
			name:  "third consecutive fail flips to down",
			ups:   []bool{down, down, down},
			want:  []transition{{monitor.StateUnknown, monitor.StateDown}},
			final: monitor.StateDown,
		},
		{
			// Restart noise (2026-08-13): the first 2xx moves State to up but
			// must not alarm, since every restart starts from unknown.
			name:  "single 2xx flips to up silently",
			ups:   []bool{up},
			want:  nil,
			final: monitor.StateUp,
		},
		{
			name:  "success resets the fail streak",
			ups:   []bool{down, down, up, down, down},
			want:  nil,
			final: monitor.StateUp,
		},
		{
			name:  "down then single 2xx recovers",
			ups:   []bool{down, down, down, up},
			want:  []transition{{monitor.StateUnknown, monitor.StateDown}, {monitor.StateDown, monitor.StateUp}},
			final: monitor.StateUp,
		},
		{
			// The silent unknown->up does not swallow the later real transition.
			name:  "up then three fails flips down",
			ups:   []bool{up, down, down, down},
			want:  []transition{{monitor.StateUp, monitor.StateDown}},
			final: monitor.StateDown,
		},
		{
			// Known weak point (2026-08-10): a target flapping below the
			// threshold never reaches down, so it never alarms.
			name:  "sub-threshold flapping never goes down",
			ups:   []bool{down, down, up, down, down, up},
			want:  nil,
			final: monitor.StateUp,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := monitor.NewMachine()
			got := feed(m, "x", tc.ups...)

			if len(got) != len(tc.want) {
				t.Fatalf("transitions = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("transition %d = %v, want %v", i, got[i], tc.want[i])
				}
			}
			if s := m.State("x"); s != tc.final {
				t.Errorf("final state = %v, want %v", s, tc.final)
			}
		})
	}
}

func TestMachineTracksMonitorsIndependently(t *testing.T) {
	m := monitor.NewMachine()

	feed(m, "a", false, false, false) // a -> down
	feed(m, "b", true)                // b -> up

	if s := m.State("a"); s != monitor.StateDown {
		t.Errorf("a state = %v, want down", s)
	}
	if s := m.State("b"); s != monitor.StateUp {
		t.Errorf("b state = %v, want up", s)
	}
}
