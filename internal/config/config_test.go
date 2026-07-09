package config

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultURLIsEmpty(t *testing.T) {
	cfg := Default()
	if cfg.URL != "" {
		t.Fatalf("default URL = %q, want empty", cfg.URL)
	}
}

func TestWithDefaultsDoesNotFillURL(t *testing.T) {
	cfg := (Config{}).WithDefaults()
	if cfg.URL != "" {
		t.Fatalf("URL after defaults = %q, want empty", cfg.URL)
	}
	if cfg.Profile != DefaultProfile {
		t.Fatalf("Profile after defaults = %q, want %q", cfg.Profile, DefaultProfile)
	}
	if cfg.Interval == "" || cfg.Timeout == "" || cfg.Mode == "" {
		t.Fatalf("expected non-URL defaults to be filled: %+v", cfg)
	}
}

func TestUnmarshalMigratesLegacyConnectionToDefaultProfile(t *testing.T) {
	var cfg Config
	if err := json.Unmarshal([]byte(`{"url":"https://komari.example.com","api_key":"secret"}`), &cfg); err != nil {
		t.Fatal(err)
	}
	cfg = cfg.WithDefaults()

	if cfg.Profile != DefaultProfile {
		t.Fatalf("Profile = %q, want %q", cfg.Profile, DefaultProfile)
	}
	if cfg.URL != "https://komari.example.com" || cfg.APIKey != "secret" {
		t.Fatalf("effective connection = %q/%q", cfg.URL, cfg.APIKey)
	}
	if got := cfg.Profiles[DefaultProfile]; got.URL != cfg.URL || got.APIKey != cfg.APIKey {
		t.Fatalf("default profile = %+v, want migrated connection", got)
	}
}

func TestProfileAddUseAndRemove(t *testing.T) {
	cfg, err := AddProfile(Default(), "prod", "https://prod.example.com", "secret")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = UseProfile(cfg, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profile != "prod" || cfg.URL != "https://prod.example.com" || cfg.APIKey != "secret" {
		t.Fatalf("active profile = %q %q %q", cfg.Profile, cfg.URL, cfg.APIKey)
	}
	cfg, err = RemoveProfile(cfg, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profile == "prod" {
		t.Fatalf("removed profile is still active: %+v", cfg)
	}
	if _, ok := cfg.Profiles["prod"]; ok {
		t.Fatalf("removed profile remains in map: %+v", cfg.Profiles)
	}
}

func TestRenameProfile(t *testing.T) {
	cfg, err := AddProfile(Default(), "prod", "https://prod.example.com", "secret")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = UseProfile(cfg, "prod")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = RenameProfile(cfg, "prod", "primary")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profile != "primary" || cfg.URL != "https://prod.example.com" || cfg.APIKey != "secret" {
		t.Fatalf("renamed active profile = %q %q %q", cfg.Profile, cfg.URL, cfg.APIKey)
	}
	if _, ok := cfg.Profiles["prod"]; ok {
		t.Fatalf("old profile remains: %+v", cfg.Profiles)
	}
	if got := cfg.Profiles["primary"]; got.URL != "https://prod.example.com" || got.APIKey != "secret" {
		t.Fatalf("renamed profile = %+v", got)
	}
}

func TestRenameProfileRejectsExistingName(t *testing.T) {
	cfg, err := AddProfile(Default(), "prod", "https://prod.example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RenameProfile(cfg, "prod", DefaultProfile); err == nil {
		t.Fatal("expected rename to existing profile to fail")
	}
}

func TestSetURLAndAPIKeyUpdateActiveProfile(t *testing.T) {
	cfg, err := Set(Default(), "url", "https://komari.example.com")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = Set(cfg, "api-key", "secret")
	if err != nil {
		t.Fatal(err)
	}

	profile := cfg.Profiles[cfg.Profile]
	if profile.URL != "https://komari.example.com" || profile.APIKey != "secret" {
		t.Fatalf("active profile = %+v", profile)
	}
}

func TestRedactedMasksProfileAPIKeys(t *testing.T) {
	cfg, err := AddProfile(Default(), "prod", "https://prod.example.com", "secret")
	if err != nil {
		t.Fatal(err)
	}
	redacted := cfg.Redacted()
	if redacted.Profiles["prod"].APIKey != "********" {
		t.Fatalf("redacted API key = %q", redacted.Profiles["prod"].APIKey)
	}
}

func TestPathUsesUserConfigDir(t *testing.T) {
	t.Setenv("KTUI_CONFIG", "")
	oldUserConfigDir := userConfigDir
	t.Cleanup(func() { userConfigDir = oldUserConfigDir })
	userConfigDir = func() (string, error) {
		return filepath.Join("base", "config"), nil
	}

	got, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("base", "config", appName, "config.json")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestPathCanBeOverridden(t *testing.T) {
	want := filepath.Join("tmp", "custom.json")
	t.Setenv("KTUI_CONFIG", want)
	got, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestSetRealtimeWindow(t *testing.T) {
	cfg, err := Set(Default(), "realtime-window", "5m0s")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RealtimeWindow != "5m" {
		t.Fatalf("RealtimeWindow = %q, want 5m", cfg.RealtimeWindow)
	}
	duration, err := cfg.RealtimeWindowDuration()
	if err != nil {
		t.Fatal(err)
	}
	if duration != 5*time.Minute {
		t.Fatalf("RealtimeWindowDuration = %s, want 5m", duration)
	}
}

func TestSetRejectsRealtimePoints(t *testing.T) {
	if _, err := Set(Default(), "realtime-points", "150"); err == nil {
		t.Fatal("expected realtime-points to be rejected")
	}
}

func TestSetChartYAxis(t *testing.T) {
	cfg, err := Set(Default(), "chart-y-axis", "relative")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ChartYAxis != "relative" {
		t.Fatalf("ChartYAxis = %q, want relative", cfg.ChartYAxis)
	}
}

func TestDefaultWarningThresholds(t *testing.T) {
	cfg := Default()
	if cfg.WarnCPU != 90 || cfg.WarnRAM != 85 || cfg.WarnDisk != 90 || cfg.WarnExpiryDays != 7 {
		t.Fatalf("warning defaults = cpu %.1f ram %.1f disk %.1f expiry %d", cfg.WarnCPU, cfg.WarnRAM, cfg.WarnDisk, cfg.WarnExpiryDays)
	}
}

func TestSetWarningThresholds(t *testing.T) {
	cfg, err := Set(Default(), "warn-cpu", "85")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = Set(cfg, "warn-ram", "80%")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = Set(cfg, "warn-disk", "95")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = Set(cfg, "warn-expiry-days", "14")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WarnCPU != 85 || cfg.WarnRAM != 80 || cfg.WarnDisk != 95 || cfg.WarnExpiryDays != 14 {
		t.Fatalf("warning thresholds = %+v", cfg)
	}
}

func TestValidateRejectsInvalidRealtimeWindow(t *testing.T) {
	cfg := Default()
	cfg.RealtimeWindow = "2m"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid realtime_window to be rejected")
	}
}

func TestValidateRejectsInvalidWarningThreshold(t *testing.T) {
	cfg := Default()
	cfg.WarnCPU = 101
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid warn_cpu to be rejected")
	}
}

func TestValidateRejectsInvalidChartYAxis(t *testing.T) {
	cfg := Default()
	cfg.ChartYAxis = "fixed"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid chart_y_axis to be rejected")
	}
}
