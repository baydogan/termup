package monitor

import "sync"

// CertWarnDays is the threshold (in days) below which a certificate triggers a
// warning. Fixed, not configurable (decision: 2026-08-11). Independent of the
// state machine's FailThreshold (3 = consecutive fails; 14 = days of cert).
const CertWarnDays = 14

// CertTracker records, per monitor, whether we have already warned about a
// soon-to-expire certificate, so the alert fires once per crossing and re-arms
// when the cert is renewed back above the threshold. Pure: the caller computes
// days-left (the clock lives in the scheduler). Mutex-guarded for the probe pool.
type CertTracker struct {
	mu     sync.Mutex
	warned map[string]bool
}

func NewCertTracker() *CertTracker {
	return &CertTracker{warned: make(map[string]bool)}
}

// Observe reports whether a CertExpiring alert should fire now. hasCert=false
// (plain HTTP or a failed handshake) carries no expiry info, so the armed state
// is left untouched. Below the threshold it fires once; back above it re-arms.
func (c *CertTracker) Observe(name string, daysLeft int, hasCert bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !hasCert {
		return false
	}
	below := daysLeft < CertWarnDays
	switch {
	case below && !c.warned[name]:
		c.warned[name] = true
		return true
	case !below:
		c.warned[name] = false // re-arm after renewal
	}
	return false
}
