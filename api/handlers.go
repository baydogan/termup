package api

import (
	"context"

	"github.com/baydogan/zerotolerance/monitor"
)

type Reader interface {
	List() []monitor.Monitor
	GetStatus(name string) (monitor.Status, error)
}

type ListHandler struct{ store Reader }

func (h *ListHandler) Handle(_ context.Context, _ *ListRequest) (*ListResponse, error) {
	ms := h.store.List()
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
