package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const appName = "ktui"
const DefaultProfile = "default"

var userConfigDir = os.UserConfigDir

type Profile struct {
	URL    string `json:"url"`
	APIKey string `json:"api_key,omitempty"`
}

type Config struct {
	URL            string             `json:"-"`
	APIKey         string             `json:"-"`
	Profile        string             `json:"profile"`
	Profiles       map[string]Profile `json:"profiles,omitempty"`
	Interval       string             `json:"interval"`
	Timeout        string             `json:"timeout"`
	Mode           string             `json:"mode"`
	RealtimeWindow string             `json:"realtime_window"`
	ASCII          bool               `json:"ascii"`
	NoColor        bool               `json:"no_color"`
	ChartYAxis     string             `json:"chart_y_axis"`
	WarnCPU        float64            `json:"warn_cpu"`
	WarnRAM        float64            `json:"warn_ram"`
	WarnDisk       float64            `json:"warn_disk"`
	WarnExpiryDays int                `json:"warn_expiry_days"`
}

func Default() Config {
	return Config{
		Profile:        DefaultProfile,
		Profiles:       map[string]Profile{DefaultProfile: {}},
		Interval:       "5s",
		Timeout:        "10s",
		Mode:           "sheet",
		RealtimeWindow: "1m",
		ChartYAxis:     "absolute",
		WarnCPU:        90,
		WarnRAM:        85,
		WarnDisk:       90,
		WarnExpiryDays: 7,
	}
}

