package notify

import (
	"github.com/baydogan/termup/monitor"
	"github.com/charmbracelet/log"
)

var _ Notifier = (*Stdout)(nil)

// Stdout reports events through a charmbracelet/log logger. It is the v0
// alerting adapter; recoveries log at info, everything else at warn.
type Stdout struct {
	log *log.Logger
}

func NewStdout(l *log.Logger) *Stdout { return &Stdout{log: l} }

func (n *Stdout) Notify(e Event) error {
	switch e.Kind {
	case KindStateChange:
		fields := []any{"monitor", e.Monitor, "from", e.From, "to", e.To, "code", e.Result.StatusCode}
		if e.To == monitor.StateDown {
			n.log.Warn("state changed", fields...)
		} else {
			n.log.Info("state changed", fields...)
		}
	case KindCertExpiring:
		n.log.Warn("cert expiring",
			"monitor", e.Monitor, "days", e.DaysLeft, "expiry", e.CertExpiry.Format("2006-01-02"))
	case KindFlapping:
		n.log.Warn("flapping", "monitor", e.Monitor, "flips", e.Flips, "window", monitor.FlapWindow)
	}
	return nil
}
