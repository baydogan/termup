package server

import (
	"context"
	"errors"
	"log"
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
)

const socketPath = "/tmp/termupd.sock"

const configPath = "config.yaml"

const dbPath = "termup.db"

const (
	probeInterval = 30 * time.Second
	probeTimeout  = 10 * time.Second
	probeWorkers  = 5
)

func Start() {
	cfg := loadConfig()
	ln := initializeListener()
	store := initializeStorage(cfg)
	machine := monitor.NewMachine()
	certs := monitor.NewCertTracker()
	flaps := monitor.NewFlapTracker()
	notifier := initializeNotifier(cfg)
	srv := initializeAPI(store, machine)
	sched := initializeScheduler(store, machine, certs, flaps, notifier)
	run(srv, sched, store, ln)
}

func loadConfig() *config.Config {
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
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
func initializeNotifier(cfg *config.Config) notify.Notifier {
	notifiers := []notify.Notifier{notify.NewStdout()}
	for _, nc := range cfg.Notifiers {
		n, err := notify.Build(nc.Type, nc.URL)
		if err != nil {
			log.Fatalf("notifier config: %v", err)
		}
		notifiers = append(notifiers, n)
	}
	return notify.NewMulti(notifiers...)
}

func initializeStorage(cfg *config.Config) storage.Store {
	store, err := storage.NewSQLite(dbPath)
	if err != nil {
		log.Fatalf("open storage: %v", err)
	}
	if err := store.Sync(cfg.ToMonitors()); err != nil {
		log.Fatalf("seed storage: %v", err)
	}
	return store
}

func initializeListener() net.Listener {
	if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("remove stale socket: %v", err)
	}
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("listen unix socket: %v", err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		log.Fatalf("chmod socket: %v", err)
	}
	return ln
}

func initializeAPI(store storage.Reader, machine *monitor.Machine) *api.API {
	return api.New(store, machine)
}

func run(srv *api.API, sched *scheduler.Scheduler, store storage.Store, ln net.Listener) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go sched.Run(ctx)

	go func() {
		err := config.Watch(ctx, configPath,
			func(c *config.Config) {
				if err := store.Sync(c.ToMonitors()); err != nil {
					log.Printf("config reload: sync failed: %v", err)
					return
				}
				log.Printf("config reloaded: %d monitors", len(c.Monitors))
			},
			func(err error) {
				log.Printf("config reload skipped: %v", err) // previous list is kept
			},
		)
		if err != nil {
			log.Printf("config watcher stopped: %v", err)
		}
	}()

	log.Printf("termupd listening on unix socket %s", socketPath)

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		if err != nil {
			log.Fatalf("server error: %v", err)
		}
	case <-ctx.Done():
		log.Println("shutdown signal received, shutting down gracefully...")
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shCtx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
		log.Println("termupd stopped")
	}
}