func (c *Config) UnmarshalJSON(data []byte) error {
	var raw struct {
		URL            string             `json:"url"`
		APIKey         string             `json:"api_key"`
		Profile        string             `json:"profile"`
		Profiles       map[string]Profile `json:"profiles"`
		Interval       string             `json:"interval"`
		Timeout        string             `json:"timeout"`
		Mode           string             `json:"mode"`
		RealtimeWindow string             `json:"realtime_window"`
		ASCII          bool               `json:"ascii"`
		NoColor        bool               `json:"no_color"`
		ChartYAxis     string             `json:"chart_y_axis"`
		WarnCPU        float64            `json:"warn_cpu"`
		WarnRAM        float64            `json:"warn_ram"`
		WarnDisk       float64            `json:"warn_disk"`
		WarnExpiryDays int                `json:"warn_expiry_days"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*c = Config{
		URL:            strings.TrimSpace(raw.URL),
		APIKey:         strings.TrimSpace(raw.APIKey),
		Profile:        strings.TrimSpace(raw.Profile),
		Profiles:       raw.Profiles,
		Interval:       raw.Interval,
		Timeout:        raw.Timeout,
		Mode:           raw.Mode,
		RealtimeWindow: raw.RealtimeWindow,
		ASCII:          raw.ASCII,
		NoColor:        raw.NoColor,
		ChartYAxis:     raw.ChartYAxis,
		WarnCPU:        raw.WarnCPU,
		WarnRAM:        raw.WarnRAM,
		WarnDisk:       raw.WarnDisk,
		WarnExpiryDays: raw.WarnExpiryDays,
	}
	return nil
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
	profileName := strings.TrimSpace(c.Profile)
	if profileName == "" {
		profileName = defaults.Profile
	}
	c.Profile = profileName
	c.Profiles = normalizedProfiles(c.Profiles)
	if len(c.Profiles) == 0 {
		c.Profiles = map[string]Profile{}
	}
	legacy := Profile{URL: strings.TrimSpace(c.URL), APIKey: strings.TrimSpace(c.APIKey)}
	if _, ok := c.Profiles[c.Profile]; !ok {
		c.Profiles[c.Profile] = legacy
	}
	active := c.Profiles[c.Profile]
	if strings.TrimSpace(c.URL) == "" {
		c.URL = active.URL
	} else {
		c.URL = strings.TrimSpace(c.URL)
	}
	if strings.TrimSpace(c.APIKey) == "" {
		c.APIKey = active.APIKey
	} else {
		c.APIKey = strings.TrimSpace(c.APIKey)
	}
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
	if strings.TrimSpace(c.RealtimeWindow) == "" {
		c.RealtimeWindow = defaults.RealtimeWindow
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
	if _, err := NormalizeProfileName(c.Profile); err != nil {
		return err
	}
	if _, ok := c.Profiles[c.Profile]; !ok {
		return fmt.Errorf("active profile %q is not configured", c.Profile)
	}
	for name := range c.Profiles {
		if _, err := NormalizeProfileName(name); err != nil {
			return err
		}
	}
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
	if _, err := c.RealtimeWindowDuration(); err != nil {
		return err
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

func NormalizeProfileName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", fmt.Errorf("profile name is required")
	}
	for _, r := range name {
		if unicode.IsSpace(r) || r == '/' || r == '\\' {
			return "", fmt.Errorf("invalid profile name %q: use a name without spaces or slashes", value)
		}
	}
	return name, nil
}

func normalizedProfiles(profiles map[string]Profile) map[string]Profile {
	out := make(map[string]Profile, len(profiles))
	for name, profile := range profiles {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		profile.URL = strings.TrimSpace(profile.URL)
		profile.APIKey = strings.TrimSpace(profile.APIKey)
		out[name] = profile
	}
	return out
}

func (c Config) ActiveProfile() Profile {
	c = c.WithDefaults()
	return c.Profiles[c.Profile]
}

func (c Config) ProfileNames() []string {
	c = c.WithDefaults()
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c Config) Redacted() Config {
	c = c.WithDefaults()
	profiles := make(map[string]Profile, len(c.Profiles))
	for name, profile := range c.Profiles {
		if profile.APIKey != "" {
			profile.APIKey = "********"
		}
		profiles[name] = profile
	}
	c.Profiles = profiles
	if c.APIKey != "" {
		c.APIKey = "********"
	}
	return c
}

func SetActiveProfileConnection(cfg Config, url string, apiKey string) (Config, error) {
	cfg = cfg.WithDefaults()
	profile := Profile{URL: strings.TrimSpace(url), APIKey: strings.TrimSpace(apiKey)}
	cfg.Profiles[cfg.Profile] = profile
	cfg.URL = profile.URL
	cfg.APIKey = profile.APIKey
	return cfg, cfg.Validate()
}

func AddProfile(cfg Config, name string, url string, apiKey string) (Config, error) {
	profileName, err := NormalizeProfileName(name)
	if err != nil {
		return cfg, err
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return cfg, fmt.Errorf("profile %q URL is required", profileName)
	}
	cfg = cfg.WithDefaults()
	cfg.Profiles[profileName] = Profile{URL: url, APIKey: strings.TrimSpace(apiKey)}
	if cfg.Profile == profileName {
		cfg.URL = url
		cfg.APIKey = strings.TrimSpace(apiKey)
	}
	return cfg, cfg.Validate()
}

func UseProfile(cfg Config, name string) (Config, error) {
	profileName, err := NormalizeProfileName(name)
	if err != nil {
		return cfg, err
	}
	cfg = cfg.WithDefaults()
	profile, ok := cfg.Profiles[profileName]
	if !ok {
		return cfg, fmt.Errorf("profile %q is not configured", profileName)
	}
	cfg.Profile = profileName
	cfg.URL = profile.URL
	cfg.APIKey = profile.APIKey
	return cfg, cfg.Validate()
}

func RemoveProfile(cfg Config, name string) (Config, error) {
	profileName, err := NormalizeProfileName(name)
	if err != nil {
		return cfg, err
	}
	cfg = cfg.WithDefaults()
	if _, ok := cfg.Profiles[profileName]; !ok {
		return cfg, fmt.Errorf("profile %q is not configured", profileName)
	}
	delete(cfg.Profiles, profileName)
	if len(cfg.Profiles) == 0 {
		cfg.Profile = DefaultProfile
		cfg.Profiles[DefaultProfile] = Profile{}
	} else if cfg.Profile == profileName {
		names := make([]string, 0, len(cfg.Profiles))
		for name := range cfg.Profiles {
			names = append(names, name)
		}
		sort.Strings(names)
		cfg.Profile = names[0]
	}
	active := cfg.Profiles[cfg.Profile]
	cfg.URL = active.URL
	cfg.APIKey = active.APIKey
	return cfg, cfg.Validate()
}

func RenameProfile(cfg Config, oldName string, newName string) (Config, error) {
	from, err := NormalizeProfileName(oldName)
	if err != nil {
		return cfg, err
	}
	to, err := NormalizeProfileName(newName)
	if err != nil {
		return cfg, err
	}
	cfg = cfg.WithDefaults()
	if from == to {
		return cfg, cfg.Validate()
	}
	profile, ok := cfg.Profiles[from]
	if !ok {
		return cfg, fmt.Errorf("profile %q is not configured", from)
	}
	if _, ok := cfg.Profiles[to]; ok {
		return cfg, fmt.Errorf("profile %q already exists", to)
	}
	delete(cfg.Profiles, from)
	cfg.Profiles[to] = profile
	if cfg.Profile == from {
		cfg.Profile = to
		cfg.URL = profile.URL
		cfg.APIKey = profile.APIKey
	}
	return cfg, cfg.Validate()
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

func (c Config) RealtimeWindowDuration() (time.Duration, error) {
	parsed, ok := parseRealtimeWindow(c.RealtimeWindow)
	if !ok {
		return 0, fmt.Errorf("invalid config realtime_window %q: use 1m, 5m, or 10m", c.RealtimeWindow)
	}
	return parsed, nil
}

func parseRealtimeWindow(value string) (time.Duration, bool) {
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	switch parsed {
	case time.Minute, 5 * time.Minute, 10 * time.Minute:
		return parsed, true
	default:
		return 0, false
	}
}

func realtimeWindowText(value time.Duration) string {
	switch value {
	case time.Minute:
		return "1m"
	case 5 * time.Minute:
		return "5m"
	case 10 * time.Minute:
		return "10m"
	default:
		return ""
	}
}

func Set(cfg Config, key string, value string) (Config, error) {
	cfg = cfg.WithDefaults()
	key = strings.TrimSpace(strings.ToLower(key))
	value = strings.TrimSpace(value)
	var err error
	switch key {
	case "url":
		cfg, err = SetActiveProfileConnection(cfg, value, cfg.APIKey)
		if err != nil {
			return cfg, err
		}
	case "api_key", "api-key":
		cfg, err = SetActiveProfileConnection(cfg, cfg.URL, value)
		if err != nil {
			return cfg, err
		}
	case "interval":
		cfg.Interval = value
	case "timeout":
		cfg.Timeout = value
	case "mode":
		cfg.Mode = value
	case "profile":
		cfg, err = UseProfile(cfg, value)
		if err != nil {
			return cfg, err
		}
	case "realtime_window", "realtime-window":
		parsed, ok := parseRealtimeWindow(value)
		if !ok {
			return cfg, fmt.Errorf("invalid realtime_window %q: use 1m, 5m, or 10m", value)
		}
		cfg.RealtimeWindow = realtimeWindowText(parsed)
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
