package config

import "testing"

func TestValidateMonitorURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"http", "http://example.com", false},
		{"https with port, path and query", "https://example.com:8443/status?x=1", false},
		{"ip literal", "http://127.0.0.1:1", false},
		{"userinfo", "https://user:pass@example.com", false},
		{"empty", "", true},
		{"no scheme", "example.com", true},
		{"bare word", "not-a-url", true},
		{"unsupported scheme", "ftp://example.com", true},
		{"tcp scheme (later phase)", "tcp://example.com:443", true},
		{"scheme without host", "http://", true},
		{"leading space", " http://example.com", true},
		{"control character", "http://exa\x7fmple.com", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{Monitors: []MonitorConfig{{Name: "a", URL: tc.url}}}
			err := c.validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("validate() err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateNotifiers(t *testing.T) {
	base := []MonitorConfig{{Name: "a", URL: "http://a"}}

	cases := []struct {
		name    string
		ns      []NotifierConfig
		wantErr bool
	}{
		{"none is fine", nil, false},
		{"valid", []NotifierConfig{{Type: "slack", URL: "http://x"}}, false},
		{"empty type", []NotifierConfig{{Type: "", URL: "http://x"}}, true},
		{"empty url", []NotifierConfig{{Type: "slack", URL: ""}}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{Monitors: base, Notifiers: tc.ns}
			err := c.validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("validate() err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}
