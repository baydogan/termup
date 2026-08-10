package stdout

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/baydogan/termup/monitor"
	"github.com/baydogan/termup/notify"
)

var _ notify.Notifier = (*Notifier)(nil)

// Notifier prints state transitions to a writer (stdout by default). It is the
// v0 alerting adapter.
type Notifier struct {
	w io.Writer
}

func New() *Notifier { return &Notifier{w: os.Stdout} }

// NewWriter builds a Notifier over an arbitrary writer (used in tests).
func NewWriter(w io.Writer) *Notifier { return &Notifier{w: w} }

func (n *Notifier) Notify(t monitor.Transition) error {
	_, err := fmt.Fprintf(n.w, "[ALERT] %s %s -> %s (code=%d, %s)\n",
		t.Monitor, t.From, t.To, t.Result.StatusCode, t.Result.Latency.Round(time.Millisecond))
	return err
}
