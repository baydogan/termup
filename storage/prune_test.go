package storage_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/baydogan/termup/monitor"
	"github.com/baydogan/termup/storage"
)

// pruneStore lets the test run against both store implementations.
func TestPruneDropsOldKeepsRecent(t *testing.T) {
	old := time.Unix(1000, 0)
	recent := time.Unix(5000, 0)
	cutoff := time.Unix(3000, 0)

	stores := map[string]storage.Store{
		"memory": storage.New(),
		"sqlite": mustSQLite(t),
	}

	for name, s := range stores {
		t.Run(name, func(t *testing.T) {
			for _, at := range []time.Time{old, recent} {
				if err := s.Save(monitor.Result{MonitorName: "x", StatusCode: 200, CheckedAt: at}); err != nil {
					t.Fatalf("save: %v", err)
				}
			}

			if err := s.Prune(cutoff); err != nil {
				t.Fatalf("prune: %v", err)
			}

			hist, err := s.History("x")
			if err != nil {
				t.Fatalf("history: %v", err)
			}
			if len(hist) != 1 {
				t.Fatalf("history len = %d, want 1 (old pruned)", len(hist))
			}
			if !hist[0].CheckedAt.Equal(recent) {
				t.Errorf("kept %v, want the recent row %v", hist[0].CheckedAt, recent)
			}
		})
	}
}

func mustSQLite(t *testing.T) *storage.SQLite {
	t.Helper()
	s, err := storage.NewSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
