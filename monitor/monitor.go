package monitor

import "time"

type Monitor struct {
	Name string
	URL  string
	// later: Interval, Method, FailThreshold, ...
}

type State int

const (
	StateUnknown State = iota
	StateUp
	StateDown
)

func (s State) String() string {
	switch s {
	case StateUp:
		return "up"
	case StateDown:
		return "down"
	default:
		return "unknown"
	}
}

type Status struct {
	State State
}

type Result struct {
	MonitorName string
	StatusCode  int
	Latency     time.Duration
	Err         error
	CheckedAt   time.Time
	// CertExpiry is the TLS leaf certificate's NotAfter for HTTPS targets.
	// Zero when the target is plain HTTP or the handshake never completed.
	CertExpiry time.Time
}

func (r Result) State() State {
	if r.Err == nil && r.StatusCode >= 200 && r.StatusCode < 300 {
		return StateUp
	}
	return StateDown
}
