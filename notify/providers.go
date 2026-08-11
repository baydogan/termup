package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/baydogan/termup/monitor"
)

var (
	_ Notifier = (*Slack)(nil)
	_ Notifier = (*Discord)(nil)
	_ Notifier = (*Webhook)(nil)
)

// Build constructs a provider notifier from its config type + url.
func Build(typ, url string) (Notifier, error) {
	switch typ {
	case "slack":
		return NewSlack(url), nil
	case "discord":
		return NewDiscord(url), nil
	case "webhook":
		return NewWebhook(url), nil
	default:
		return nil, fmt.Errorf("unknown notifier type %q", typ)
	}
}

// Message is the one-line human text shared by the chat providers.
func Message(e Event) string {
	switch e.Kind {
	case KindStateChange:
		return fmt.Sprintf("%s: %s -> %s (code=%d)", e.Monitor, e.From, e.To, e.Result.StatusCode)
	case KindCertExpiring:
		return fmt.Sprintf("%s: certificate expires in %dd (%s)",
			e.Monitor, e.DaysLeft, e.CertExpiry.Format("2006-01-02"))
	case KindFlapping:
		return fmt.Sprintf("%s: flapping (%d flips in last %d)",
			e.Monitor, e.Flips, monitor.FlapWindow)
	}
	return ""
}

// Slack posts to an incoming-webhook URL as {"text": ...}.
type Slack struct {
	url    string
	client *http.Client
}

func NewSlack(url string) *Slack { return &Slack{url: url, client: httpClient()} }

func (s *Slack) Notify(e Event) error {
	return postJSON(s.client, s.url, map[string]string{"text": Message(e)})
}

// Discord posts to a webhook URL as {"content": ...}.
type Discord struct {
	url    string
	client *http.Client
}

func NewDiscord(url string) *Discord { return &Discord{url: url, client: httpClient()} }

func (d *Discord) Notify(e Event) error {
	return postJSON(d.client, d.url, map[string]string{"content": Message(e)})
}

// Webhook posts the event as a structured JSON payload.
type Webhook struct {
	url    string
	client *http.Client
}

func NewWebhook(url string) *Webhook { return &Webhook{url: url, client: httpClient()} }

func (w *Webhook) Notify(e Event) error {
	return postJSON(w.client, w.url, toPayload(e))
}

// webhookPayload is the wire shape for the generic webhook adapter.
type webhookPayload struct {
	Kind       string `json:"kind"`
	Monitor    string `json:"monitor"`
	From       string `json:"from,omitempty"`
	To         string `json:"to,omitempty"`
	StatusCode int    `json:"statusCode,omitempty"`
	CertExpiry int64  `json:"certExpiry,omitempty"`
	DaysLeft   int    `json:"daysLeft,omitempty"`
	Flips      int    `json:"flips,omitempty"`
}

func toPayload(e Event) webhookPayload {
	p := webhookPayload{Monitor: e.Monitor}
	switch e.Kind {
	case KindStateChange:
		p.Kind = "state_change"
		p.From = e.From.String()
		p.To = e.To.String()
		p.StatusCode = e.Result.StatusCode
	case KindCertExpiring:
		p.Kind = "cert_expiring"
		if !e.CertExpiry.IsZero() {
			p.CertExpiry = e.CertExpiry.Unix()
		}
		p.DaysLeft = e.DaysLeft
	case KindFlapping:
		p.Kind = "flapping"
		p.Flips = e.Flips
	}
	return p
}

func httpClient() *http.Client { return &http.Client{Timeout: 10 * time.Second} }

func postJSON(client *http.Client, url string, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post %s: %w", url, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 300 {
		return fmt.Errorf("post %s: status %d", url, resp.StatusCode)
	}
	return nil
}
