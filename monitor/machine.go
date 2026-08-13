package monitor

import "sync"

// FailThreshold is the number of consecutive failed probes required before a
// monitor is declared down. Recovery is asymmetric: a single successful (2xx)
// probe flips it back up. Fixed, not configurable (decision: 2026-08-10).
const FailThreshold = 3

// Transition is emitted when a monitor's State changes. The notifier consumes it.
// When Observe reports changed=false the zero-value Transition is meaningless.
type Transition struct {
	Monitor string
	From    State
	To      State
	Result  Result
}

// Machine holds per-monitor State and the consecutive-fail counter. Pure: no I/O.
// Access is mutex-guarded because it is called from the concurrent probe pool.
//
// In-memory and process-local: on restart it starts fresh (Unknown) and
// re-derives State from observations. Accepted cost in v0.
type Machine struct {
	mu    sync.Mutex
	track map[string]tracker
}

type tracker struct {
	state      State
	failStreak int
}

func NewMachine() *Machine {
	return &Machine{track: make(map[string]tracker)}
}

// Observe processes one probe result and returns the transition when State
// changed *and* the change is worth alarming on (changed=true). Rules:
// up->down = FailThreshold consecutive fails; down->up = a single 2xx. Unknown
// is the initial state: a single 2xx -> up, FailThreshold fails -> down.
//
// unknown->up is silent (decision: 2026-08-13): the machine is process-local, so
// every restart re-derives State from Unknown and would otherwise alarm for
// every healthy target. State still becomes Up — only the alert is dropped.
// unknown->down stays loud: a target that is down at startup is news.
func (m *Machine) Observe(r Result) (Transition, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	t := m.track[r.MonitorName] // zero value: Unknown, failStreak 0
	from := t.state

	if r.State() == StateUp {
		t.failStreak = 0
		t.state = StateUp
	} else if t.state != StateDown {
		// Count fails only while not already down; drop once the threshold hits.
		t.failStreak++
		if t.failStreak >= FailThreshold {
			t.state = StateDown
		}
	}

	m.track[r.MonitorName] = t

	if t.state == from {
		return Transition{}, false
	}
	if from == StateUnknown && t.state == StateUp {
		return Transition{}, false // first observation of a healthy target: not an alarm
	}
	return Transition{Monitor: r.MonitorName, From: from, To: t.state, Result: r}, true
}

// State returns a monitor's current (debounced) State.
// Unknown if it has never been observed.
func (m *Machine) State(name string) State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.track[name].state
}
