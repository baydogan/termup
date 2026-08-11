package api

import (
	"context"

	"github.com/baydogan/termup/monitor"
	"github.com/baydogan/termup/storage"
)

type Reader interface {
	List() ([]monitor.Monitor, error)
	History(name string) ([]monitor.Result, error)
}

// StateReader serves the debounced State from the state machine, replacing the
// raw last-result derivation the store used to expose.
type StateReader interface {
	State(name string) monitor.State
}

type ListHandler struct{ store Reader }

func (h *ListHandler) Handle(_ context.Context, _ *ListRequest) (*ListResponse, error) {
	ms, err := h.store.List()
	if err != nil {
		return nil, err
	}
	out := make([]MonitorDTO, 0, len(ms))
	for _, m := range ms {
		out = append(out, MonitorDTO{Name: m.Name, URL: m.URL})
	}
	return &ListResponse{Monitors: out}, nil
}

type StatusHandler struct {
	store Reader
	state StateReader
}

func (h *StatusHandler) Handle(_ context.Context, req *StatusRequest) (*StatusResponse, error) {
	ms, err := h.store.List()
	if err != nil {
		return nil, err
	}
	if !containsMonitor(ms, req.Name) {
		return nil, storage.ErrNotFound
	}
	return &StatusResponse{Name: req.Name, State: h.state.State(req.Name).String()}, nil
}

type DashboardHandler struct {
	store Reader
	state StateReader
}

func (h *DashboardHandler) Handle(_ context.Context, _ *DashboardRequest) (*DashboardResponse, error) {
	ms, err := h.store.List()
	if err != nil {
		return nil, err
	}
	out := make([]MonitorHealthDTO, 0, len(ms))
	for _, m := range ms {
		hist, err := h.store.History(m.Name)
		if err != nil {
			return nil, err
		}
		out = append(out, buildHealth(m, hist, h.state.State(m.Name)))
	}
	return &DashboardResponse{Monitors: out}, nil
}

func containsMonitor(ms []monitor.Monitor, name string) bool {
	for _, m := range ms {
		if m.Name == name {
			return true
		}
	}
	return false
}

// buildHealth maps a monitor + its result history into the dashboard DTO. The
// card State is the debounced machine state; the bars/uptime stay per-probe raw.
func buildHealth(m monitor.Monitor, hist []monitor.Result, state monitor.State) MonitorHealthDTO {
	dto := MonitorHealthDTO{Name: m.Name, URL: m.URL, State: state.String()}
	if len(hist) == 0 {
		return dto
	}

	recent := make([]CheckDTO, 0, len(hist))
	up := 0
	for _, r := range hist {
		isUp := r.State() == monitor.StateUp
		if isUp {
			up++
		}
		var errStr string
		if r.Err != nil {
			errStr = r.Err.Error()
		}
		recent = append(recent, CheckDTO{
			Up:         isUp,
			LatencyMs:  r.Latency.Milliseconds(),
			At:         r.CheckedAt.Unix(),
			StatusCode: r.StatusCode,
			Error:      errStr,
		})
	}

	last := hist[len(hist)-1]
	dto.LatencyMs = last.Latency.Milliseconds()
	dto.UptimePct = float64(up) / float64(len(hist)) * 100
	dto.Recent = recent
	if !last.CertExpiry.IsZero() {
		dto.CertExpiry = last.CertExpiry.Unix()
	}
	return dto
}
