package monitor

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
	Up          bool
}
