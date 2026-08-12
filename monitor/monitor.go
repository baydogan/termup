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
	// Stage timings from httptrace (zero when not applicable: DNS for an IP
	// literal, TLS for plain HTTP, or a reused connection). Latency is the total.
	DNS     time.Duration // DNS lookup
	Connect time.Duration // TCP connect
	TLS     time.Duration // TLS handshake
	TTFB    time.Duration // request written -> first response byte (server wait)
}

func (r Result) State() State {
	if r.Err == nil && r.StatusCode >= 200 && r.StatusCode < 300 {
		return StateUp
	}
	return StateDown
}
