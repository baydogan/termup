package storage

import (
	"sync"

	"github.com/baydogan/termup/monitor"
)

// historySize is how many recent results we keep per monitor (in-memory ring
// buffer). At a 30s probe interval this is ~30 min of history — enough for the
// dashboard bar. Persistence/retention is a later phase.
const historySize = 60

type Memory struct {
	mu       sync.RWMutex
	monitors []monitor.Monitor
	history  map[string][]monitor.Result
}

func New(seed ...monitor.Monitor) *Memory {
	return &Memory{monitors: seed, history: make(map[string][]monitor.Result)}
}

func (s *Memory) List() ([]monitor.Monitor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]monitor.Monitor, len(s.monitors))
	copy(out, s.monitors)
	return out, nil
}

func (s *Memory) GetStatus(name string) (monitor.Status, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h := s.history[name]
	if len(h) == 0 {
		return monitor.Status{}, ErrNotFound
	}
	return monitor.Status{State: h[len(h)-1].State()}, nil
}

func (s *Memory) History(name string) ([]monitor.Result, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h := s.history[name]
	out := make([]monitor.Result, len(h))
	copy(out, h)
	return out, nil
}

func (s *Memory) Save(r monitor.Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	h := append(s.history[r.MonitorName], r)
	if len(h) > historySize {
		h = h[len(h)-historySize:]
	}
	s.history[r.MonitorName] = h
	return nil
}

func (s *Memory) Sync(monitors []monitor.Monitor) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.monitors = monitors

	keep := make(map[string]struct{}, len(monitors))
	for _, m := range monitors {
		keep[m.Name] = struct{}{}
	}
	for name := range s.history {
		if _, ok := keep[name]; !ok {
			delete(s.history, name)
		}
	}
	return nil
}
