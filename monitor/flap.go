package monitor

import "sync"

// Flapping thresholds (fixed, decision: 2026-08-11): over the last FlapWindow
// raw probe results, FlapThreshold or more up<->down flips means the target is
// flapping. This works on the raw per-probe up/down, not the debounced State:
// an oscillating target never settles to down, so the machine would miss it.
const (
	FlapWindow    = 10
	FlapThreshold = 5
)

// FlapTracker keeps a per-monitor sliding window of raw up/down results and
// fires once when the flip count crosses into flapping, re-arming when it
// settles. Pure and count-based (no clock). Mutex-guarded for the probe pool.
type FlapTracker struct {
	mu     sync.Mutex
	states map[string]*flapState
}

type flapState struct {
	window []bool
	warned bool
}

func NewFlapTracker() *FlapTracker {
	return &FlapTracker{states: make(map[string]*flapState)}
}

// Observe records one raw result (up=true for 2xx) and returns the current flip
// count plus whether a flapping alert should fire now (once per crossing).
func (f *FlapTracker) Observe(name string, up bool) (int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	st := f.states[name]
	if st == nil {
		st = &flapState{}
		f.states[name] = st
	}

	st.window = append(st.window, up)
	if len(st.window) > FlapWindow {
		st.window = st.window[len(st.window)-FlapWindow:]
	}

	flips := 0
	for i := 1; i < len(st.window); i++ {
		if st.window[i] != st.window[i-1] {
			flips++
		}
	}

	if flips >= FlapThreshold {
		if !st.warned {
			st.warned = true
			return flips, true
		}
	} else {
		st.warned = false // settled: re-arm
	}
	return flips, false
}
