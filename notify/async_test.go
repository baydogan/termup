package notify_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/baydogan/termup/notify"
)

// blockingNotifier records the events it receives and can be held on a gate, so
// a test can simulate a slow provider.
type blockingNotifier struct {
	gate chan struct{} // receive before handling each event; nil = never blocks
	err  error

	mu  sync.Mutex
	got []notify.Event
}

func (b *blockingNotifier) Notify(e notify.Event) error {
	if b.gate != nil {
		<-b.gate
	}
	b.mu.Lock()
	b.got = append(b.got, e)
	b.mu.Unlock()
	return b.err
}

func (b *blockingNotifier) events() []notify.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]notify.Event, len(b.got))
	copy(out, b.got)
	return out
}

func event(name string) notify.Event {
	return notify.Event{Kind: notify.KindStateChange, Monitor: name}
}

func closeAsync(t *testing.T, a *notify.Async) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Close(ctx); err != nil {
		t.Fatalf("close: %v", err)
	}
}

// TestAsyncNotifyDoesNotWaitOnChild is the point of the wrapper: the caller (a
// probe worker) returns even though the child is stuck.
func TestAsyncNotifyDoesNotWaitOnChild(t *testing.T) {
	child := &blockingNotifier{gate: make(chan struct{})} // stuck until released
	a := notify.NewAsync(child, 4)

	returned := make(chan struct{})
	go func() {
		for i := 0; i < 4; i++ {
			if err := a.Notify(event("m")); err != nil {
				t.Errorf("notify %d: %v", i, err)
			}
		}
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("Notify blocked behind the child")
	}

	close(child.gate) // let the worker drain
	closeAsync(t, a)
	if got := len(child.events()); got != 4 {
		t.Errorf("delivered %d events, want 4", got)
	}
}

func TestAsyncPreservesOrder(t *testing.T) {
	child := &blockingNotifier{}
	a := notify.NewAsync(child, 8)

	want := []string{"a", "b", "c", "d"}
	for _, name := range want {
		if err := a.Notify(event(name)); err != nil {
			t.Fatalf("notify %s: %v", name, err)
		}
	}
	closeAsync(t, a)

	got := child.events()
	if len(got) != len(want) {
		t.Fatalf("delivered %d events, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i].Monitor != name {
			t.Errorf("event %d = %q, want %q (single worker must keep order)", i, got[i].Monitor, name)
		}
	}
}

// TestAsyncFullQueueDropsEvent pins the decision: a full queue loses the alarm
// rather than stalling the probe that produced it.
func TestAsyncFullQueueDropsEvent(t *testing.T) {
	child := &blockingNotifier{gate: make(chan struct{})}
	a := notify.NewAsync(child, 1)

	// The worker picks up the first event and blocks on the gate; the queue slot
	// it frees is taken by the second. The third has nowhere to go.
	if err := a.Notify(event("first")); err != nil {
		t.Fatalf("first: %v", err)
	}
	var dropped int
	for i := 0; i < 8; i++ {
		if err := a.Notify(event("filler")); errors.Is(err, notify.ErrQueueFull) {
			dropped++
		}
	}
	if dropped == 0 {
		t.Fatal("no event was dropped; a full queue must not block the caller")
	}

	close(child.gate)
	closeAsync(t, a)
	if got := len(child.events()); got > 2 {
		t.Errorf("child saw %d events, want at most 2 (queue size 1 + in-flight)", got)
	}
}

func TestAsyncCloseDrainsQueue(t *testing.T) {
	child := &blockingNotifier{}
	a := notify.NewAsync(child, 16)

	for i := 0; i < 10; i++ {
		if err := a.Notify(event("m")); err != nil {
			t.Fatalf("notify %d: %v", i, err)
		}
	}
	closeAsync(t, a) // must block until all 10 are delivered

	if got := len(child.events()); got != 10 {
		t.Errorf("delivered %d events after Close, want 10", got)
	}
}

func TestAsyncCloseGivesUpOnDeadline(t *testing.T) {
	child := &blockingNotifier{gate: make(chan struct{})} // never released
	a := notify.NewAsync(child, 4)
	if err := a.Notify(event("m")); err != nil {
		t.Fatalf("notify: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := a.Close(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close err = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Close took %v; it must honour the caller's deadline", elapsed)
	}
}

func TestAsyncNotifyAfterCloseIsRejected(t *testing.T) {
	child := &blockingNotifier{}
	a := notify.NewAsync(child, 4)
	closeAsync(t, a)

	if err := a.Notify(event("late")); !errors.Is(err, notify.ErrClosed) {
		t.Errorf("Notify after Close = %v, want ErrClosed", err)
	}
	if got := len(child.events()); got != 0 {
		t.Errorf("child saw %d events, want 0", got)
	}
	// Close is idempotent (shutdown paths may call it twice).
	closeAsync(t, a)
}

// TestAsyncConcurrentNotifyAndClose covers the shutdown race: probes may still
// be reporting while Close runs. Meaningful under -race.
func TestAsyncConcurrentNotifyAndClose(t *testing.T) {
	child := &blockingNotifier{}
	a := notify.NewAsync(child, 32)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				// Either queued, dropped or rejected — never a panic.
				if err := a.Notify(event("m")); err != nil &&
					!errors.Is(err, notify.ErrQueueFull) && !errors.Is(err, notify.ErrClosed) {
					t.Errorf("unexpected notify error: %v", err)
				}
			}
		}()
	}
	closeAsync(t, a)
	wg.Wait()
}

// TestAsyncChildErrorDoesNotStopTheWorker: a failing provider must not kill the
// delivery loop.
func TestAsyncChildErrorDoesNotStopTheWorker(t *testing.T) {
	child := &blockingNotifier{err: errors.New("provider down")}
	a := notify.NewAsync(child, 8)

	for i := 0; i < 3; i++ {
		if err := a.Notify(event("m")); err != nil {
			t.Fatalf("notify %d: %v", i, err)
		}
	}
	closeAsync(t, a)

	if got := len(child.events()); got != 3 {
		t.Errorf("delivered %d events, want 3", got)
	}
}
