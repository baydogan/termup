package storage_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/baydogan/termup/monitor"
	"github.com/baydogan/termup/storage"
)

func TestSQLiteCertExpiryRoundTrip(t *testing.T) {
	s, err := storage.NewSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	exp := time.Unix(1791828375, 0)
	if err := s.Save(monitor.Result{
		MonitorName: "x",
		StatusCode:  200,
		CheckedAt:   time.Unix(1000, 0),
		CertExpiry:  exp,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	hist, err := s.History("x")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("history len = %d, want 1", len(hist))
	}
	if !hist[0].CertExpiry.Equal(exp) {
		t.Errorf("CertExpiry = %v, want %v", hist[0].CertExpiry, exp)
	}
}

func TestSQLiteStageTimingsRoundTrip(t *testing.T) {
	s, err := storage.NewSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	in := monitor.Result{
		MonitorName: "x",
		StatusCode:  200,
		CheckedAt:   time.Unix(1000, 0),
		Latency:     120 * time.Millisecond,
		DNS:         2 * time.Millisecond,
		Connect:     5 * time.Millisecond,
		TLS:         30 * time.Millisecond,
		TTFB:        40 * time.Millisecond,
	}
	if err := s.Save(in); err != nil {
		t.Fatalf("save: %v", err)
	}

	hist, err := s.History("x")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("history len = %d, want 1", len(hist))
	}
	got := hist[0]
	if got.DNS != in.DNS || got.Connect != in.Connect || got.TLS != in.TLS || got.TTFB != in.TTFB {
		t.Errorf("stages = {dns %v conn %v tls %v ttfb %v}, want {%v %v %v %v}",
			got.DNS, got.Connect, got.TLS, got.TTFB, in.DNS, in.Connect, in.TLS, in.TTFB)
	}
}

func TestSQLiteNoCertStaysZero(t *testing.T) {
	s, err := storage.NewSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	if err := s.Save(monitor.Result{
		MonitorName: "x",
		StatusCode:  200,
		CheckedAt:   time.Unix(1000, 0),
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	hist, err := s.History("x")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("history len = %d, want 1", len(hist))
	}
	if !hist[0].CertExpiry.IsZero() {
		t.Errorf("CertExpiry = %v, want zero", hist[0].CertExpiry)
	}
}

func TestSQLiteHistoryOrderAndLimit(t *testing.T) {
	s, err := storage.NewSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	// Save more than the store's history limit (60); encode the index in the
	// status code so we can identify which rows come back.
	const total = 70
	for i := 0; i < total; i++ {
		if err := s.Save(monitor.Result{MonitorName: "x", StatusCode: i, CheckedAt: time.Unix(int64(i), 0)}); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	hist, err := s.History("x")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	// Capped at 60, newest kept, returned oldest -> newest.
	if len(hist) != 60 {
		t.Fatalf("history len = %d, want 60 (limit)", len(hist))
	}
	if hist[0].StatusCode != total-60 {
		t.Errorf("first = %d, want %d (oldest kept)", hist[0].StatusCode, total-60)
	}
	if hist[len(hist)-1].StatusCode != total-1 {
		t.Errorf("last = %d, want %d (newest)", hist[len(hist)-1].StatusCode, total-1)
	}
	for i := 1; i < len(hist); i++ {
		if hist[i].StatusCode <= hist[i-1].StatusCode {
			t.Fatalf("not chronological at %d: %d after %d", i, hist[i].StatusCode, hist[i-1].StatusCode)
		}
	}
}

func TestSQLiteSyncPrunesDroppedMonitors(t *testing.T) {
	s, err := storage.NewSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	if err := s.Sync([]monitor.Monitor{{Name: "a", URL: "http://a"}, {Name: "b", URL: "http://b"}}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	for _, name := range []string{"a", "b"} {
		if err := s.Save(monitor.Result{MonitorName: name, StatusCode: 200, CheckedAt: time.Unix(1, 0)}); err != nil {
			t.Fatalf("save %s: %v", name, err)
		}
	}

	// Reconfigure: drop b.
	if err := s.Sync([]monitor.Monitor{{Name: "a", URL: "http://a"}}); err != nil {
		t.Fatalf("resync: %v", err)
	}

	ms, err := s.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ms) != 1 || ms[0].Name != "a" {
		t.Errorf("list = %v, want [a]", ms)
	}

	// a's history survives, b's is pruned.
	if ha, _ := s.History("a"); len(ha) != 1 {
		t.Errorf("a history = %d, want 1 (kept)", len(ha))
	}
	if hb, _ := s.History("b"); len(hb) != 0 {
		t.Errorf("b history = %d, want 0 (pruned)", len(hb))
	}
}
