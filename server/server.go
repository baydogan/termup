package server

import (
	"context"
	"errors"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/baydogan/termup/api"
	"github.com/baydogan/termup/config"
	"github.com/baydogan/termup/monitor"
	"github.com/baydogan/termup/notify"
	"github.com/baydogan/termup/probe"
	"github.com/baydogan/termup/scheduler"
	"github.com/baydogan/termup/storage"
	log "github.com/charmbracelet/log"
	"github.com/muesli/termenv"
)

const socketPath = "/tmp/termupd.sock"

const configPath = "config.yaml"

const dbPath = "termup.db"

const (
	probeInterval = 30 * time.Second
	probeTimeout  = 10 * time.Second
	probeWorkers  = 5
)

const (
	retentionAge   = 30 * 24 * time.Hour // drop results older than this
	retentionSweep = time.Hour           // how often the sweep runs
)

const (
	// notifyQueueSize is how many pending alerts the async notifier holds before
	// it starts dropping them (probing never waits on alerting).
	notifyQueueSize = 256
	// notifyDrainTimeout bounds the shutdown drain so termupd cannot hang behind
	// a slow provider.
	notifyDrainTimeout = 5 * time.Second
	// workerStopTimeout is how long shutdown waits for each store writer
	// (scheduler, retention sweep, config watcher) to return.
	workerStopTimeout = 5 * time.Second
)

func Start() {
	logger := initLogger()
	cfg := loadConfig()
	ln := initializeListener()
	store := initializeStorage(cfg)
	machine := monitor.NewMachine()
	certs := monitor.NewCertTracker()
	flaps := monitor.NewFlapTracker()
	notifier := initializeNotifier(cfg, logger)
	srv := initializeAPI(store, machine)
	sched := initializeScheduler(store, machine, certs, flaps, notifier)
	run(srv, sched, store, notifier, ln)
}

// initLogger configures the shared charmbracelet/log default logger: styled,
// timestamped, colored (forced so `docker compose logs` renders it). Level is
// info by default, overridable with LOG_LEVEL (e.g. debug).
func initLogger() *log.Logger {
	logger := log.Default()
	logger.SetReportTimestamp(true)
	logger.SetTimeFormat("15:04:05")
	logger.SetColorProfile(termenv.ANSI256)
	if lvl, err := log.ParseLevel(os.Getenv("LOG_LEVEL")); err == nil {
		logger.SetLevel(lvl)
	}
	return logger
}

func loadConfig() *config.Config {
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatal("load config", "err", err)
	}
	return cfg
}

func initializeScheduler(store storage.Store, machine *monitor.Machine, certs *monitor.CertTracker, flaps *monitor.FlapTracker, notifier notify.Notifier) *scheduler.Scheduler {
	prober := probe.NewHTTP(probeTimeout)
	clock := scheduler.RealClock()
	return scheduler.New(prober, store, machine, certs, flaps, notifier, clock, probeInterval, probeTimeout, probeWorkers)
}

// initializeNotifier builds the fan-out notifier: stdout is always on; provider
// adapters from config are added on top. Notifiers are boot-time only (the
// notifier section is not hot-reloaded). The whole chain is wrapped in
// notify.Async so a slow provider can never stall a probe worker; the caller
// must Close it to drain on shutdown.
func initializeNotifier(cfg *config.Config, logger *log.Logger) *notify.Async {
	notifiers := []notify.Notifier{notify.NewStdout(logger)}
	for _, nc := range cfg.Notifiers {
		n, err := notify.Build(nc.Type, nc.URL)
		if err != nil {
			log.Fatal("notifier config", "err", err)
		}
		notifiers = append(notifiers, n)
	}
	return notify.NewAsync(notify.NewMulti(notifiers...), notifyQueueSize)
}

func initializeStorage(cfg *config.Config) storage.Store {
	store, err := storage.NewSQLite(dbPath)
	if err != nil {
		log.Fatal("open storage", "err", err)
	}
	if err := store.Sync(cfg.ToMonitors()); err != nil {
		log.Fatal("seed storage", "err", err)
	}
	return store
}

func initializeListener() net.Listener {
	if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatal("remove stale socket", "err", err)
	}
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatal("listen unix socket", "err", err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		log.Fatal("chmod socket", "err", err)
	}
	return ln
}

func initializeAPI(store storage.Reader, machine *monitor.Machine) *api.API {
	return api.New(store, machine)
}

// retentionLoop periodically drops results older than retentionAge. Runs once
// at startup, then on retentionSweep ticks. Uses real time (coarse maintenance).
func retentionLoop(ctx context.Context, store storage.Store) {
	prune := func() {
		if err := store.Prune(time.Now().Add(-retentionAge)); err != nil {
			log.Error("retention prune", "err", err)
		}
	}
	prune()

	t := time.NewTicker(retentionSweep)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			prune()
		}
	}
}

// start runs fn in a goroutine and returns a channel that closes when it returns.
func start(fn func()) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	return done
}

// waitFor blocks until a shutdown worker signals it is done, or until the grace
// period runs out. A worker that overstays is reported and left behind: the
// process is going down either way.
func waitFor(what string, done <-chan struct{}) {
	select {
	case <-done:
	case <-time.After(workerStopTimeout):
		log.Warn("shutdown: worker did not stop in time", "worker", what)
	}
}

// watchConfig re-seeds the store whenever config.yaml changes. Returns when ctx
// is cancelled (or the watcher itself fails).
func watchConfig(ctx context.Context, store storage.Store) {
	err := config.Watch(ctx, configPath,
		func(c *config.Config) {
			if err := store.Sync(c.ToMonitors()); err != nil {
				log.Error("config reload: sync failed", "err", err)
				return
			}
			log.Info("config reloaded", "monitors", len(c.Monitors))
		},
		func(err error) {
			log.Warn("config reload skipped (keeping previous list)", "err", err)
		},
	)
	if err != nil {
		log.Warn("config watcher stopped", "err", err)
	}
}

func run(srv *api.API, sched *scheduler.Scheduler, store storage.Store, notifier *notify.Async, ln net.Listener) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Every goroutine that writes to the store reports when it is done, so the
	// store can be closed only after the last writer has stopped.
	schedDone := start(func() { sched.Run(ctx) })
	retentionDone := start(func() { retentionLoop(ctx, store) })
	watchDone := start(func() { watchConfig(ctx, store) })

	log.Info("termupd listening", "socket", socketPath)

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		if err != nil {
			log.Fatal("server error", "err", err)
		}
	case <-ctx.Done():
		log.Info("shutdown signal received, shutting down gracefully")
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shCtx); err != nil {
			log.Error("shutdown error", "err", err)
		}
		// Stop the store's writers first: in-flight probes, the retention sweep
		// and a config reload all write, and a write after Close just errors.
		waitFor("scheduler", schedDone)
		waitFor("retention sweep", retentionDone)
		waitFor("config watcher", watchDone)

		// Deliver the alerts still queued. Bounded: a stuck provider must not
		// hold the process open.
		drainCtx, cancelDrain := context.WithTimeout(context.Background(), notifyDrainTimeout)
		defer cancelDrain()
		if err := notifier.Close(drainCtx); err != nil {
			log.Warn("notifier drain", "err", err)
		}

		if err := store.Close(); err != nil {
			log.Error("store close", "err", err)
		}
		log.Info("termupd stopped")
	}
}
