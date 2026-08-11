package notify_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/baydogan/termup/monitor"
	"github.com/baydogan/termup/notify"
)

func TestStdoutStateChangeFormat(t *testing.T) {
	var buf bytes.Buffer
	n := notify.NewStdoutWriter(&buf)

	e := notify.Event{
		Kind:    notify.KindStateChange,
		Monitor: "local",
		From:    monitor.StateUp,
		To:      monitor.StateDown,
		Result:  monitor.Result{StatusCode: 500, Latency: 12 * time.Millisecond},
	}
	if err := n.Notify(e); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	want := "[ALERT] local up -> down (code=500, 12ms)\n"
	if got := buf.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestStdoutFlappingFormat(t *testing.T) {
	var buf bytes.Buffer
	n := notify.NewStdoutWriter(&buf)

	if err := n.Notify(notify.Event{Kind: notify.KindFlapping, Monitor: "local", Flips: 6}); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	want := fmt.Sprintf("[FLAP] local flapping (6 flips in last %d)\n", monitor.FlapWindow)
	if got := buf.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestStdoutCertExpiringFormat(t *testing.T) {
	var buf bytes.Buffer
	n := notify.NewStdoutWriter(&buf)

	e := notify.Event{
		Kind:       notify.KindCertExpiring,
		Monitor:    "local",
		CertExpiry: time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC),
		DaysLeft:   7,
	}
	if err := n.Notify(e); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	want := "[CERT] local certificate expires in 7d (2026-10-04)\n"
	if got := buf.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
