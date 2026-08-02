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

	"github.com/baydogan/zerotolerance/api"
	"github.com/baydogan/zerotolerance/monitor"
	"github.com/baydogan/zerotolerance/probe"
	"github.com/baydogan/zerotolerance/scheduler"
	"github.com/baydogan/zerotolerance/storage"
)

const socketPath = "/tmp/ztd.sock"

const (
	probeInterval = 30 * time.Second
	probeTimeout  = 10 * time.Second
	probeWorkers  = 5
)

func Start() {
	ln := initializeListener()
	store := initializeStorage()
	srv := initializeAPI(store)
	sched := initializeScheduler(store)
	run(srv, sched, ln)
}

func initializeScheduler(store storage.Store) *scheduler.Scheduler {
	prober := probe.NewHTTP(probeTimeout)
	clock := scheduler.RealClock()
	return scheduler.New(prober, store, clock, probeInterval, probeTimeout, probeWorkers)
}

func initializeStorage() storage.Store {
	store := storage.New(
		monitor.Monitor{Name: "google", URL: "https://google.com"},
		monitor.Monitor{Name: "example", URL: "https://example.com"},
	)
	//TODO false data will be remove
	_ = store.Save(monitor.Result{MonitorName: "google", StatusCode: 200})
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

func initializeAPI(store storage.Reader) *api.API {
	return api.New(store)
}

func run(srv *api.API, sched *scheduler.Scheduler, ln net.Listener) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go sched.Run(ctx)

	log.Printf("ztd listening on unix socket %s", socketPath)

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
		log.Println("ztd stopped")
	}
}
