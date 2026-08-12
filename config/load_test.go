package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadValid(t *testing.T) {
	p := writeConfig(t, `
monitors:
  - name: a
    url: https://a.test
  - name: b
    url: https://b.test
notifiers:
  - type: slack
    url: https://hooks.example/x
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Monitors) != 2 {
		t.Errorf("monitors = %d, want 2", len(cfg.Monitors))
	}
	if len(cfg.Notifiers) != 1 || cfg.Notifiers[0].Type != "slack" {
		t.Errorf("notifiers = %+v", cfg.Notifiers)
	}

	ms := cfg.ToMonitors()
	if len(ms) != 2 || ms[0].Name != "a" || ms[0].URL != "https://a.test" {
		t.Errorf("ToMonitors = %+v", ms)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("expected error for a missing file")
	}
}

func TestLoadMalformedYAML(t *testing.T) {
	p := writeConfig(t, "monitors: [unclosed")
	if _, err := Load(p); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestLoadPropagatesValidationError(t *testing.T) {
	p := writeConfig(t, `
monitors:
  - name: dup
    url: https://a
  - name: dup
    url: https://b
`)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("Load err = %v, want a duplicate error", err)
	}
}

func TestValidateMonitors(t *testing.T) {
	cases := []struct {
		name    string
		ms      []MonitorConfig
		wantErr bool
	}{
		{"valid", []MonitorConfig{{Name: "a", URL: "http://a"}}, false},
		{"empty list", nil, true},
		{"empty name", []MonitorConfig{{Name: "", URL: "http://a"}}, true},
		{"empty url", []MonitorConfig{{Name: "a", URL: ""}}, true},
		{"duplicate name", []MonitorConfig{{Name: "a", URL: "http://a"}, {Name: "a", URL: "http://b"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{Monitors: tc.ms}
			if err := c.validate(); (err != nil) != tc.wantErr {
				t.Errorf("validate() = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
