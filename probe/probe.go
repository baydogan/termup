package probe

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/baydogan/zerotolerance/monitor"
)

type Prober interface {
	Probe(ctx context.Context, m *monitor.Monitor) monitor.Result
}

type HTTP struct {
	client *http.Client
}

func NewHTTP(timeout time.Duration) *HTTP {
	return &HTTP{
		client: &http.Client{
			Timeout: timeout,
			// Do NOT follow redirects: 3xx must surface as-is so it maps to
			// down (only 2xx is up). Default client would follow 301→200 and
			// wrongly report up.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (h *HTTP) Probe(ctx context.Context, m *monitor.Monitor) monitor.Result {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.URL, nil)
	if err != nil {
		return monitor.Result{MonitorName: m.Name, Latency: time.Since(start), Err: err}
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return monitor.Result{MonitorName: m.Name, Latency: time.Since(start), Err: err}
	}
	defer resp.Body.Close()
	io.Copy(ioutil.Discard, resp.Body)

	return monitor.Result{MonitorName: m.Name, Latency: time.Since(start), StatusCode: resp.StatusCode}
}
