package notify

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/charmbracelet/log"
)

var _ Notifier = (*Async)(nil)

var (
	// ErrQueueFull means the event was dropped: the queue was full, and dropping
	// the alarm is preferred over stalling the probe that reported it.
	ErrQueueFull = errors.New("notify: queue full, event dropped")
	// ErrClosed means Close already ran, so the event was not queued.
	ErrClosed = errors.New("notify: notifier closed")
)

// Async decouples alerting from probing: Notify only enqueues, and a single
// worker goroutine delivers to the wrapped notifier. Without it a slow webhook
// blocks the probe worker that produced the event (the pool is small, and each
// provider has a 10s timeout).
//
// One worker, so delivery order is preserved. A full queue drops the event
// (decision: 2026-08-14) — probing must never wait on alerting. Close drains
// what is already queued.
type Async struct {
	child Notifier
	queue chan Event
	done  chan struct{} // closed once the worker has drained and exited

	mu     sync.RWMutex // guards closed; RLock on the (non-blocking) send path
	closed bool
}

// NewAsync wraps child with a queue of the given size and starts the worker.
// The caller owns the capacity choice and must Close to drain.
func NewAsync(child Notifier, queueSize int) *Async {
	a := &Async{
		child: child,
		queue: make(chan Event, queueSize),
		done:  make(chan struct{}),
	}
	go a.run()
	return a
}

// Notify enqueues the event and returns immediately. It never blocks: a full
// queue yields ErrQueueFull so the caller can report the dropped alarm.
func (a *Async) Notify(e Event) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.closed {
		return ErrClosed
	}
	select {
	case a.queue <- e:
		return nil
	default:
		return ErrQueueFull
	}
}

// Close stops accepting events and blocks until the queued ones are delivered or
// ctx expires. On expiry the worker is left to finish on its own (the process is
// shutting down anyway), so the caller's deadline stays authoritative.
func (a *Async) Close(ctx context.Context) error {
	a.mu.Lock()
	if !a.closed {
		a.closed = true
		close(a.queue)
	}
	a.mu.Unlock()

	select {
	case <-a.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("notify: drain incomplete: %w", ctx.Err())
	}
}

// run delivers queued events one at a time. Errors are logged here: by this
// point the producer is long gone, so there is no caller left to report to.
func (a *Async) run() {
	defer close(a.done)
	for e := range a.queue {
		if err := a.child.Notify(e); err != nil {
			log.Error("async notify failed", "monitor", e.Monitor, "err", err)
		}
	}
}
