package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type ServiceConfig struct {
	Name string `yaml:"name" json:"name"`
	Type string `yaml:"type" json:"type"`
	URL  string `yaml:"url"  json:"url"`
}

type Config struct {
	Port int `yaml:"port"`
	// ScanPorts, if set (e.g. "8000-8010"), enables port-range scanning
	// for vLLM instances during both startup and the periodic rediscover loop.
	ScanPorts string `yaml:"scan_ports,omitempty"`
	// RescanInterval is a Go duration string (e.g. "30m"). Defaults to 30m.
	// Set to "0" to disable the background rediscover loop.
	RescanInterval string          `yaml:"rescan_interval,omitempty"`
	Services       []ServiceConfig `yaml:"services"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Port == 0 {
		cfg.Port = 9100
	}
	return &cfg, nil
}

func saveConfig(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
