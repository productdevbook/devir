package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Service struct {
	Dir   string `yaml:"dir"`
	Cmd   string `yaml:"cmd"`
	Port  int    `yaml:"port"`
	Color string `yaml:"color"`
}

type Config struct {
	Services map[string]Service `yaml:"services"`
	Defaults []string           `yaml:"defaults"`
	RootDir  string             `yaml:"-"` // Computed from config file location
}

func loadConfig(path string) (*Config, error) {
	if path == "" {
		path = findConfigFile()
	}

	if path == "" {
		return nil, fmt.Errorf("devir.yaml not found")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Set root dir from config file location
	cfg.RootDir = filepath.Dir(path)

	// Set defaults if not specified
	if len(cfg.Defaults) == 0 {
		for name := range cfg.Services {
			cfg.Defaults = append(cfg.Defaults, name)
		}
	}

	// Validate services
	for name, svc := range cfg.Services {
		if svc.Dir == "" {
			return nil, fmt.Errorf("service %s: dir is required", name)
		}
		if svc.Cmd == "" {
			return nil, fmt.Errorf("service %s: cmd is required", name)
		}
		if svc.Color == "" {
			// Set default color
			svc.Color = "white"
			cfg.Services[name] = svc
		}
	}

	return &cfg, nil
}
