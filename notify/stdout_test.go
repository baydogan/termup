package notify_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/baydogan/termup/monitor"
	"github.com/baydogan/termup/notify"
	charmlog "github.com/charmbracelet/log"
)

// testStdout builds a Stdout notifier logging to buf with timestamps off, so
// assertions are deterministic.
func testStdout(buf *bytes.Buffer) *notify.Stdout {
	l := charmlog.New(buf)
	l.SetReportTimestamp(false)
	return notify.NewStdout(l)
}

func contains(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("output %q missing %q", out, w)
		}
	}
}

func TestStdoutStateChangeDownIsWarn(t *testing.T) {
	var buf bytes.Buffer
	if err := testStdout(&buf).Notify(notify.Event{
		Kind: notify.KindStateChange, Monitor: "local",
		From: monitor.StateUp, To: monitor.StateDown, Result: monitor.Result{StatusCode: 500},
	}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	contains(t, buf.String(), "WARN", "state changed", "monitor=local", "from=up", "to=down", "code=500")
}

func TestStdoutStateChangeRecoveryIsInfo(t *testing.T) {
	var buf bytes.Buffer
	testStdout(&buf).Notify(notify.Event{
		Kind: notify.KindStateChange, Monitor: "x", From: monitor.StateDown, To: monitor.StateUp,
	})
	contains(t, buf.String(), "INFO", "to=up")
}

func TestStdoutCertExpiring(t *testing.T) {
	var buf bytes.Buffer
	testStdout(&buf).Notify(notify.Event{
		Kind: notify.KindCertExpiring, Monitor: "x", DaysLeft: 7,
		CertExpiry: time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC),
	})
	contains(t, buf.String(), "WARN", "cert expiring", "days=7", "expiry=2026-10-04")
}

func TestStdoutFlapping(t *testing.T) {
	var buf bytes.Buffer
	testStdout(&buf).Notify(notify.Event{Kind: notify.KindFlapping, Monitor: "x", Flips: 6})
	contains(t, buf.String(), "WARN", "flapping", "flips=6")
}
