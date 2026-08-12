package probe

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptrace"
	"time"

	"github.com/baydogan/termup/monitor"
)

type Prober interface {
	Probe(ctx context.Context, m *monitor.Monitor) monitor.Result
}

type HTTP struct {
	client *http.Client
}

func NewHTTP(timeout time.Duration) *HTTP {
	// Clone the default transport (keeps proxy/dial defaults) but disable
	// keep-alives so every probe does a fresh DNS/connect/TLS — otherwise the
	// stage timings would be zero on reused connections.
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DisableKeepAlives = true

	return &HTTP{
		client: &http.Client{
			Timeout:   timeout,
			Transport: tr,
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

	var st stageTimes
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), st.trace()))

	resp, err := h.client.Do(req)
	if err != nil {
		return monitor.Result{MonitorName: m.Name, Latency: time.Since(start), Err: err}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	res := monitor.Result{
		MonitorName: m.Name,
		Latency:     time.Since(start),
		StatusCode:  resp.StatusCode,
		DNS:         st.dns,
		Connect:     st.connect,
		TLS:         st.tls,
		TTFB:        st.ttfb,
	}
	// For HTTPS targets, surface the leaf certificate's expiry (NotAfter).
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		res.CertExpiry = resp.TLS.PeerCertificates[0].NotAfter
	}
	return res
}

// stageTimes captures per-stage durations via httptrace hooks. The hooks run
// synchronously within client.Do, so the fields are set by the time Do returns.
type stageTimes struct {
	dnsStart, connStart, tlsStart, wroteReq time.Time
	dns, connect, tls, ttfb                 time.Duration
}

func (s *stageTimes) trace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { s.dnsStart = time.Now() },
		DNSDone:  func(httptrace.DNSDoneInfo) { s.dns = time.Since(s.dnsStart) },
		ConnectStart: func(_, _ string) {
			if s.connStart.IsZero() {
				s.connStart = time.Now()
			}
		},
		ConnectDone:       func(_, _ string, _ error) { s.connect = time.Since(s.connStart) },
		TLSHandshakeStart: func() { s.tlsStart = time.Now() },
		TLSHandshakeDone:  func(tls.ConnectionState, error) { s.tls = time.Since(s.tlsStart) },
		WroteRequest:      func(httptrace.WroteRequestInfo) { s.wroteReq = time.Now() },
		GotFirstResponseByte: func() {
			if !s.wroteReq.IsZero() {
				s.ttfb = time.Since(s.wroteReq)
			}
		},
	}
}
