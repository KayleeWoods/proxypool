package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Listen string `yaml:"listen"`
}

type HealthConfig struct {
	TestURL            string        `yaml:"test_url"`
	Interval           time.Duration `yaml:"interval"`
	Timeout            time.Duration `yaml:"timeout"`
	MaxFailures        int           `yaml:"max_failures"`
	QuarantineDuration time.Duration `yaml:"quarantine_duration"`
}

type ProxyConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

func (p ProxyConfig) Addr() string {
	return fmt.Sprintf("%s:%d", p.Host, p.Port)
}

func (p ProxyConfig) URL(scheme string) string {
	if p.Username != "" {
		return fmt.Sprintf("%s://%s:%s@%s:%d", scheme, p.Username, p.Password, p.Host, p.Port)
	}
	return fmt.Sprintf("%s://%s:%d", scheme, p.Host, p.Port)
}

type PoolConfig struct {
	Name      string        `yaml:"name"`
	Type      string        `yaml:"type"`     // http, socks5
	Rotation  string        `yaml:"rotation"` // round_robin, random, sticky
	StickyTTL time.Duration `yaml:"sticky_ttl"`
	Proxies   []ProxyConfig `yaml:"proxies"`
}

type Config struct {
	Server ServerConfig `yaml:"server"`
	Health HealthConfig `yaml:"health"`
	Pools  []PoolConfig `yaml:"pools"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Server.Listen == "" {
		cfg.Server.Listen = "127.0.0.1:8899"
	}
	if cfg.Health.TestURL == "" {
		cfg.Health.TestURL = "http://api.ipify.org?format=json"
	}
	if cfg.Health.Interval == 0 {
		cfg.Health.Interval = 60 * time.Second
	}
	if cfg.Health.Timeout == 0 {
		cfg.Health.Timeout = 10 * time.Second
	}
	if cfg.Health.MaxFailures == 0 {
		cfg.Health.MaxFailures = 1
	}
	if cfg.Health.QuarantineDuration == 0 {
		cfg.Health.QuarantineDuration = 60 * time.Second
	}
	for i := range cfg.Pools {
		p := &cfg.Pools[i]
		if p.Type == "" {
			p.Type = "http"
		}
		if p.Rotation == "" {
			p.Rotation = "round_robin"
		}
		if p.StickyTTL == 0 {
			p.StickyTTL = 300 * time.Second
		}
	}
	return &cfg, nil
}
