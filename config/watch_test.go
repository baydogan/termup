package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// monitorsYAML builds a valid config body with the given monitor names.
func monitorsYAML(names ...string) string {
	var b strings.Builder
	b.WriteString("monitors:\n")
	for _, n := range names {
		fmt.Fprintf(&b, "  - name: %s\n    url: https://%s.test\n", n, n)
	}
	return b.String()
}

func rewrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// awaitReload waits for a reload carrying want monitors, draining earlier ones.
// A single save can emit several fsnotify WRITE events, so the same config may
// already be queued more than once; that is inherent to watching a file, not a
// failure, and the test must not assume one save means exactly one reload.
func awaitReload(t *testing.T, reloads <-chan *Config, want int) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case c := <-reloads:
			if len(c.Monitors) == want {
				return
			}
			t.Logf("draining duplicate reload with %d monitors", len(c.Monitors))
		case <-deadline:
			t.Fatalf("no reload with %d monitors", want)
		}
	}
}

func TestWatchReloadsAndSurvivesBadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	rewrite(t, path, monitorsYAML("a"))

	reloads := make(chan *Config, 8)
	errs := make(chan error, 8)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Watch(ctx, path, func(c *Config) { reloads <- c }, func(e error) { errs <- e })
		close(done)
	}()
	// Give the watcher a moment to register with fsnotify.
	time.Sleep(150 * time.Millisecond)

	// Valid change -> onReload with the new list.
	rewrite(t, path, monitorsYAML("a", "b"))
	awaitReload(t, reloads, 2)

	// Invalid change -> onError (and the watcher must keep running).
	rewrite(t, path, "monitors: [broken")
	select {
	case <-errs:
	case <-time.After(3 * time.Second):
		t.Fatal("no error after an invalid change")
	}

	// A valid change after the bad one still reloads (watcher survived).
	rewrite(t, path, monitorsYAML("a", "b", "c"))
	awaitReload(t, reloads, 3)

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return after context cancel")
	}
}
