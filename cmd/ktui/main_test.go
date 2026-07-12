package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if looksLikeCommand("--mode") {
		t.Fatal("--mode should not look like a command")
	}
	if looksLikeCommand("") {
		t.Fatal("empty string should not look like a command")
	}
}

func TestHelpTextUsesRefactoredCommands(t *testing.T) {
	for _, want := range []string{
		"ktui status [flags]",
		"ktui export <markdown|csv|json> [flags]",
		"ktui profile <list|current|use|add|rename|remove>",
		"ktui update <check|install>",
		"ktui completion <bash|zsh|fish|powershell>",
		"--profile NAME",
		"--mode MODE",
		"--realtime-window DURATION",
		"ktui update check",
		"ktui update install",
		"ktui completion bash",
	} {
		if !strings.Contains(helpText, want) {
			t.Fatalf("help text missing %q:\n%s", want, helpText)
		}
	}
	for _, old := range []string{
		"--once",
		"--line",
		"--sheet",
		"--realtime-points",
		"ktui update --check",
	} {
		if strings.Contains(helpText, old) {
			t.Fatalf("help text still contains old command %q:\n%s", old, helpText)
		}
	}
}

func TestParseModeFlag(t *testing.T) {
	for _, value := range []string{"sheet", " line "} {
		if _, err := parseModeFlag(value); err != nil {
			t.Fatalf("parseModeFlag(%q) error = %v", value, err)
		}
	}
	if _, err := parseModeFlag("grid"); err == nil {
		t.Fatal("expected invalid mode to fail")
	}
}

func TestValidRealtimeWindow(t *testing.T) {
	for _, value := range []time.Duration{time.Minute, 5 * time.Minute, 10 * time.Minute} {
		if !validRealtimeWindow(value) {
			t.Fatalf("validRealtimeWindow(%s) = false, want true", value)
		}
	}
	if validRealtimeWindow(2 * time.Minute) {
		t.Fatal("2m should not be a valid realtime window")
	}
}

func TestApplyEnvSelectsProfile(t *testing.T) {
	cfg, err := config.AddProfile(config.Default(), "prod", "https://prod.example.com", "secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("KTUI_PROFILE", "prod")

	got, err := applyEnv(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != "prod" || got.URL != "https://prod.example.com" || got.APIKey != "secret" {
		t.Fatalf("cfg = %+v", got)
	}
}

func TestHandleProfileAddUse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("KTUI_CONFIG", path)

	if err := handleProfile([]string{"add", "prod", "--url", "prod.example.com", "--api-key", "secret", "--use"}); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profile != "prod" || cfg.URL != "https://prod.example.com" || cfg.APIKey != "secret" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestHandleProfileRename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("KTUI_CONFIG", path)

	if err := handleProfile([]string{"add", "prod", "--url", "prod.example.com", "--use"}); err != nil {
		t.Fatal(err)
	}
	if err := handleProfile([]string{"rename", "prod", "primary"}); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profile != "primary" || cfg.URL != "https://prod.example.com" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if _, ok := cfg.Profiles["prod"]; ok {
		t.Fatalf("old profile remains: %+v", cfg.Profiles)
	}
}

func TestHandleUpdateRejectsOldFlagSyntax(t *testing.T) {
	err := handleUpdate([]string{"--check"})
	if err == nil || !strings.Contains(err.Error(), "unknown update command") {
		t.Fatalf("handleUpdate old syntax error = %v", err)
	}
}

func TestKeysHelpTextFormattingAndFooterActions(t *testing.T) {
	help := keysHelpText()
	if strings.Contains(help, "\t") {
		t.Fatalf("keys help should not contain tab indentation: %q", help)
	}
	for _, want := range []string{
		"Footer click       back/tabs/window/scroll/settings/refresh",
		"Footer click       back/previous/next/window/refresh",
		"About:",
		"?                  open about",
		"profile            switch active profile",
		"rename_profile     Enter edit",
		"site/url/api_key   shown as read-only",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("keys help missing %q:\n%s", want, help)
		}
	}
}

func TestCheckSystemClockRejectsLargeSkew(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 30, 0, 0, time.UTC)
	err := checkSystemClock(context.Background(), fakeServerTimeSource{time: now.Add(-time.Minute)}, func() time.Time {
		return now
	})
	if err == nil {
		t.Fatal("expected clock skew error")
	}
	if !strings.Contains(err.Error(), "Please correct your system time") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestCheckSystemClockAllowsSmallSkew(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 30, 0, 0, time.UTC)
	err := checkSystemClock(context.Background(), fakeServerTimeSource{time: now.Add(-10 * time.Second)}, func() time.Time {
		return now
	})
	if err != nil {
		t.Fatalf("clock check error = %v", err)
	}
}

func TestCheckSystemClockIgnoresUnavailableServerTime(t *testing.T) {
	err := checkSystemClock(context.Background(), fakeServerTimeSource{err: errors.New("missing date")}, time.Now)
	if err != nil {
		t.Fatalf("clock check error = %v", err)
	}
}

type fakeServerTimeSource struct {
	time time.Time
	err  error
}

func (f fakeServerTimeSource) ServerTime(context.Context) (time.Time, error) {
	return f.time, f.err
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
