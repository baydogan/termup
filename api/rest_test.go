package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/baydogan/termup/monitor"
	"github.com/baydogan/termup/storage"
	"github.com/gofiber/fiber/v2"
)

// fakeReader is an in-test Reader whose List/History can be made to fail, so the
// error mapping in the handlers is exercised without a real store.
type fakeReader struct {
	monitors []monitor.Monitor
	hist     map[string][]monitor.Result
	listErr  error
	histErr  error
}

func (f fakeReader) List() ([]monitor.Monitor, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.monitors, nil
}

func (f fakeReader) History(name string) ([]monitor.Result, error) {
	if f.histErr != nil {
		return nil, f.histErr
	}
	return f.hist[name], nil
}

// fakeState stands in for the state machine; unknown names read as unknown.
type fakeState map[string]monitor.State

func (f fakeState) State(name string) monitor.State { return f[name] }

func do(t *testing.T, a *API, target string) *http.Response {
	t.Helper()
	resp, err := a.router.Test(httptest.NewRequest(http.MethodGet, target, nil))
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	return resp
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return out
}

func TestStatusRoute(t *testing.T) {
	a := New(
		fakeReader{monitors: []monitor.Monitor{{Name: "a", URL: "http://a"}}},
		fakeState{"a": monitor.StateDown},
	)

	resp := do(t, a, "/v1/monitors/a/status")
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := decode[StatusResponse](t, resp)
	if body.Name != "a" || body.State != "down" {
		t.Errorf("body = %+v, want {a down}", body)
	}
}

func TestStatusRouteUnknownMonitorIs404(t *testing.T) {
	a := New(fakeReader{monitors: []monitor.Monitor{{Name: "a", URL: "http://a"}}}, fakeState{})

	resp := do(t, a, "/v1/monitors/nope/status")
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	body := decode[map[string]string](t, resp)
	if body["error"] != storage.ErrNotFound.Error() {
		t.Errorf("error = %q, want %q", body["error"], storage.ErrNotFound)
	}
}

func TestDashboardRoute(t *testing.T) {
	now := time.Unix(1000, 0)
	a := New(
		fakeReader{
			monitors: []monitor.Monitor{{Name: "a", URL: "http://a"}, {Name: "b", URL: "http://b"}},
			hist: map[string][]monitor.Result{
				"a": {
					{MonitorName: "a", StatusCode: 200, CheckedAt: now, Latency: 5 * time.Millisecond},
					{MonitorName: "a", StatusCode: 500, CheckedAt: now.Add(time.Second), Latency: 7 * time.Millisecond},
				},
			},
		},
		fakeState{"a": monitor.StateDown},
	)

	resp := do(t, a, "/v1/dashboard")
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := decode[DashboardResponse](t, resp)
	if len(body.Monitors) != 2 {
		t.Fatalf("monitors len = %d, want 2", len(body.Monitors))
	}

	// With history: machine state, uptime and the per-check bars come through.
	ha := body.Monitors[0]
	if ha.Name != "a" || ha.State != "down" || ha.UptimePct != 50 || len(ha.Recent) != 2 {
		t.Errorf("monitors[0] = %+v, want a/down/50/2 checks", ha)
	}
	// Without history: unknown state, no bars.
	hb := body.Monitors[1]
	if hb.Name != "b" || hb.State != "unknown" || len(hb.Recent) != 0 {
		t.Errorf("monitors[1] = %+v, want b/unknown/no checks", hb)
	}
}

func TestRoutesMapStoreErrorTo500(t *testing.T) {
	boom := errors.New("store down")
	tests := []struct {
		name   string
		store  fakeReader
		target string
	}{
		{"status", fakeReader{listErr: boom}, "/v1/monitors/a/status"},
		{"dashboard list", fakeReader{listErr: boom}, "/v1/dashboard"},
		{
			"dashboard history",
			fakeReader{monitors: []monitor.Monitor{{Name: "a", URL: "http://a"}}, histErr: boom},
			"/v1/dashboard",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := do(t, New(tt.store, fakeState{}), tt.target)
			if resp.StatusCode != fiber.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", resp.StatusCode)
			}
			if body := decode[map[string]string](t, resp); body["error"] != boom.Error() {
				t.Errorf("error = %q, want %q", body["error"], boom)
			}
		})
	}
}

// TestServeAndShutdown drives the real listener path: Serve accepts requests on
// the given net.Listener and Shutdown releases it, returning from Serve cleanly.
func TestServeAndShutdown(t *testing.T) {
	a := New(fakeReader{monitors: []monitor.Monitor{{Name: "a", URL: "http://a"}}}, fakeState{"a": monitor.StateUp})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	serveErr := make(chan error, 1)
	go func() { serveErr <- a.Serve(ln) }()

	client := &http.Client{Timeout: 2 * time.Second}
	var resp *http.Response
	for i := 0; ; i++ {
		resp, err = client.Get("http://" + addr + "/v1/dashboard")
		if err == nil {
			break
		}
		if i == 50 {
			t.Fatalf("server never came up: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	select {
	case err := <-serveErr:
		if err != nil {
			t.Errorf("Serve returned %v, want nil after shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after Shutdown")
	}
}

func TestHTTPStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"not found", storage.ErrNotFound, fiber.StatusNotFound},
		{"wrapped not found", fmt.Errorf("history: %w", storage.ErrNotFound), fiber.StatusNotFound},
		{"other", errors.New("boom"), fiber.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := httpStatus(tt.err); got != tt.want {
				t.Errorf("httpStatus(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}
