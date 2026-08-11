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
