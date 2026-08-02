package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/baydogan/zerotolerance/monitor"
	"github.com/baydogan/zerotolerance/probe"
	"github.com/baydogan/zerotolerance/storage"
)

type Clock interface {
	Now() time.Time
	Tick(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                        { return time.Now() }
func (realClock) Tick(d time.Duration) <-chan time.Time { return time.Tick(d) }

func RealClock() Clock { return realClock{} }

type Scheduler struct {
	prober   probe.Prober
	store    storage.Store
	clock    Clock
	interval time.Duration
	timeout  time.Duration
	workers  int
}

func New(p probe.Prober, s storage.Store, c Clock, interval, timeout time.Duration, workers int) *Scheduler {
	return &Scheduler{prober: p, store: s, clock: c, interval: interval, timeout: timeout, workers: workers}
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
	monitors := s.store.List()

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

	for _, m := range monitors {
		jobs <- m
	}
	close(jobs)
	wg.Wait()
}

func (s *Scheduler) probeOne(ctx context.Context, m monitor.Monitor) {
	pctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	res := s.prober.Probe(pctx, &m)
	res.CheckedAt = s.clock.Now()
	if err := s.store.Save(res); err != nil {
		log.Printf("probe %s: save failed: %v", m.Name, err)
	}
	log.Printf("probe %-10s -> %-4s (code=%d, %s)",
		m.Name, res.State(), res.StatusCode, res.Latency.Round(time.Millisecond))
}
