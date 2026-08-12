package scheduler

import (
	"context"
	"hash/fnv"
	"sync"
	"time"

	"github.com/baydogan/termup/monitor"
	"github.com/baydogan/termup/notify"
	"github.com/baydogan/termup/probe"
	"github.com/baydogan/termup/storage"
	log "github.com/charmbracelet/log"
)

type Clock interface {
	Now() time.Time
	Tick(d time.Duration) <-chan time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) Tick(d time.Duration) <-chan time.Time  { return time.Tick(d) }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

func RealClock() Clock { return realClock{} }

type Scheduler struct {
	prober   probe.Prober
	store    storage.Store
	machine  *monitor.Machine
	certs    *monitor.CertTracker
	flaps    *monitor.FlapTracker
	notifier notify.Notifier
	clock    Clock
	interval time.Duration
	timeout  time.Duration
	workers  int
}

func New(p probe.Prober, s storage.Store, m *monitor.Machine, ct *monitor.CertTracker, ft *monitor.FlapTracker, n notify.Notifier, c Clock, interval, timeout time.Duration, workers int) *Scheduler {
	return &Scheduler{prober: p, store: s, machine: m, certs: ct, flaps: ft, notifier: n, clock: c, interval: interval, timeout: timeout, workers: workers}
}

func (s *Scheduler) Run(ctx context.Context) {
	tick := s.clock.Tick(s.interval)
	s.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			s.runOnce(ctx)
		}
	}
}

func (s *Scheduler) runOnce(ctx context.Context) {
	monitors, err := s.store.List()
	if err != nil {
		log.Error("list monitors", "err", err)
		return
	}

	jobs := make(chan monitor.Monitor)
	var wg sync.WaitGroup
	for i := 0; i < s.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for m := range jobs {
				s.probeOne(ctx, m)
			}
		}()
	}

	// Jitter: dispatch each monitor after its deterministic phase offset so the
	// batch spreads across the interval instead of firing all at once. The
	// worker pool still caps concurrency.
	var dispatch sync.WaitGroup
	for _, m := range monitors {
		dispatch.Add(1)
		go func(m monitor.Monitor) {
			defer dispatch.Done()
			select {
			case <-ctx.Done():
				return
			case <-s.clock.After(s.offset(m.Name)):
			}
			select {
			case <-ctx.Done():
			case jobs <- m:
			}
		}(m)
	}
	dispatch.Wait()
	close(jobs)
	wg.Wait()
}

// offset is a monitor's stable phase within the interval, derived from its name
// hash so the spread is deterministic (no RNG) and stable across restarts.
func (s *Scheduler) offset(name string) time.Duration {
	if s.interval <= 0 {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	return time.Duration(h.Sum64() % uint64(s.interval))
}

func (s *Scheduler) probeOne(ctx context.Context, m monitor.Monitor) {
	pctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	res := s.prober.Probe(pctx, &m)
	res.CheckedAt = s.clock.Now()
	if err := s.store.Save(res); err != nil {
		log.Error("save result", "monitor", m.Name, "err", err)
	}
	log.Info("probe",
		"monitor", m.Name, "state", res.State(), "code", res.StatusCode,
		"latency", res.Latency.Round(time.Millisecond))

	// Feed the result into the state machine; a transition is alarm-worthy.
	// The notifier reports the transition (no duplicate log line here).
	if t, ok := s.machine.Observe(res); ok {
		s.notify(notify.Event{
			Kind:    notify.KindStateChange,
			Monitor: t.Monitor,
			From:    t.From,
			To:      t.To,
			Result:  t.Result,
		})
	}

	// Cert expiry: scheduler owns the clock and computes days-left; the tracker
	// decides whether this crossing warrants a one-shot alert.
	if !res.CertExpiry.IsZero() {
		daysLeft := int(res.CertExpiry.Sub(res.CheckedAt).Hours() / 24)
		if s.certs.Observe(m.Name, daysLeft, true) {
			s.notify(notify.Event{
				Kind:       notify.KindCertExpiring,
				Monitor:    m.Name,
				CertExpiry: res.CertExpiry,
				DaysLeft:   daysLeft,
			})
		}
	}

	// Flapping: feed the raw up/down into the tracker; it fires once when the
	// target starts oscillating (which the debounced machine would miss).
	if flips, ok := s.flaps.Observe(m.Name, res.State() == monitor.StateUp); ok {
		s.notify(notify.Event{Kind: notify.KindFlapping, Monitor: m.Name, Flips: flips})
	}
}

func (s *Scheduler) notify(e notify.Event) {
	if err := s.notifier.Notify(e); err != nil {
		log.Error("notify", "monitor", e.Monitor, "err", err)
	}
}
