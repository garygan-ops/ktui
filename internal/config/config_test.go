package config

import (
	"path/filepath"
	"testing"
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
	if cfg.Interval == "" || cfg.Timeout == "" || cfg.Mode == "" {
		t.Fatalf("expected non-URL defaults to be filled: %+v", cfg)
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

func TestSetRealtimePoints(t *testing.T) {
	cfg, err := Set(Default(), "realtime-points", "150")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RealtimePoints != 150 {
		t.Fatalf("RealtimePoints = %d, want 150", cfg.RealtimePoints)
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

func TestValidateRejectsNegativeRealtimePoints(t *testing.T) {
	cfg := Default()
	cfg.RealtimePoints = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected negative realtime_points to be rejected")
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
