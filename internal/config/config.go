package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const appName = "ktui"

var userConfigDir = os.UserConfigDir

type Config struct {
	URL      string `json:"url"`
	APIKey   string `json:"api_key,omitempty"`
	Interval string `json:"interval"`
	Timeout  string `json:"timeout"`
	Mode     string `json:"mode"`
	ASCII    bool   `json:"ascii"`
	NoColor  bool   `json:"no_color"`
}

func Default() Config {
	return Config{
		Interval: "5s",
		Timeout:  "10s",
		Mode:     "sheet",
	}
}

func Path() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("KTUI_CONFIG")); dir != "" {
		return expandHome(dir)
	}
	dir, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(dir, appName, "config.json"), nil
}

func Load() (Config, string, error) {
	cfg := Default()
	path, err := Path()
	if err != nil {
		return cfg, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, path, nil
		}
		return cfg, path, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, path, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return cfg, path, err
	}
	return cfg, path, nil
}

func Save(cfg Config) (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write config %s: %w", path, err)
	}
	return path, nil
}

func (c Config) WithDefaults() Config {
	defaults := Default()
	if strings.TrimSpace(c.Interval) == "" {
		c.Interval = defaults.Interval
	}
	if strings.TrimSpace(c.Timeout) == "" {
		c.Timeout = defaults.Timeout
	}
	if strings.TrimSpace(c.Mode) == "" {
		c.Mode = defaults.Mode
	}
	return c
}

func (c Config) Validate() error {
	if _, err := c.IntervalDuration(); err != nil {
		return err
	}
	if _, err := c.TimeoutDuration(); err != nil {
		return err
	}
	switch c.Mode {
	case "sheet", "line":
	default:
		return fmt.Errorf("invalid config mode %q: use sheet or line", c.Mode)
	}
	return nil
}

func (c Config) IntervalDuration() (time.Duration, error) {
	value, err := time.ParseDuration(c.Interval)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid config interval %q", c.Interval)
	}
	return value, nil
}

func (c Config) TimeoutDuration() (time.Duration, error) {
	value, err := time.ParseDuration(c.Timeout)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid config timeout %q", c.Timeout)
	}
	return value, nil
}

func Set(cfg Config, key string, value string) (Config, error) {
	key = strings.TrimSpace(strings.ToLower(key))
	value = strings.TrimSpace(value)
	switch key {
	case "url":
		cfg.URL = value
	case "api_key", "api-key":
		cfg.APIKey = value
	case "interval":
		cfg.Interval = value
	case "timeout":
		cfg.Timeout = value
	case "mode":
		cfg.Mode = value
	case "ascii":
		parsed, err := parseBool(value)
		if err != nil {
			return cfg, err
		}
		cfg.ASCII = parsed
	case "no_color", "no-color":
		parsed, err := parseBool(value)
		if err != nil {
			return cfg, err
		}
		cfg.NoColor = parsed
	default:
		return cfg, fmt.Errorf("unknown config key %q", key)
	}
	cfg = cfg.WithDefaults()
	return cfg, cfg.Validate()
}

func parseBool(value string) (bool, error) {
	parsed, err := strconv.ParseBool(value)
	if err == nil {
		return parsed, nil
	}
	switch strings.ToLower(value) {
	case "yes", "on":
		return true, nil
	case "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool %q", value)
	}
}

func expandHome(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("find home directory: %w", err)
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}
