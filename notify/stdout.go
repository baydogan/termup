package notify

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/baydogan/termup/monitor"
)

var _ Notifier = (*Stdout)(nil)

// Stdout prints state transitions to a writer (stdout by default). It is the
// v0 alerting adapter.
type Stdout struct {
	w io.Writer
}

func NewStdout() *Stdout { return &Stdout{w: os.Stdout} }

// NewStdoutWriter builds a Stdout notifier over an arbitrary writer (used in tests).
func NewStdoutWriter(w io.Writer) *Stdout { return &Stdout{w: w} }

func (n *Stdout) Notify(t monitor.Transition) error {
	_, err := fmt.Fprintf(n.w, "[ALERT] %s %s -> %s (code=%d, %s)\n",
		t.Monitor, t.From, t.To, t.Result.StatusCode, t.Result.Latency.Round(time.Millisecond))
	return err
}
