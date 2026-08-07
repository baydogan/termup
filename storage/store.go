package storage

import (
	"errors"

	"github.com/baydogan/termup/monitor"
)

var ErrNotFound = errors.New("monitor not found")

type Reader interface {
	List() ([]monitor.Monitor, error)
	GetStatus(name string) (monitor.Status, error)
	History(name string) ([]monitor.Result, error)
}

type Store interface {
	Reader
	Writer
}

type Writer interface {
	Save(monitor.Result) error
	Sync(monitors []monitor.Monitor) error
}
