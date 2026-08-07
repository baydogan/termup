package api

import (
	"context"

	"github.com/baydogan/termup/monitor"
)

type Reader interface {
	List() ([]monitor.Monitor, error)
	GetStatus(name string) (monitor.Status, error)
	History(name string) ([]monitor.Result, error)
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

type StatusHandler struct{ store Reader }

func (h *StatusHandler) Handle(_ context.Context, req *StatusRequest) (*StatusResponse, error) {
	st, err := h.store.GetStatus(req.Name)
	if err != nil {
		return nil, err
	}
	return &StatusResponse{Name: req.Name, State: st.State.String()}, nil
}

type DashboardHandler struct{ store Reader }

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
		out = append(out, buildHealth(m, hist))
	}
	return &DashboardResponse{Monitors: out}, nil
}

// buildHealth maps a monitor + its result history into the dashboard DTO.
func buildHealth(m monitor.Monitor, hist []monitor.Result) MonitorHealthDTO {
	dto := MonitorHealthDTO{Name: m.Name, URL: m.URL, State: monitor.StateUnknown.String()}
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
		recent = append(recent, CheckDTO{
			Up:        isUp,
			LatencyMs: r.Latency.Milliseconds(),
			At:        r.CheckedAt.Unix(),
		})
	}

	last := hist[len(hist)-1]
	dto.State = last.State().String()
	dto.LatencyMs = last.Latency.Milliseconds()
	dto.UptimePct = float64(up) / float64(len(hist)) * 100
	dto.Recent = recent
	return dto
}
