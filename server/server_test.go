package server

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/baydogan/termup/api"
	"github.com/baydogan/termup/config"
	"github.com/baydogan/termup/monitor"
	"github.com/baydogan/termup/notify"
	"github.com/baydogan/termup/probe"
	"github.com/baydogan/termup/scheduler"
	"github.com/baydogan/termup/storage"
	log "github.com/charmbracelet/log"
)

// writeConfig drops a config.yaml into dir and makes dir the working directory,
// so the relative configPath/dbPath constants resolve inside the test sandbox.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, configPath), []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Chdir(dir)
	return dir
}

const oneMonitor = `
monitors:
  - name: a
    url: http://a.example
`

func TestInitLoggerLevelFromEnv(t *testing.T) {
	orig := log.Default().GetLevel()
	t.Cleanup(func() { log.Default().SetLevel(orig) })

	tests := []struct {
		name string
		env  string
		want log.Level
	}{
		{"debug", "debug", log.DebugLevel},
		{"error", "error", log.ErrorLevel},
		{"unset keeps current", "", log.InfoLevel},
		{"garbage keeps current", "not-a-level", log.InfoLevel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log.Default().SetLevel(log.InfoLevel)
			t.Setenv("LOG_LEVEL", tt.env)

			logger := initLogger()
			if logger.GetLevel() != tt.want {
				t.Errorf("level = %v, want %v", logger.GetLevel(), tt.want)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	writeConfig(t, oneMonitor)

	cfg := loadConfig()
	if len(cfg.Monitors) != 1 || cfg.Monitors[0].Name != "a" {
		t.Fatalf("monitors = %+v, want one named a", cfg.Monitors)
	}
}

func TestInitializeStorageSeedsConfigMonitors(t *testing.T) {
	dir := writeConfig(t, oneMonitor)
	cfg := loadConfig()

	store := initializeStorage(cfg)

	ms, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ms) != 1 || ms[0].Name != "a" || ms[0].URL != "http://a.example" {
		t.Errorf("seeded monitors = %+v, want config list", ms)
	}
	if _, err := os.Stat(filepath.Join(dir, dbPath)); err != nil {
		t.Errorf("db file not created: %v", err)
	}
}

func TestInitializeNotifierFansOutToConfiguredProviders(t *testing.T) {
	var mu sync.Mutex
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		hits = append(hits, string(body))
		mu.Unlock()
	}))
	defer srv.Close()

	cfg := &config.Config{Notifiers: []config.NotifierConfig{
		{Type: "slack", URL: srv.URL},
		{Type: "discord", URL: srv.URL},
	}}

	n := initializeNotifier(cfg, log.New(io.Discard))
	if err := n.Notify(notify.Event{Kind: notify.KindStateChange, Monitor: "a", To: monitor.StateDown}); err != nil {
		t.Fatalf("notify: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(hits) != 2 {
		t.Fatalf("provider hits = %d (%v), want 2", len(hits), hits)
	}
	// stdout is implicit and added on top of the config providers, so the two
	// hits above prove fan-out reached both configured adapters.
	if hits[0] == hits[1] {
		t.Errorf("slack and discord sent identical payloads: %q", hits[0])
	}
}

func TestInitializeNotifierWithoutConfigIsStdoutOnly(t *testing.T) {
	n := initializeNotifier(&config.Config{}, log.New(io.Discard))
	if err := n.Notify(notify.Event{Kind: notify.KindFlapping, Monitor: "a", Flips: 6}); err != nil {
		t.Fatalf("notify: %v", err)
	}
}

