package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"ktui/internal/komari"
)

func TestSettingsKeyOpensSettingsPage(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true})
	keys := parseKeys([]byte("s"))
	if len(keys) != 1 || keys[0].name != "settings" {
		t.Fatalf("keys = %#v, want settings", keys)
	}

	app.handleKey(context.Background(), keys[0])
	if !app.settings {
		t.Fatal("settings page was not opened")
	}

	lines := app.renderSettingsBody(80, 18)
	joined := strings.Join(lines, "\n")
	for _, label := range []string{"profile", "rename_profile", "site", "url", "api_key", "interval", "timeout", "mode", "realtime_window", "chart_y_axis", "ascii", "no_color", "warn_cpu", "warn_ram", "warn_disk", "warn_expiry_days", "about"} {
		if !strings.Contains(joined, label) {
			t.Fatalf("settings body missing %s: %#v", label, lines)
		}
	}
}

func TestSettingsItemsIncludeAllConfigFields(t *testing.T) {
	app := NewWithOptions(nil, Options{Profile: "default", URL: "https://komari.example.com", APIKey: "secret"})
	items := app.settingsItems()
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.Label)
	}
	want := []string{"profile", "rename_profile", "site", "url", "api_key", "interval", "timeout", "mode", "realtime_window", "chart_y_axis", "ascii", "no_color", "warn_cpu", "warn_ram", "warn_disk", "warn_expiry_days", "about"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("settings labels = %#v, want %#v", got, want)
	}
	if items[0].Value != "default" || items[3].Value != "https://komari.example.com" || items[4].Value != "********" {
		t.Fatalf("profile/URL/API display = %q/%q/%q", items[0].Value, items[3].Value, items[4].Value)
	}
	if !items[2].ReadOnly || !items[3].ReadOnly || !items[4].ReadOnly {
		t.Fatalf("site/URL/API should be marked read-only: %#v %#v %#v", items[2], items[3], items[4])
	}
}

func TestSettingsReadOnlyItemsRenderAffordance(t *testing.T) {
	app := NewWithOptions(nil, Options{
		URL:    "https://komari.example.com",
		APIKey: "secret",
		ASCII:  true,
	})

	lines := app.renderSettingsBody(100, 18)
	joined := strings.Join(lines, "\n")
	if strings.Count(joined, "read only") < 2 {
		t.Fatalf("settings body should mark URL and API key read-only: %#v", lines)
	}
}

func selectSetting(t *testing.T, app *App, label string) {
	t.Helper()
	for i, item := range app.settingsItems() {
		if item.Label == label {
			app.settingsSelected = i
			return
		}
	}
	t.Fatalf("missing setting %q", label)
}

func TestReadOnlySettingsDoNotPersist(t *testing.T) {
	called := false
	app := NewWithOptions(nil, Options{
		SaveSettings: func(settings PersistentSettings) error {
			called = true
			return nil
		},
	})
	selectSetting(t, app, "url")

	app.handleSettingsKey(keyEvent{name: "open"})
	if called {
		t.Fatal("read-only URL setting should not persist")
	}
	if app.settingsStatus != "read only" {
		t.Fatalf("settingsStatus = %q, want read only", app.settingsStatus)
	}
}

func TestSettingsAboutItemOpensAboutWithoutPersisting(t *testing.T) {
	called := false
	app := NewWithOptions(nil, Options{
		SaveSettings: func(settings PersistentSettings) error {
			called = true
			return nil
		},
	})
	app.settings = true
	selectSetting(t, app, "about")

	app.handleSettingsKey(keyEvent{name: "open"})
	if called {
		t.Fatal("about setting should not persist settings")
	}
	if !app.about || app.settings {
		t.Fatalf("about=%t settings=%t, want about open from settings", app.about, app.settings)
	}
	app.handleAboutKey(keyEvent{name: "back"})
	if app.about || !app.settings {
		t.Fatalf("about=%t settings=%t, want settings restored", app.about, app.settings)
	}
}

