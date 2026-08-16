package api

import (
	"errors"
	"testing"
	"time"

	"github.com/baydogan/termup/monitor"
)

func TestBuildHealth(t *testing.T) {
	now := time.Unix(1000, 0)
	m := monitor.Monitor{Name: "x", URL: "http://x"}
	hist := []monitor.Result{
		{MonitorName: "x", StatusCode: 500, CheckedAt: now, Latency: 10 * time.Millisecond, Err: errors.New("boom")},
		{
			MonitorName: "x", StatusCode: 200, CheckedAt: now.Add(time.Second), Latency: 20 * time.Millisecond,
			DNS: 1 * time.Millisecond, Connect: 2 * time.Millisecond, TLS: 3 * time.Millisecond, TTFB: 4 * time.Millisecond,
			CertExpiry: now.Add(48 * time.Hour),
		},
	}

	// State comes from the machine, not the raw last result.
	dto := buildHealth(m, hist, monitor.StateUp)

	if dto.State != "up" {
		t.Errorf("State = %q, want up", dto.State)
	}
	if dto.UptimePct != 50 {
		t.Errorf("UptimePct = %v, want 50", dto.UptimePct)
	}
	if dto.LatencyMs != 20 { // latest result
		t.Errorf("LatencyMs = %d, want 20", dto.LatencyMs)
	}
	if dto.CertExpiry != hist[1].CertExpiry.Unix() {
		t.Errorf("CertExpiry = %d, want %d", dto.CertExpiry, hist[1].CertExpiry.Unix())
	}
	if len(dto.Recent) != 2 {
		t.Fatalf("Recent len = %d, want 2", len(dto.Recent))
	}

	// Oldest check: down with the error text and status.
	if r0 := dto.Recent[0]; r0.Up || r0.StatusCode != 500 || r0.Error == "" {
		t.Errorf("Recent[0] = %+v, want down/500/error", r0)
	}
	// Newest check: up with the stage breakdown mapped through.
	r1 := dto.Recent[1]
	if !r1.Up || r1.StatusCode != 200 {
		t.Errorf("Recent[1] up/status = %v/%d", r1.Up, r1.StatusCode)
	}
	if r1.DNSMs != 1 || r1.ConnectMs != 2 || r1.TLSMs != 3 || r1.TTFBMs != 4 {
		t.Errorf("Recent[1] stages = {%d %d %d %d}, want {1 2 3 4}", r1.DNSMs, r1.ConnectMs, r1.TLSMs, r1.TTFBMs)
	}
}

func TestBuildHealthEmptyHistory(t *testing.T) {
	dto := buildHealth(monitor.Monitor{Name: "x", URL: "u"}, nil, monitor.StateUnknown)
	if dto.State != "unknown" {
		t.Errorf("State = %q, want unknown", dto.State)
	}
	if dto.UptimePct != 0 || len(dto.Recent) != 0 || dto.CertExpiry != 0 {
		t.Errorf("empty history should give zero-valued health: %+v", dto)
	}
	// Empty, not nil: the wire shape must stay an array (see TestDashboardRecentIsNeverNull).
	if dto.Recent == nil {
		t.Error("Recent is nil; it must be an empty slice so it serialises as []")
	}
}