// TestInitializeListener binds the real socket path, so it is skipped when a
// daemon is already listening there (removing that socket would cut it off).
func TestInitializeListener(t *testing.T) {
	if c, err := net.DialTimeout("unix", socketPath, 200*time.Millisecond); err == nil {
		c.Close()
		t.Skipf("a daemon is listening on %s", socketPath)
	}
	// Leave a stale socket file behind to cover the removal branch.
	if err := os.WriteFile(socketPath, nil, 0o644); err != nil {
		t.Fatalf("plant stale socket: %v", err)
	}

	ln := initializeListener()
	t.Cleanup(func() {
		ln.Close()
		os.Remove(socketPath)
	})

	fi, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("socket perm = %o, want 600", perm)
	}
	c, err := net.DialTimeout("unix", socketPath, time.Second)
	if err != nil {
		t.Fatalf("dial socket: %v", err)
	}
	c.Close()
}

func TestInitializeSchedulerAndAPI(t *testing.T) {
	store := storage.New()
	machine := monitor.NewMachine()

	sched := initializeScheduler(store, machine, monitor.NewCertTracker(), monitor.NewFlapTracker(),
		notify.NewStdout(log.New(io.Discard)))
	if sched == nil {
		t.Error("initializeScheduler returned nil")
	}
	if initializeAPI(store, machine) == nil {
		t.Error("initializeAPI returned nil")
	}
}

// pruneStore counts Prune calls and can fail them, standing in for the store in
// the retention loop.
type pruneStore struct {
	storage.Store
	mu     sync.Mutex
	before []time.Time
	err    error
	called chan struct{}
}

func newPruneStore(err error) *pruneStore {
	return &pruneStore{Store: storage.New(), err: err, called: make(chan struct{}, 8)}
}

func (s *pruneStore) Prune(before time.Time) error {
	s.mu.Lock()
	s.before = append(s.before, before)
	s.mu.Unlock()
	select {
	case s.called <- struct{}{}:
	default:
	}
	return s.err
}

func (s *pruneStore) calls() []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]time.Time, len(s.before))
	copy(out, s.before)
	return out
}

func TestRetentionLoopPrunesOnceThenStopsWithContext(t *testing.T) {
	store := newPruneStore(nil)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() { retentionLoop(ctx, store); close(done) }()

	select {
	case <-store.called:
	case <-time.After(2 * time.Second):
		t.Fatal("retentionLoop did not prune at startup")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("retentionLoop did not return on context cancel")
	}

	calls := store.calls()
	if len(calls) != 1 {
		t.Fatalf("prune calls = %d, want 1 (sweep interval is %v)", len(calls), retentionSweep)
	}
	// The cutoff is retentionAge in the past, not "now".
	age := time.Since(calls[0])
	if age < retentionAge-time.Minute || age > retentionAge+time.Minute {
		t.Errorf("prune cutoff age = %v, want ~%v", age, retentionAge)
	}
}

func TestRetentionLoopSurvivesPruneError(t *testing.T) {
	store := newPruneStore(errors.New("prune boom"))
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() { retentionLoop(ctx, store); close(done) }()

	select {
	case <-store.called:
	case <-time.After(2 * time.Second):
		t.Fatal("retentionLoop did not prune at startup")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("retentionLoop did not return after a failing prune")
	}
}

// unixSocket returns a listener on a short temp path (unix socket paths are
// length-capped and t.TempDir names are long).
func unixSocket(t *testing.T) (net.Listener, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "termup")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "api.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return ln, sock
}

func socketClient(sock string) *http.Client {
	return &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", sock)
			},
		},
	}
}