func TestSettingsAboutReturnPreservesDetailRestore(t *testing.T) {
	app := NewWithOptions(nil, Options{})
	app.detail = true
	app.handleKey(context.Background(), keyEvent{name: "settings"})
	selectSetting(t, app, "about")

	app.handleSettingsKey(keyEvent{name: "open"})
	app.handleAboutKey(keyEvent{name: "back"})
	app.handleSettingsKey(keyEvent{name: "back"})

	if app.about || app.settings || !app.detail {
		t.Fatalf("about=%t settings=%t detail=%t, want detail restored", app.about, app.settings, app.detail)
	}
}

func TestSettingsAdjustConfigFields(t *testing.T) {
	var saved PersistentSettings
	app := NewWithOptions(nil, Options{
		RefreshInterval: 5 * time.Second,
		FetchTimeout:    10 * time.Second,
		Mode:            ModeSheet,
		SaveSettings: func(settings PersistentSettings) error {
			saved = settings
			return nil
		},
	})

	selectSetting(t, app, "interval")
	app.handleSettingsKey(keyEvent{name: "tab-right"})
	if app.refreshInterval != 10*time.Second || saved.Interval != "10s" {
		t.Fatalf("interval = %s saved=%q, want 10s", app.refreshInterval, saved.Interval)
	}
	if !app.intervalChanged {
		t.Fatal("interval change should mark refresh ticker for reset")
	}

	selectSetting(t, app, "timeout")
	app.handleSettingsKey(keyEvent{name: "tab-right"})
	if app.fetchTimeout != 15*time.Second || app.detailTimeout != 15*time.Second || saved.Timeout != "15s" {
		t.Fatalf("timeout = fetch %s detail %s saved %q, want 15s", app.fetchTimeout, app.detailTimeout, saved.Timeout)
	}

	selectSetting(t, app, "mode")
	app.handleSettingsKey(keyEvent{name: "open"})
	if app.mode != ModeLine || saved.Mode != "line" {
		t.Fatalf("mode = %s saved=%q, want line", app.mode, saved.Mode)
	}

	selectSetting(t, app, "ascii")
	app.handleSettingsKey(keyEvent{name: "open"})
	if !app.style.ASCII || !saved.ASCII {
		t.Fatalf("ascii = %t saved=%t, want true", app.style.ASCII, saved.ASCII)
	}

	selectSetting(t, app, "no_color")
	app.handleSettingsKey(keyEvent{name: "open"})
	if !app.style.NoColor || !saved.NoColor {
		t.Fatalf("no_color = %t saved=%t, want true", app.style.NoColor, saved.NoColor)
	}

	selectSetting(t, app, "warn_cpu")
	app.handleSettingsKey(keyEvent{name: "tab-right"})
	if app.warnCPU != 95 || saved.WarnCPU != 95 {
		t.Fatalf("warn_cpu = %.1f saved=%.1f, want 95", app.warnCPU, saved.WarnCPU)
	}

	selectSetting(t, app, "warn_expiry_days")
	app.handleSettingsKey(keyEvent{name: "tab-right"})
	if app.warnExpiryDays != 14 || saved.WarnExpiryDays != 14 {
		t.Fatalf("warn_expiry_days = %d saved=%d, want 14", app.warnExpiryDays, saved.WarnExpiryDays)
	}
}

func TestSettingsBodyContainsCoreEditableItems(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true})
	lines := app.renderSettingsBody(80, 12)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "realtime_window") || !strings.Contains(joined, "chart_y_axis") {
		t.Fatalf("settings body missing items: %#v", lines)
	}
}

func TestSettingsBodyScrollsSelectedItemIntoView(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true, NoColor: true})
	selectSetting(t, app, "warn_expiry_days")

	lines := app.renderSettingsBody(80, 5)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "warn_expiry_days") {
		t.Fatalf("settings body did not scroll selected item into view: %#v", lines)
	}
	if app.settingsScroll == 0 {
		t.Fatal("settingsScroll should move when selected item is below the visible area")
	}
	if !strings.HasSuffix(lines[1], "^") {
		t.Fatalf("first visible settings row = %q, want up scroll indicator", lines[1])
	}
}

func TestSettingsClickUsesSettingsScrollOffset(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true, NoColor: true})
	selectSetting(t, app, "warn_expiry_days")
	app.renderSettingsBody(80, 5)

	scroll := app.settingsScroll
	app.selectSettingsAtBodyRow(app.settingsChromeRows(), 5)
	if app.settingsSelected != scroll {
		t.Fatalf("settingsSelected = %d, want first visible item index %d", app.settingsSelected, scroll)
	}
}

