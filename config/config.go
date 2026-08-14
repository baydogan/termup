package config

import (
	"fmt"
	"net/url"
	"os"

	"github.com/baydogan/termup/monitor"
	"github.com/goccy/go-yaml"
)

type Config struct {
	Monitors  []MonitorConfig  `yaml:"monitors"`
	Notifiers []NotifierConfig `yaml:"notifiers"`
}

type MonitorConfig struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// NotifierConfig is an outbound alert target. Type validity is checked when the
// adapter is built at boot; here we only require type and url to be present.
type NotifierConfig struct {
	Type string `yaml:"type"`
	URL  string `yaml:"url"`
}

// Load reads, parses and validates the config file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validate() error {
	if len(c.Monitors) == 0 {
		return fmt.Errorf("config: no monitors defined")
	}
	seen := make(map[string]struct{})
	for i, m := range c.Monitors {
		if m.Name == "" {
			return fmt.Errorf("config: monitor #%d has empty name", i)
		}
		if m.URL == "" {
			return fmt.Errorf("config: monitor %q has empty url", m.Name)
		}
		if err := validateURL(m.URL); err != nil {
			return fmt.Errorf("config: monitor %q has invalid url %q: %w", m.Name, m.URL, err)
		}
		if _, dup := seen[m.Name]; dup {
			return fmt.Errorf("config: duplicate monitor name %q", m.Name)
		}
		seen[m.Name] = struct{}{}
	}
	for i, n := range c.Notifiers {
		if n.Type == "" {
			return fmt.Errorf("config: notifier #%d has empty type", i)
		}
		if n.URL == "" {
			return fmt.Errorf("config: notifier %q has empty url", n.Type)
		}
	}
	return nil
}

// validateURL requires what the HTTP prober can actually probe: an absolute
// http(s) URL with a host. Caught here, at load time, instead of failing every
// probe cycle forever. Widens when non-HTTP probers arrive (tcp://, dns://).
func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("not a url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("missing host")
	}
	return nil
}

// ToMonitors maps config entries to domain monitors for store seeding.
func (c *Config) ToMonitors() []monitor.Monitor {
	out := make([]monitor.Monitor, 0, len(c.Monitors))
	for _, m := range c.Monitors {
		out = append(out, monitor.Monitor{Name: m.Name, URL: m.URL})
	}
	return out
}
