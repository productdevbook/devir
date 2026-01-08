package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// ServiceType defines the type of service
type ServiceType string

const (
	ServiceTypeDefault  ServiceType = ""         // Long-running service (default)
	ServiceTypeService  ServiceType = "service"  // Explicit long-running service
	ServiceTypeOneshot  ServiceType = "oneshot"  // Run once and exit
	ServiceTypeInterval ServiceType = "interval" // Run periodically
	ServiceTypeHTTP     ServiceType = "http"     // HTTP request
)

// Service represents a single service configuration
type Service struct {
	Dir      string        `yaml:"dir"`
	Cmd      string        `yaml:"cmd"`
	Port     int           `yaml:"port"`
	Color    string        `yaml:"color"`
	Icon     string        `yaml:"icon"`     // custom icon/emoji for display
	Type     ServiceType   `yaml:"type"`     // service, oneshot, interval, http
	Interval time.Duration `yaml:"interval"` // for interval type
	URL      string        `yaml:"url"`      // for http type
	Method   string        `yaml:"method"`   // GET, POST, etc.
	Body     string        `yaml:"body"`     // request body
	Headers  []string      `yaml:"headers"`  // custom headers (key: value format)
}

// IsLongRunning returns true if this service runs continuously
func (s *Service) IsLongRunning() bool {
	return s.Type == ServiceTypeDefault || s.Type == ServiceTypeService
}

// GetEffectiveType returns the effective service type (handles empty default)
func (s *Service) GetEffectiveType() ServiceType {
	if s.Type == ServiceTypeDefault {
		return ServiceTypeService
	}
	return s.Type
}

// Config represents the devir configuration
type Config struct {
	Services map[string]Service `yaml:"services"`
	Defaults []string           `yaml:"defaults"`
	RootDir  string             `yaml:"-"` // Computed from config file location
}

// Load loads configuration from the given path or searches for devir.yaml
func Load(path string) (*Config, error) {
	if path == "" {
		path = FindConfigFile()
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
		// Validate based on service type
		switch svc.Type {
		case ServiceTypeHTTP:
			// HTTP type requires URL, not cmd
			if svc.URL == "" {
				return nil, fmt.Errorf("service %s: url is required for http type", name)
			}
			if svc.Method == "" {
				svc.Method = "GET"
			}
		case ServiceTypeInterval:
			// Interval type requires cmd and interval
			if svc.Cmd == "" {
				return nil, fmt.Errorf("service %s: cmd is required", name)
			}
			if svc.Interval <= 0 {
				return nil, fmt.Errorf("service %s: interval is required for interval type", name)
			}
		default:
			// Default and oneshot types require dir and cmd
			if svc.Dir == "" && svc.Type != ServiceTypeOneshot {
				return nil, fmt.Errorf("service %s: dir is required", name)
			}
			if svc.Cmd == "" {
				return nil, fmt.Errorf("service %s: cmd is required", name)
			}
		}

		// Set default color
		if svc.Color == "" {
			svc.Color = "white"
		}

		// Set default dir for oneshot if not specified
		if svc.Dir == "" && svc.Type == ServiceTypeOneshot {
			svc.Dir = "."
		}

		cfg.Services[name] = svc
	}

	return &cfg, nil
}

// FindConfigFile looks for devir.yaml in current dir and parents
func FindConfigFile() string {
	dir, _ := os.Getwd()
	for {
		path := filepath.Join(dir, "devir.yaml")
		if _, err := os.Stat(path); err == nil {
			return path
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