func TestSettingsBackRestoresDetailLayer(t *testing.T) {
	app := NewWithOptions(nil, Options{})
	app.detail = true
	app.detailScroll = 14

	app.handleKey(context.Background(), keyEvent{name: "settings"})
	if !app.settings || app.detail {
		t.Fatalf("settings=%t detail=%t, want settings open and detail hidden", app.settings, app.detail)
	}

	app.handleSettingsKey(keyEvent{name: "back"})
	if app.settings || !app.detail {
		t.Fatalf("settings=%t detail=%t, want settings closed and detail restored", app.settings, app.detail)
	}
	if app.detailScroll != 14 {
		t.Fatalf("detailScroll = %d, want preserved 14", app.detailScroll)
	}
}

func TestSettingsAdjustRealtimeWindowAndYAxisMode(t *testing.T) {
	app := NewWithOptions(nil, Options{RefreshInterval: 2 * time.Second})
	app.settings = true

	selectSetting(t, app, "realtime_window")
	app.handleSettingsKey(keyEvent{name: "tab-right"})
	if app.realtimeWindow != 5*time.Minute {
		t.Fatalf("realtimeWindow = %s, want 5m", app.realtimeWindow)
	}
	app.handleSettingsKey(keyEvent{name: "tab-left"})
	if app.realtimeWindow != time.Minute {
		t.Fatalf("realtimeWindow = %s, want 1m", app.realtimeWindow)
	}

	selectSetting(t, app, "chart_y_axis")
	app.handleSettingsKey(keyEvent{name: "open"})
	if app.chartYAxisMode != chartYAxisRelative {
		t.Fatalf("chartYAxisMode = %s, want relative", app.chartYAxisMode)
	}
	app.handleSettingsKey(keyEvent{name: "open"})
	if app.chartYAxisMode != chartYAxisAbsolute {
		t.Fatalf("chartYAxisMode = %s, want absolute", app.chartYAxisMode)
	}
}

func TestSettingsAdjustProfileSwitchesConnection(t *testing.T) {
	var saved PersistentSettings
	app := NewWithOptions(nil, Options{
		Profile: "default",
		URL:     "https://default.example.com",
		APIKey:  "default-key",
		Profiles: []ConnectionProfile{
			{Name: "default", URL: "https://default.example.com", APIKey: "default-key"},
			{Name: "lab", URL: "https://lab.example.com", APIKey: "lab-key"},
		},
		SaveSettings: func(settings PersistentSettings) error {
			saved = settings
			return nil
		},
	})
	app.settings = true
	app.snapshot = komari.Snapshot{
		Nodes:  []komari.Node{{UUID: "node-1", Name: "node"}},
		Public: komari.PublicInfo{SiteName: "Default"},
	}
	app.nodeDetail[detailKey{UUID: "node-1"}] = nodeDetail{UUID: "node-1"}
	app.realtimeStatus["node-1"] = []komari.Status{{CPU: 10}}
	selectSetting(t, app, "profile")

	app.handleSettingsKey(keyEvent{name: "tab-right"})

	if app.profileName != "lab" || app.settingsURL != "https://lab.example.com" || app.settingsAPIKey != "lab-key" {
		t.Fatalf("profile = %q url=%q api=%q", app.profileName, app.settingsURL, app.settingsAPIKey)
	}
	if app.client == nil || app.client.BaseURL() != "https://lab.example.com" {
		t.Fatalf("client URL = %v", app.client)
	}
	if saved.Profile != "lab" {
		t.Fatalf("saved Profile = %q, want lab", saved.Profile)
	}
	if len(app.snapshot.Nodes) != 0 || len(app.nodeDetail) != 0 || len(app.realtimeStatus) != 0 {
		t.Fatalf("old site state was not cleared: nodes=%d detail=%d realtime=%d", len(app.snapshot.Nodes), len(app.nodeDetail), len(app.realtimeStatus))
	}
	if !app.loading || app.connectionVersion != 1 {
		t.Fatalf("loading=%t connectionVersion=%d, want loading true version 1", app.loading, app.connectionVersion)
	}
	select {
	case <-app.refreshCh:
	default:
		t.Fatal("profile switch should request a refresh")
	}
}

