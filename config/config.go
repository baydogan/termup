package config

import (
	"fmt"
	"os"

	"github.com/baydogan/zerotolerance/monitor"
	"github.com/goccy/go-yaml"
)

type Config struct {
	Monitors []MonitorConfig `yaml:"monitors"`
}

type MonitorConfig struct {
	Name string `yaml:"name"`
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
		if _, dup := seen[m.Name]; dup {
			return fmt.Errorf("config: duplicate monitor name %q", m.Name)
		}
		seen[m.Name] = struct{}{}
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
