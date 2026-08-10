package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/baydogan/termup/monitor"
	"github.com/baydogan/termup/storage"
)

// scriptedProber returns a predetermined up/down result per call.
type scriptedProber struct {
	ups []bool
	i   int
}

func (p *scriptedProber) Probe(_ context.Context, m *monitor.Monitor) monitor.Result {
	up := p.ups[p.i]
	p.i++
	code := 500
	if up {
		code = 200
	}
	return monitor.Result{MonitorName: m.Name, StatusCode: code}
}

type fixedClock struct{}

func (fixedClock) Now() time.Time                      { return time.Unix(0, 0) }
func (fixedClock) Tick(time.Duration) <-chan time.Time { return nil }

// recordingNotifier captures every transition it is asked to notify.
type recordingNotifier struct {
	got []monitor.Transition
}

func (n *recordingNotifier) Notify(t monitor.Transition) error {
	n.got = append(n.got, t)
	return nil
}

func TestSchedulerNotifiesOnlyOnTransition(t *testing.T) {
	store := storage.New(monitor.Monitor{Name: "x", URL: "http://x"})
	prober := &scriptedProber{ups: []bool{false, false, false, true}}
	notifier := &recordingNotifier{}
	s := New(prober, store, monitor.NewMachine(), notifier, fixedClock{}, time.Second, time.Second, 1)

	ctx := context.Background()

	// Two fails: below the threshold, no alert.
	s.runOnce(ctx)
	s.runOnce(ctx)
	if len(notifier.got) != 0 {
		t.Fatalf("after 2 fails: %d notifications, want 0", len(notifier.got))
	}

	// Third consecutive fail: unknown -> down.
	s.runOnce(ctx)
	if len(notifier.got) != 1 {
		t.Fatalf("after 3 fails: %d notifications, want 1", len(notifier.got))
	}
	if notifier.got[0].To != monitor.StateDown {
		t.Errorf("first transition To = %v, want down", notifier.got[0].To)
	}

	// Single success: down -> up.
	s.runOnce(ctx)
	if len(notifier.got) != 2 {
		t.Fatalf("after recovery: %d notifications, want 2", len(notifier.got))
	}
	if notifier.got[1].To != monitor.StateUp {
		t.Errorf("second transition To = %v, want up", notifier.got[1].To)
	}
}