// waitForAPI blocks until the API answers over the socket. For run/Start that
// also proves signal.NotifyContext is installed, so a SIGTERM sent afterwards is
// caught instead of killing the test process.
func waitForAPI(t *testing.T, sock string) {
	t.Helper()
	client := socketClient(sock)
	for i := 0; ; i++ {
		resp, err := client.Get("http://localhost/v1/monitors")
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			return
		}
		if i == 100 {
			t.Fatalf("api never came up: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func testScheduler(store storage.Store) *scheduler.Scheduler {
	return scheduler.New(probe.NewHTTP(probeTimeout), store, monitor.NewMachine(),
		monitor.NewCertTracker(), monitor.NewFlapTracker(), notify.NewStdout(log.New(io.Discard)),
		scheduler.RealClock(), probeInterval, probeTimeout, probeWorkers)
}

func writeConfigFile(t *testing.T, body string) {
	t.Helper()
	if err := os.WriteFile(configPath, []byte(body), 0o644); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
}

// waitForMonitors polls the store until it holds want monitors.
func waitForMonitors(t *testing.T, store storage.Reader, want int) {
	t.Helper()
	for i := 0; ; i++ {
		ms, err := store.List()
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(ms) == want {
			return
		}
		if i == 200 {
			t.Fatalf("store holds %d monitors, want %d", len(ms), want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func sigterm(t *testing.T, done <-chan struct{}) {
	t.Helper()
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("signal self: %v", err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("did not return after SIGTERM")
	}
}

// TestRunServesThenShutsDownOnSignal exercises the orchestration in run: the API
// serves on the given listener, config changes are hot-reloaded into the store,
// and SIGTERM triggers the graceful shutdown path.
func TestRunServesThenShutsDownOnSignal(t *testing.T) {
	// config.Watch needs the file to exist; the empty store keeps the scheduler
	// from probing anything.
	writeConfig(t, oneMonitor)

	store := storage.New()
	ln, sock := unixSocket(t)

	done := make(chan struct{})
	go func() { run(api.New(store, monitor.NewMachine()), testScheduler(store), store, ln); close(done) }()

	waitForAPI(t, sock)

	// Hot-reload: the new monitor lands in the store.
	writeConfigFile(t, oneMonitor+`  - name: b
    url: http://b.example
`)
	waitForMonitors(t, store, 2)

	// An invalid config is skipped and the previous list is kept.
	writeConfigFile(t, "monitors:\n")
	time.Sleep(200 * time.Millisecond)
	if ms, _ := store.List(); len(ms) != 2 {
		t.Errorf("after invalid config store holds %d monitors, want the previous 2", len(ms))
	}
	// The watcher survived the bad config: a later valid one still applies.
	writeConfigFile(t, oneMonitor)
	waitForMonitors(t, store, 1)

	sigterm(t, done)
}

// syncFailStore fails Sync, so the reload error branch in run is exercised.
type syncFailStore struct {
	storage.Store
	attempts chan struct{}
}

func (s *syncFailStore) Sync([]monitor.Monitor) error {
	select {
	case s.attempts <- struct{}{}:
	default:
	}
	return errors.New("sync boom")
}

func TestRunKeepsServingWhenReloadSyncFails(t *testing.T) {
	writeConfig(t, oneMonitor)

	store := &syncFailStore{Store: storage.New(), attempts: make(chan struct{}, 8)}
	ln, sock := unixSocket(t)

	done := make(chan struct{})
	go func() { run(api.New(store, monitor.NewMachine()), testScheduler(store), store, ln); close(done) }()

	waitForAPI(t, sock)

	writeConfigFile(t, oneMonitor+`  - name: b
    url: http://b.example
`)
	select {
	case <-store.attempts:
	case <-time.After(5 * time.Second):
		t.Fatal("config change did not reach store.Sync")
	}

	// The failed sync must not take the server down.
	waitForAPI(t, sock)
	sigterm(t, done)
}

// TestStart wires the real composition root. It binds the production socket
// path, so it is skipped when a daemon is already listening there.
func TestStart(t *testing.T) {
	if c, err := net.DialTimeout("unix", socketPath, 200*time.Millisecond); err == nil {
		c.Close()
		t.Skipf("a daemon is listening on %s", socketPath)
	}
	orig := log.Default().GetLevel()
	t.Cleanup(func() {
		log.Default().SetLevel(orig)
		os.Remove(socketPath)
	})

	// Probe target is a local test server: Start's scheduler must not reach out
	// to the internet.
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer target.Close()
	writeConfig(t, "monitors:\n  - name: local\n    url: "+target.URL+"\n")

	done := make(chan struct{})
	go func() { Start(); close(done) }()

	waitForAPI(t, socketPath)
	sigterm(t, done)
}
