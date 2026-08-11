package main

import (
	"testing"

	"github.com/baydogan/termup/api"
	"github.com/charmbracelet/bubbles/textinput"
)

func TestVisibleFilter(t *testing.T) {
	m := model{
		filter: textinput.New(),
		health: []api.MonitorHealthDTO{
			{Name: "api-prod", URL: "https://api.example.com"},
			{Name: "web", URL: "https://shop.example.com"},
			{Name: "db", URL: "https://db.internal"},
		},
	}

	if got := len(m.visible()); got != 3 {
		t.Fatalf("empty filter: %d visible, want 3", got)
	}

	// Case-insensitive name substring.
	m.filter.SetValue("API")
	if v := m.visible(); len(v) != 1 || v[0].Name != "api-prod" {
		t.Errorf("name filter: %v, want [api-prod]", names(v))
	}

	// URL substring matches multiple.
	m.filter.SetValue("example.com")
	if v := m.visible(); len(v) != 2 {
		t.Errorf("url filter: %v, want 2", names(v))
	}

	// No match.
	m.filter.SetValue("zzz")
	if got := len(m.visible()); got != 0 {
		t.Errorf("no-match filter: %d visible, want 0", got)
	}
}

func names(hs []api.MonitorHealthDTO) []string {
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = h.Name
	}
	return out
}
