package storage

import (
	"sync"

	"github.com/baydogan/zerotolerance/monitor"
)

type Memory struct {
	mu       sync.RWMutex
	monitors []monitor.Monitor
	statuses map[string]monitor.Status
}

func New() *Memory {
	return &Memory{statuses: make(map[string]monitor.Status)}
}

func (s *Memory) List() []monitor.Monitor {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]monitor.Monitor, len(s.monitors))
	copy(out, s.monitors)
	return out
}

func (s *Memory) GetStatus(name string) (monitor.Status, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.statuses[name]
	if !ok {
		return monitor.Status{}, ErrNotFound
	}
	return st, nil
}

func (s *Memory) Save(r monitor.Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return nil
}
