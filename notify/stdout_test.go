package notify_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/baydogan/termup/monitor"
	"github.com/baydogan/termup/notify"
)

func TestStdoutNotifyFormat(t *testing.T) {
	var buf bytes.Buffer
	n := notify.NewStdoutWriter(&buf)

	tr := monitor.Transition{
		Monitor: "local",
		From:    monitor.StateUp,
		To:      monitor.StateDown,
		Result:  monitor.Result{StatusCode: 500, Latency: 12 * time.Millisecond},
	}
	if err := n.Notify(tr); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	want := "[ALERT] local up -> down (code=500, 12ms)\n"
	if got := buf.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
