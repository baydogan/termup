package storage

import (
	"errors"

	"github.com/baydogan/zerotolerance/monitor"
)

var ErrNotFound = errors.New("monitor not found")

type Reader interface {
	List() []monitor.Monitor
	GetStatus(name string) (monitor.Status, error)
}

type Store interface {
	Reader
	Writer
}

type Writer interface {
	Save(monitor.Result) error
	Sync(monitors []monitor.Monitor)
}
