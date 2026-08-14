package storage_test

import (
	"testing"

	"github.com/baydogan/termup/monitor"
	"github.com/baydogan/termup/storage"
)

func TestSQLiteCloseReleasesTheDatabase(t *testing.T) {
	s := mustSQLite(t)

	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Every port method must fail rather than silently pretend to work.
	if _, err := s.List(); err == nil {
		t.Error("List after Close succeeded, want error")
	}
	if err := s.Save(monitor.Result{MonitorName: "a", StatusCode: 200}); err == nil {
		t.Error("Save after Close succeeded, want error")
	}
	if err := s.Sync([]monitor.Monitor{{Name: "a", URL: "http://a"}}); err == nil {
		t.Error("Sync after Close succeeded, want error")
	}
	// Close is called from the shutdown path; a second one must not blow up.
	if err := s.Close(); err != nil {
		t.Errorf("second close: %v", err)
	}
}

func TestMemoryCloseIsANoOp(t *testing.T) {
	s := storage.New(monitor.Monitor{Name: "a", URL: "http://a"})

	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Nothing external to release, so the adapter stays usable.
	if _, err := s.List(); err != nil {
		t.Errorf("List after Close: %v", err)
	}
}
