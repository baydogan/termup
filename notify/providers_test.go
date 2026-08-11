package notify_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/baydogan/termup/monitor"
	"github.com/baydogan/termup/notify"
)

// captureServer records the body and content-type of a single POST.
func captureServer(t *testing.T) (*httptest.Server, *[]byte, *string) {
	t.Helper()
	var body []byte
	var ctype string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		ctype = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)
	return ts, &body, &ctype
}

func stateEvent() notify.Event {
	return notify.Event{
		Kind:    notify.KindStateChange,
		Monitor: "local",
		From:    monitor.StateUp,
		To:      monitor.StateDown,
		Result:  monitor.Result{StatusCode: 500},
	}
}

func TestSlackPayload(t *testing.T) {
	ts, body, ctype := captureServer(t)
	if err := notify.NewSlack(ts.URL).Notify(stateEvent()); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if *ctype != "application/json" {
		t.Errorf("content-type = %q", *ctype)
	}
	var got map[string]string
	if err := json.Unmarshal(*body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if want := "local: up -> down (code=500)"; got["text"] != want {
		t.Errorf("text = %q, want %q", got["text"], want)
	}
}

func TestDiscordPayload(t *testing.T) {
	ts, body, _ := captureServer(t)
	if err := notify.NewDiscord(ts.URL).Notify(stateEvent()); err != nil {
		t.Fatalf("notify: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(*body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := got["content"]; !ok {
		t.Errorf("payload missing content key: %s", *body)
	}
}

func TestWebhookPayload(t *testing.T) {
	ts, body, _ := captureServer(t)
	e := notify.Event{
		Kind:       notify.KindCertExpiring,
		Monitor:    "local",
		CertExpiry: time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC),
		DaysLeft:   7,
	}
	if err := notify.NewWebhook(ts.URL).Notify(e); err != nil {
		t.Fatalf("notify: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(*body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["kind"] != "cert_expiring" || got["monitor"] != "local" {
		t.Errorf("payload = %s", *body)
	}
	if got["daysLeft"] != float64(7) {
		t.Errorf("daysLeft = %v, want 7", got["daysLeft"])
	}
}

func TestBuildKnownAndUnknown(t *testing.T) {
	for _, typ := range []string{"slack", "discord", "webhook"} {
		if n, err := notify.Build(typ, "http://x"); err != nil || n == nil {
			t.Errorf("Build(%q) = %v, %v", typ, n, err)
		}
	}
	if _, err := notify.Build("telegram", "http://x"); err == nil {
		t.Error("Build(unknown) should error")
	}
}

func TestWebhookNon2xxErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	if err := notify.NewWebhook(ts.URL).Notify(stateEvent()); err == nil {
		t.Error("expected error on 500 response")
	}
}
