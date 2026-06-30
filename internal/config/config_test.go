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
