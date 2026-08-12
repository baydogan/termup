package storage

import (
	"errors"
	"time"

	"github.com/baydogan/termup/monitor"
)

var ErrNotFound = errors.New("monitor not found")

type Reader interface {
	List() ([]monitor.Monitor, error)
	History(name string) ([]monitor.Result, error)
}

type Store interface {
	Reader
	Writer
}

type Writer interface {
	Save(monitor.Result) error
	Sync(monitors []monitor.Monitor) error
	// Prune drops results checked before the given time (retention).
	Prune(before time.Time) error
}
