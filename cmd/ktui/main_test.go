package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"ktui/internal/config"
)

func TestSplitConfigArgSeparateValue(t *testing.T) {
	args, path := splitConfigArg([]string{"--config", "/tmp/ktui.json", "config", "show"})
	if path != "/tmp/ktui.json" {
		t.Fatalf("path = %q", path)
	}
	want := []string{"config", "show"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %#v", args)
		}
	}
}

func TestSplitConfigArgEqualsValue(t *testing.T) {
	args, path := splitConfigArg([]string{"config", "show", "--config=/tmp/ktui.json"})
	if path != "/tmp/ktui.json" {
		t.Fatalf("path = %q", path)
	}
	want := []string{"config", "show"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %#v", args)
		}
	}
}

func TestLooksLikeCommand(t *testing.T) {
	if !looksLikeCommand("add") {
		t.Fatal("add should look like a command")
	}
	if looksLikeCommand("--sheet") {
		t.Fatal("--sheet should not look like a command")
	}
	if looksLikeCommand("") {
		t.Fatal("empty string should not look like a command")
	}
}

func TestFirstRunSetupSavesURLAndAPIKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("KTUI_CONFIG", path)
	var out bytes.Buffer

	cfg, err := firstRunSetup(applyEnvConfigForTest(), strings.NewReader("https://komari.example.com\nsecret\n"), &out)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != "https://komari.example.com" || cfg.APIKey != "secret" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if !strings.Contains(out.String(), "Saved config") {
		t.Fatalf("output = %q, want saved config message", out.String())
	}
}

func TestFirstRunSetupPreservesExistingAPIKeyWhenBlank(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("KTUI_CONFIG", path)
	cfg := config.Default()
	cfg.APIKey = "existing"

	got, err := firstRunSetup(cfg, strings.NewReader("https://komari.example.com\n\n"), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != "existing" {
		t.Fatalf("APIKey = %q, want existing", got.APIKey)
	}
}

func TestFirstRunSetupAllowsMissingOptionalAPIKeyAtEOF(t *testing.T) {
	t.Setenv("KTUI_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	got, err := firstRunSetup(config.Default(), strings.NewReader("https://komari.example.com\n"), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://komari.example.com" || got.APIKey != "" {
		t.Fatalf("cfg = %+v", got)
	}
}

func TestFirstRunSetupNormalizesURL(t *testing.T) {
	t.Setenv("KTUI_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	got, err := firstRunSetup(config.Default(), strings.NewReader("komari.example.com\n"), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://komari.example.com" {
		t.Fatalf("URL = %q, want normalized https URL", got.URL)
	}
}

func TestFirstRunSetupRequiresURL(t *testing.T) {
	t.Setenv("KTUI_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	_, err := firstRunSetup(applyEnvConfigForTest(), strings.NewReader("\n"), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected missing URL to fail")
	}
}

func TestFirstRunSetupRejectsUnsupportedURLScheme(t *testing.T) {
	t.Setenv("KTUI_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	_, err := firstRunSetup(config.Default(), strings.NewReader("ftp://komari.example.com\n"), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected unsupported scheme to fail")
	}
}

func applyEnvConfigForTest() config.Config {
	return config.Default()
}
