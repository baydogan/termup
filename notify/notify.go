package notify

import (
	"time"

	"github.com/baydogan/termup/monitor"
)

// Kind discriminates the union carried by Event.
type Kind int

const (
	KindStateChange  Kind = iota // up/down transition
	KindCertExpiring             // TLS certificate nearing expiry
	KindFlapping                 // target oscillating up/down
)

// Event is what the notifier reports on. Fields are populated per Kind:
// StateChange uses From/To/Result; CertExpiring uses CertExpiry/DaysLeft;
// Flapping uses Flips.
type Event struct {
	Kind    Kind
	Monitor string

	// StateChange
	From   monitor.State
	To     monitor.State
	Result monitor.Result

	// CertExpiring
	CertExpiry time.Time
	DaysLeft   int

	// Flapping
	Flips int
}

// Notifier is the outbound port for alerting. v0 has a single stdout adapter;
// webhook/Slack/Discord come later in Phase 2.
type Notifier interface {
	Notify(Event) error
}
