package scheduler

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/baydogan/termup/monitor"
	"github.com/baydogan/termup/notify"
	"github.com/baydogan/termup/storage"
)

// proberFunc adapts a function to probe.Prober for table-ish tests.
type proberFunc func(m *monitor.Monitor) monitor.Result

func (f proberFunc) Probe(_ context.Context, m *monitor.Monitor) monitor.Result { return f(m) }

// recordingClock captures the durations passed to After (used to prove the
// jitter offsets), firing each immediately so runOnce doesn't really wait.
type recordingClock struct {
	mu   sync.Mutex
	durs []time.Duration
}

func (c *recordingClock) Now() time.Time                      { return time.Unix(0, 0) }
func (c *recordingClock) Tick(time.Duration) <-chan time.Time { return nil }
func (c *recordingClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	c.durs = append(c.durs, d)
	c.mu.Unlock()
	ch := make(chan time.Time, 1)
	ch <- time.Unix(0, 0)
	return ch
}
func (c *recordingClock) afters() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]time.Duration, len(c.durs))
	copy(out, c.durs)
	return out
}

// atClock is a fixed clock whose Now can be set (for cert day-math).
type atClock struct{ now time.Time }

func (c atClock) Now() time.Time                      { return c.now }
func (c atClock) Tick(time.Duration) <-chan time.Time { return nil }
func (c atClock) After(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- c.now
	return ch
}

func newTestScheduler(prober proberFunc, store storage.Store, n notify.Notifier, clk Clock, workers int) *Scheduler {
	return New(prober, store, monitor.NewMachine(), monitor.NewCertTracker(), monitor.NewFlapTracker(),
		n, clk, 30*time.Second, time.Second, workers)
}

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

// After fires immediately so jittered dispatch runs without real waiting.
func (fixedClock) After(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- time.Unix(0, 0)
	return ch
}

// recordingNotifier captures every event it is asked to notify. Notify may be
// called concurrently from the worker pool, so it is mutex-guarded.
type recordingNotifier struct {
	mu  sync.Mutex
	got []notify.Event
}

func (n *recordingNotifier) Notify(e notify.Event) error {
	n.mu.Lock()
	n.got = append(n.got, e)
	n.mu.Unlock()
	return nil
}

func TestOffsetIsDeterministicAndSpread(t *testing.T) {
	s := &Scheduler{interval: 30 * time.Second}

	// Deterministic: a second scheduler derives the same phase for the same name,
	// i.e. the offset comes from the name hash and not from per-instance state
	// (an RNG would drift here, and restarts would reshuffle the spread).
	restarted := &Scheduler{interval: 30 * time.Second}
	if got, want := restarted.offset("api-prod"), s.offset("api-prod"); got != want {
		t.Errorf("offset(api-prod) = %v on a second scheduler, want %v", got, want)
	}

	// In range [0, interval).
	names := []string{"api-prod", "web", "db", "cache", "queue", "auth"}
	seen := map[time.Duration]bool{}
	for _, n := range names {
		o := s.offset(n)
		if o < 0 || o >= 30*time.Second {
			t.Errorf("offset(%q) = %v, out of [0,30s)", n, o)
		}
		seen[o] = true
	}
	// Spread: names should not all collapse to one offset.
	if len(seen) < 2 {
		t.Errorf("offsets not spread: %d distinct for %d names", len(seen), len(names))
	}
}

