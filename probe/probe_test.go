package probe

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/baydogan/termup/monitor"
)

func TestProbeCapturesCertExpiry(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	h := NewHTTP(5 * time.Second)
	h.client = ts.Client() // trust the server's self-signed cert

	res := h.Probe(context.Background(), &monitor.Monitor{Name: "t", URL: ts.URL})
	if res.Err != nil {
		t.Fatalf("probe: %v", res.Err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	want := ts.Certificate().NotAfter
	if !res.CertExpiry.Equal(want) {
		t.Errorf("CertExpiry = %v, want %v", res.CertExpiry, want)
	}

	// httptrace stages: an HTTPS round-trip must record connect + TLS.
	if res.Connect <= 0 {
		t.Errorf("Connect stage not captured: %v", res.Connect)
	}
	if res.TLS <= 0 {
		t.Errorf("TLS stage not captured: %v", res.TLS)
	}
	if res.TTFB <= 0 {
		t.Errorf("TTFB not captured: %v", res.TTFB)
	}
}

func TestProbeNoCertForPlainHTTP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	res := NewHTTP(5*time.Second).Probe(context.Background(), &monitor.Monitor{Name: "t", URL: ts.URL})
	if res.Err != nil {
		t.Fatalf("probe: %v", res.Err)
	}
	if !res.CertExpiry.IsZero() {
		t.Errorf("CertExpiry = %v, want zero for HTTP", res.CertExpiry)
	}
	// Plain HTTP: no TLS stage; connect still happens.
	if res.TLS != 0 {
		t.Errorf("TLS stage = %v, want 0 for HTTP", res.TLS)
	}
	if res.Connect <= 0 {
		t.Errorf("Connect stage not captured: %v", res.Connect)
	}
}

func TestProbeTTFBReflectsServerDelay(t *testing.T) {
	const delay = 60 * time.Millisecond
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(delay)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	res := NewHTTP(5*time.Second).Probe(context.Background(), &monitor.Monitor{Name: "t", URL: ts.URL})
	if res.Err != nil {
		t.Fatalf("probe: %v", res.Err)
	}
	// TTFB is the server wait; it must reflect the delay (allow scheduling slack).
	if res.TTFB < delay/2 {
		t.Errorf("TTFB = %v, want >= ~%v", res.TTFB, delay)
	}
	if res.Latency < res.TTFB {
		t.Errorf("Latency %v < TTFB %v", res.Latency, res.TTFB)
	}
}

func TestProbeErrorHasNoStages(t *testing.T) {
	// Nothing listening on port 1 -> connection refused before any response.
	res := NewHTTP(2*time.Second).Probe(context.Background(), &monitor.Monitor{Name: "t", URL: "http://127.0.0.1:1"})
	if res.Err == nil {
		t.Fatal("expected an error")
	}
	if res.DNS != 0 || res.Connect != 0 || res.TLS != 0 || res.TTFB != 0 {
		t.Errorf("stages not zero on error: dns=%v conn=%v tls=%v ttfb=%v",
			res.DNS, res.Connect, res.TLS, res.TTFB)
	}
}

func TestProbeRedirectCapturesStagesAndStatus(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.com", http.StatusMovedPermanently)
	}))
	defer ts.Close()

	// Keep the prober's own client (its no-follow CheckRedirect is what we test);
	// just make its transport trust the self-signed test cert.
	h := NewHTTP(5 * time.Second)
	h.client.Transport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	res := h.Probe(context.Background(), &monitor.Monitor{Name: "t", URL: ts.URL})
	if res.Err != nil {
		t.Fatalf("probe: %v", res.Err)
	}
	// 3xx is not followed and the TLS stage is still recorded.
	if res.StatusCode != http.StatusMovedPermanently {
		t.Errorf("status = %d, want 301 (redirect not followed)", res.StatusCode)
	}
	if res.TLS <= 0 {
		t.Errorf("TLS stage not captured on redirect: %v", res.TLS)
	}
}
