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
	// RealtimePoints limits realtime chart samples. 0 keeps the auto strategy.
	RealtimePoints int     `json:"realtime_points,omitempty"`
	ASCII          bool    `json:"ascii"`
	NoColor        bool    `json:"no_color"`
	ChartYAxis     string  `json:"chart_y_axis"`
	WarnCPU        float64 `json:"warn_cpu"`
	WarnRAM        float64 `json:"warn_ram"`
	WarnDisk       float64 `json:"warn_disk"`
	WarnExpiryDays int     `json:"warn_expiry_days"`
}

func Default() Config {
	return Config{
		Interval:       "5s",
		Timeout:        "10s",
		Mode:           "sheet",
		ChartYAxis:     "absolute",
		WarnCPU:        90,
		WarnRAM:        85,
		WarnDisk:       90,
		WarnExpiryDays: 7,
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
	if strings.TrimSpace(c.ChartYAxis) == "" {
		c.ChartYAxis = defaults.ChartYAxis
	}
	if c.WarnCPU == 0 {
		c.WarnCPU = defaults.WarnCPU
	}
	if c.WarnRAM == 0 {
		c.WarnRAM = defaults.WarnRAM
	}
	if c.WarnDisk == 0 {
		c.WarnDisk = defaults.WarnDisk
	}
	if c.WarnExpiryDays == 0 {
		c.WarnExpiryDays = defaults.WarnExpiryDays
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
	if c.RealtimePoints < 0 {
		return fmt.Errorf("invalid config realtime_points %d: use 0 or a positive number", c.RealtimePoints)
	}
	switch c.ChartYAxis {
	case "absolute", "relative":
	default:
		return fmt.Errorf("invalid config chart_y_axis %q: use absolute or relative", c.ChartYAxis)
	}
	if err := validatePercent("warn_cpu", c.WarnCPU); err != nil {
		return err
	}
	if err := validatePercent("warn_ram", c.WarnRAM); err != nil {
		return err
	}
	if err := validatePercent("warn_disk", c.WarnDisk); err != nil {
		return err
	}
	if c.WarnExpiryDays <= 0 {
		return fmt.Errorf("invalid config warn_expiry_days %d: use a positive number", c.WarnExpiryDays)
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
	case "realtime_points", "realtime-points":
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			return cfg, fmt.Errorf("invalid realtime_points %q", value)
		}
		cfg.RealtimePoints = parsed
	case "chart_y_axis", "chart-y-axis", "percent_y_axis", "percent-y-axis":
		value = strings.ToLower(value)
		switch value {
		case "absolute", "relative":
			cfg.ChartYAxis = value
		default:
			return cfg, fmt.Errorf("invalid chart_y_axis %q", value)
		}
	case "warn_cpu", "warn-cpu":
		parsed, err := parsePercent(value)
		if err != nil {
			return cfg, fmt.Errorf("invalid warn_cpu %q", value)
		}
		cfg.WarnCPU = parsed
	case "warn_ram", "warn-ram":
		parsed, err := parsePercent(value)
		if err != nil {
			return cfg, fmt.Errorf("invalid warn_ram %q", value)
		}
		cfg.WarnRAM = parsed
	case "warn_disk", "warn-disk":
		parsed, err := parsePercent(value)
		if err != nil {
			return cfg, fmt.Errorf("invalid warn_disk %q", value)
		}
		cfg.WarnDisk = parsed
	case "warn_expiry_days", "warn-expiry-days":
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return cfg, fmt.Errorf("invalid warn_expiry_days %q", value)
		}
		cfg.WarnExpiryDays = parsed
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

func parsePercent(value string) (float64, error) {
	parsed, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(value), "%"), 64)
	if err != nil {
		return 0, err
	}
	return parsed, validatePercent("", parsed)
}

func validatePercent(name string, value float64) error {
	if value <= 0 || value > 100 {
		if name == "" {
			name = "percent"
		}
		return fmt.Errorf("invalid config %s %.1f: use a value from 1 to 100", name, value)
	}
	return nil
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
