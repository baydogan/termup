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
	select {
	case c := <-reloads:
		if len(c.Monitors) != 2 {
			t.Errorf("reload monitors = %d, want 2", len(c.Monitors))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no reload after a valid change")
	}

	// Invalid change -> onError (and the watcher must keep running).
	rewrite(t, path, "monitors: [broken")
	select {
	case <-errs:
	case <-time.After(3 * time.Second):
		t.Fatal("no error after an invalid change")
	}

	// A valid change after the bad one still reloads (watcher survived).
	rewrite(t, path, monitorsYAML("a", "b", "c"))
	select {
	case c := <-reloads:
		if len(c.Monitors) != 3 {
			t.Errorf("reload monitors = %d, want 3", len(c.Monitors))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watcher did not survive the bad config")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return after context cancel")
	}
}
