// internal/config/config.go
package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration wraps time.Duration for YAML "1s"/"500ms" strings.
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return fmt.Errorf("duration must be a string (e.g., \"2s\"): %w", err)
	}
	// env expansion (rare, but supported)
	s = expandEnvDefault(s)
	if s == "" {
		d.Duration = 0
		return nil
	}
	dd, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = dd
	return nil
}

type Config struct {
	LogLevel string `yaml:"logLevel"` // info | debug | warn | error

	Miner struct {
		ID     string `yaml:"id"`
		Listen string `yaml:"listen"` // e.g., ":8080"
		Region string `yaml:"region"`
	} `yaml:"miner"`

	MediaMTX struct {
		API          string   `yaml:"api"`          // e.g., http://127.0.0.1:9997
		PollInterval Duration `yaml:"pollInterval"` // e.g., "2s"
	} `yaml:"mediamtx"`

	Metrics struct {
		Enable bool   `yaml:"enable"`
		Path   string `yaml:"path"` // e.g., "/metrics"
	} `yaml:"metrics"`

	Presence struct {
		Enable bool `yaml:"enable"`
	} `yaml:"presence"`

	Service struct {
		Enable bool `yaml:"enable"`
	} `yaml:"service"`
}

// Load reads, environment-expands, parses YAML, applies defaults, and validates.
func Load(path string) (*Config, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	// First pass: basic YAML → struct (strings may still contain ${} tokens)
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	// Expand environment variables (with defaults) on known string fields.
	cfg.LogLevel = expandEnvDefault(cfg.LogLevel)

	cfg.Miner.ID = expandEnvDefault(cfg.Miner.ID)
	cfg.Miner.Listen = expandEnvDefault(cfg.Miner.Listen)
	cfg.Miner.Region = expandEnvDefault(cfg.Miner.Region)

	cfg.MediaMTX.API = expandEnvDefault(cfg.MediaMTX.API)

	cfg.Metrics.Path = expandEnvDefault(cfg.Metrics.Path)

	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func applyDefaults(c *Config) {
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	if c.Miner.Listen == "" {
		c.Miner.Listen = ":8080"
	}
	if c.MediaMTX.API == "" {
		// matches docker-compose env var in examples
		c.MediaMTX.API = "http://127.0.0.1:9997"
	}
	if c.MediaMTX.PollInterval.Duration == 0 {
		c.MediaMTX.PollInterval = Duration{Duration: 2 * time.Second}
	}
	if c.Metrics.Path == "" {
		c.Metrics.Path = "/metrics"
	}
}

func validate(c *Config) error {
	if c.Miner.ID == "" {
		return errors.New("miner.id is required")
	}
	if c.Miner.Listen == "" {
		return errors.New("miner.listen is required")
	}
	if c.MediaMTX.API == "" {
		return errors.New("mediamtx.api is required")
	}
	// simple sanity: reasonable poll interval
	if c.MediaMTX.PollInterval.Duration < 200*time.Millisecond {
		return fmt.Errorf("mediamtx.pollInterval too small: %s", c.MediaMTX.PollInterval.Duration)
	}
	return nil
}

// --- env expansion with ${VAR} and ${VAR:default} ---

var envRe = regexp.MustCompile(`\$\{([^}:]+)(?::([^}]*))?\}`)

// expandEnvDefault replaces ${VAR} with os.Getenv("VAR"),
// and ${VAR:default} with env value or "default" if unset.
func expandEnvDefault(s string) string {
	if s == "" {
		return s
	}
	return envRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := envRe.FindStringSubmatch(m)
		if len(parts) != 3 {
			return m
		}
		name := parts[1]
		def := parts[2]
		if val, ok := os.LookupEnv(name); ok {
			return val
		}
		return def
	})
}
