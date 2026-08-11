package config

import "testing"

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