func TestSchedulerNotifiesOnlyOnTransition(t *testing.T) {
	store := storage.New(monitor.Monitor{Name: "x", URL: "http://x"})
	prober := &scriptedProber{ups: []bool{false, false, false, true}}
	notifier := &recordingNotifier{}
	s := New(prober, store, monitor.NewMachine(), monitor.NewCertTracker(), monitor.NewFlapTracker(), notifier, fixedClock{}, time.Second, time.Second, 1)

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

func TestRunOnceDispatchesWithPerMonitorOffset(t *testing.T) {
	monitors := []monitor.Monitor{
		{Name: "a", URL: "http://a"},
		{Name: "b", URL: "http://b"},
		{Name: "c", URL: "http://c"},
	}
	store := storage.New(monitors...)
	clk := &recordingClock{}
	up := proberFunc(func(m *monitor.Monitor) monitor.Result {
		return monitor.Result{MonitorName: m.Name, StatusCode: 200}
	})
	s := newTestScheduler(up, store, &recordingNotifier{}, clk, 2)

	s.runOnce(context.Background())

	want := map[time.Duration]int{}
	for _, m := range monitors {
		want[s.offset(m.Name)]++
	}
	got := map[time.Duration]int{}
	for _, d := range clk.afters() {
		got[d]++
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("After offsets = %v, want %v", got, want)
	}
}

func TestRunOnceProbesEachMonitorOnce(t *testing.T) {
	monitors := []monitor.Monitor{{Name: "a", URL: "http://a"}, {Name: "b", URL: "http://b"}, {Name: "c", URL: "http://c"}}
	store := storage.New(monitors...)

	var mu sync.Mutex
	counts := map[string]int{}
	prober := proberFunc(func(m *monitor.Monitor) monitor.Result {
		mu.Lock()
		counts[m.Name]++
		mu.Unlock()
		return monitor.Result{MonitorName: m.Name, StatusCode: 200}
	})
	s := newTestScheduler(prober, store, &recordingNotifier{}, fixedClock{}, 2)

	s.runOnce(context.Background())

	for _, m := range monitors {
		if counts[m.Name] != 1 {
			t.Errorf("%s probed %d times, want 1", m.Name, counts[m.Name])
		}
	}
}

func TestSchedulerFiresCertExpiring(t *testing.T) {
	store := storage.New(monitor.Monitor{Name: "x", URL: "https://x"})
	notifier := &recordingNotifier{}
	now := time.Unix(1_700_000_000, 0)
	exp := now.Add((monitor.CertWarnDays - 1) * 24 * time.Hour) // below threshold
	prober := proberFunc(func(m *monitor.Monitor) monitor.Result {
		return monitor.Result{MonitorName: m.Name, StatusCode: 200, CertExpiry: exp}
	})
	s := newTestScheduler(prober, store, notifier, atClock{now}, 1)

	s.runOnce(context.Background())

	var cert *notify.Event
	for i := range notifier.got {
		if notifier.got[i].Kind == notify.KindCertExpiring {
			cert = &notifier.got[i]
		}
	}
	if cert == nil {
		t.Fatalf("no cert-expiring event; got %+v", notifier.got)
	}
	if cert.DaysLeft != monitor.CertWarnDays-1 {
		t.Errorf("DaysLeft = %d, want %d", cert.DaysLeft, monitor.CertWarnDays-1)
	}
}

func TestSchedulerFiresFlapping(t *testing.T) {
	store := storage.New(monitor.Monitor{Name: "x", URL: "http://x"})
	notifier := &recordingNotifier{}

	var mu sync.Mutex
	i := 0
	prober := proberFunc(func(m *monitor.Monitor) monitor.Result {
		mu.Lock()
		code := 500
		if i%2 == 0 {
			code = 200
		}
		i++
		mu.Unlock()
		return monitor.Result{MonitorName: m.Name, StatusCode: code}
	})
	s := newTestScheduler(prober, store, notifier, fixedClock{}, 1)

	for k := 0; k < monitor.FlapWindow; k++ {
		s.runOnce(context.Background())
	}

	found := false
	for _, e := range notifier.got {
		if e.Kind == notify.KindFlapping {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a flapping event after oscillation; got %+v", notifier.got)
	}
}

// controllableClock lets the test drive ticks; After fires immediately so the
// jittered dispatch inside runOnce doesn't actually wait.
type controllableClock struct{ tick chan time.Time }

func (c *controllableClock) Now() time.Time                      { return time.Unix(0, 0) }
func (c *controllableClock) Tick(time.Duration) <-chan time.Time { return c.tick }
func (c *controllableClock) After(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- time.Unix(0, 0)
	return ch
}

func waitProbe(t *testing.T, ch <-chan string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("expected a probe within 1s")
	}
}

func TestRunTicksThenStopsOnCancel(t *testing.T) {
	store := storage.New(monitor.Monitor{Name: "x", URL: "http://x"})
	probed := make(chan string, 16)
	prober := proberFunc(func(m *monitor.Monitor) monitor.Result {
		probed <- m.Name
		return monitor.Result{MonitorName: m.Name, StatusCode: 200}
	})
	clk := &controllableClock{tick: make(chan time.Time)}
	s := New(prober, store, monitor.NewMachine(), monitor.NewCertTracker(), monitor.NewFlapTracker(),
		&recordingNotifier{}, clk, 30*time.Second, time.Second, 1)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	// Run probes once immediately, then once per tick.
	waitProbe(t, probed) // initial runOnce
	clk.tick <- time.Unix(0, 0)
	waitProbe(t, probed) // tick 1
	clk.tick <- time.Unix(0, 0)
	waitProbe(t, probed) // tick 2

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}

// TestRunWaitsForInFlightProbeBeforeReturning is the guarantee the daemon's
// shutdown sequence relies on: once Run has returned, nothing writes to the
// store any more, so it is safe to close.
func TestRunWaitsForInFlightProbeBeforeReturning(t *testing.T) {
	store := storage.New(monitor.Monitor{Name: "x", URL: "http://x"})
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	prober := proberFunc(func(m *monitor.Monitor) monitor.Result {
		entered <- struct{}{}
		<-release // still probing while the context gets cancelled
		return monitor.Result{MonitorName: m.Name, StatusCode: 200}
	})
	clk := &controllableClock{tick: make(chan time.Time)}
	s := New(prober, store, monitor.NewMachine(), monitor.NewCertTracker(), monitor.NewFlapTracker(),
		&recordingNotifier{}, clk, 30*time.Second, time.Second, 1)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	<-entered
	cancel()

	select {
	case <-done:
		t.Fatal("Run returned while a probe was still in flight")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after the in-flight probe finished")
	}

	// The in-flight result was persisted before Run returned.
	if h, _ := store.History("x"); len(h) != 1 {
		t.Errorf("history len = %d, want 1", len(h))
	}
}