func TestSettingsRenameProfile(t *testing.T) {
	var saved PersistentSettings
	app := NewWithOptions(nil, Options{
		Profile: "default",
		URL:     "https://default.example.com",
		Profiles: []ConnectionProfile{
			{Name: "default", URL: "https://default.example.com"},
			{Name: "lab", URL: "https://lab.example.com"},
		},
		SaveSettings: func(settings PersistentSettings) error {
			saved = settings
			return nil
		},
	})
	app.settings = true
	selectSetting(t, app, "rename_profile")

	app.handleSettingsKey(keyEvent{name: "open"})
	for range "default" {
		app.handleSettingsKey(keyEvent{name: "backspace"})
	}
	for _, r := range "primary" {
		app.handleSettingsKey(keyEvent{name: "char", text: string(r)})
	}
	app.handleSettingsKey(keyEvent{name: "open"})

	if app.profileName != "primary" {
		t.Fatalf("profileName = %q, want primary", app.profileName)
	}
	if app.profiles[0].Name != "primary" {
		t.Fatalf("profiles = %#v, want first profile renamed", app.profiles)
	}
	if saved.RenameProfileFrom != "default" || saved.Profile != "primary" {
		t.Fatalf("saved rename = %q -> %q", saved.RenameProfileFrom, saved.Profile)
	}
	if app.settingsRenamingProfile || app.settingsProfileDraft != "" {
		t.Fatalf("rename state not cleared: renaming=%t draft=%q", app.settingsRenamingProfile, app.settingsProfileDraft)
	}
}

func TestSettingsPersistAfterAdjustment(t *testing.T) {
	var saved PersistentSettings
	app := NewWithOptions(nil, Options{
		SaveSettings: func(settings PersistentSettings) error {
			saved = settings
			return nil
		},
	})
	app.settings = true
	selectSetting(t, app, "chart_y_axis")

	app.handleSettingsKey(keyEvent{name: "open"})
	if saved.ChartYAxisMode != "relative" {
		t.Fatalf("saved ChartYAxisMode = %q, want relative", saved.ChartYAxisMode)
	}
	if saved.RealtimeWindow != "1m" {
		t.Fatalf("saved RealtimeWindow = %q, want 1m", saved.RealtimeWindow)
	}
	if app.settingsStatus != "saved" {
		t.Fatalf("settingsStatus = %q, want saved", app.settingsStatus)
	}
}

func TestSettingsASCIIShortcutPersists(t *testing.T) {
	var saved PersistentSettings
	app := NewWithOptions(nil, Options{
		SaveSettings: func(settings PersistentSettings) error {
			saved = settings
			return nil
		},
	})
	app.settings = true

	app.handleSettingsKey(keyEvent{name: "ascii"})
	if !app.style.ASCII || !saved.ASCII {
		t.Fatalf("ascii = %t saved=%t, want true", app.style.ASCII, saved.ASCII)
	}
	if app.settingsStatus != "saved" {
		t.Fatalf("settingsStatus = %q, want saved", app.settingsStatus)
	}
}

func TestNewWithOptionsUsesChartYAxisMode(t *testing.T) {
	app := NewWithOptions(nil, Options{ChartYAxisMode: "relative"})
	if app.chartYAxisMode != chartYAxisRelative {
		t.Fatalf("chartYAxisMode = %s, want relative", app.chartYAxisMode)
	}
}

func TestRelativePercentYAxisDisablesFixedRange(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true})
	app.chartYAxisMode = chartYAxisRelative
	node := komari.Node{UUID: "node-1", Name: "node", MemTotal: 1000, DiskTotal: 2000}
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	records := []komari.Status{
		{CPU: 10, RAM: 100, RAMTotal: 1000, Disk: 300, DiskTotal: 2000, Time: komari.NullTime{Time: now, Valid: true}},
	}

	sections := app.historyMetricSections(node, records[0], records, "Realtime")
	cpu := sectionByTitle(sections, "CPU Chart")
	if cpu == nil || cpu.Chart == nil {
		t.Fatalf("missing CPU chart: %#v", sections)
	}
	if cpu.Chart.FixedRange {
		t.Fatal("relative percent Y axis should not use fixed range")
	}
}
