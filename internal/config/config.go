package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Config struct {
	UpstreamDNS   string          `json:"upstream_dns"`
	RedirectTo    string          `json:"redirect_to"`
	DNSListen     string          `json:"dns_listen"`
	APIListen     string          `json:"api_listen"`
	LogBlocked    bool            `json:"log_blocked"`
	Categories    map[string]bool `json:"categories"`
	CustomBlocked []string        `json:"custom_blocked"`
	CustomAllowed []string        `json:"custom_allowed"`

	mu   sync.RWMutex
	path string
}

func Default() *Config {
	return &Config{
		UpstreamDNS: "8.8.8.8:53",
		RedirectTo:  "127.0.0.1",
		DNSListen:   "127.0.0.1:53",
		APIListen:   "127.0.0.1:6144",
		LogBlocked:  true,
		Categories: map[string]bool{
			"adult":        true,
			"social_media": true,
			"gambling":     false,
			"streaming":    false,
			"doh_bypass":   true,
		},
		CustomBlocked: []string{},
		CustomAllowed: []string{},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()
	cfg.path = path

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.path == "" {
		c.path = DefaultPath()
	}

	if err := os.MkdirAll(filepath.Dir(c.path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0600)
}

func (c *Config) SetPath(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.path = path
}

func (c *Config) Path() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.path
}

func (c *Config) Update(fn func(*Config)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fn(c)
}

func (c *Config) Read(fn func(*Config)) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	fn(c)
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".veil", "config.json")
}
