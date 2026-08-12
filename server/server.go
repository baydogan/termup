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
	run(srv, sched, store, ln)
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
// notifier section is not hot-reloaded).
func initializeNotifier(cfg *config.Config, logger *log.Logger) notify.Notifier {
	notifiers := []notify.Notifier{notify.NewStdout(logger)}
	for _, nc := range cfg.Notifiers {
		n, err := notify.Build(nc.Type, nc.URL)
		if err != nil {
			log.Fatal("notifier config", "err", err)
		}
		notifiers = append(notifiers, n)
	}
	return notify.NewMulti(notifiers...)
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

func run(srv *api.API, sched *scheduler.Scheduler, store storage.Store, ln net.Listener) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go sched.Run(ctx)
	go retentionLoop(ctx, store)

	go func() {
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
	}()

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
		log.Info("termupd stopped")
	}
}
