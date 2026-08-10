package notify

import "github.com/baydogan/termup/monitor"

// Notifier is the outbound port for alerting on a monitor state transition.
// v0 has a single stdout adapter; webhook/Slack/Discord come in Phase 2.
type Notifier interface {
	Notify(monitor.Transition) error
}
