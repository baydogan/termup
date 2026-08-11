package notify

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/baydogan/termup/monitor"
)

var _ Notifier = (*Stdout)(nil)

// Stdout prints events to a writer (stdout by default). It is the v0 alerting
// adapter.
type Stdout struct {
	w io.Writer
}

func NewStdout() *Stdout { return &Stdout{w: os.Stdout} }

// NewStdoutWriter builds a Stdout notifier over an arbitrary writer (used in tests).
func NewStdoutWriter(w io.Writer) *Stdout { return &Stdout{w: w} }

func (n *Stdout) Notify(e Event) error {
	var err error
	switch e.Kind {
	case KindStateChange:
		_, err = fmt.Fprintf(n.w, "[ALERT] %s %s -> %s (code=%d, %s)\n",
			e.Monitor, e.From, e.To, e.Result.StatusCode, e.Result.Latency.Round(time.Millisecond))
	case KindCertExpiring:
		_, err = fmt.Fprintf(n.w, "[CERT] %s certificate expires in %dd (%s)\n",
			e.Monitor, e.DaysLeft, e.CertExpiry.Format("2006-01-02"))
	case KindFlapping:
		_, err = fmt.Fprintf(n.w, "[FLAP] %s flapping (%d flips in last %d)\n",
			e.Monitor, e.Flips, monitor.FlapWindow)
	}
	return err
}
