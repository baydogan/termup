package probe

import (
	"context"
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
}
