package tui

import (
	"context"
	"strings"
	"testing"

	"ktui/internal/komari"
)

func TestAboutKeyOpensAndRendersAboutPage(t *testing.T) {
	keys := parseKeys([]byte("?"))
	if len(keys) != 1 || keys[0].name != "about" || keys[0].text != "?" {
		t.Fatalf("keys = %#v, want about key with text", keys)
	}

	app := NewWithOptions(nil, Options{
		URL:       "https://komari.example.com",
		APIKey:    "secret-token",
		Version:   "v1.2.3",
		Commit:    "abc123",
		BuildDate: "2026-07-06",
		ASCII:     true,
		NoColor:   true,
	})
	app.snapshot = komari.Snapshot{
		SourceURL:  "https://snapshot.example.com",
		Online:     1,
		Total:      2,
		Version:    komari.VersionInfo{Version: "1.0.0", Hash: "hash"},
		RPCVersion: "2.0",
		Public:     komari.PublicInfo{SiteName: "Lab"},
	}
	app.komariUpdate = komariUpdateState{
		Checked:      true,
		Available:    true,
		Current:      "1.0.0",
		Latest:       "1.1.0",
		ReleaseURL:   "https://github.com/komari-monitor/komari/releases/tag/v1.1.0",
		ReleaseCount: 1,
	}

	app.handleKey(context.Background(), keys[0])
	if !app.about {
		t.Fatal("about page was not opened")
	}

	lines := app.renderAboutBody(100, 60)
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"ktui",
		"v1.2.3",
		"abc123",
		"2026-07-06",
		"Komari",
		"Lab",
		"https://komari.example.com",
		"configured",
		"1/2 online",
		"Komari Update",
		"1.1.0",
		"https://github.com/komari-monitor/komari/releases/tag/v1.1.0",
		"ktui help keys",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("about body missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "secret-token") {
		t.Fatalf("about body leaked API key:\n%s", joined)
	}
}

func TestSearchCanTypeQuestionMark(t *testing.T) {
	app := NewWithOptions(nil, Options{})
	app.searchEditing = true

	app.handleKey(context.Background(), keyEvent{name: "about", text: "?"})
	if app.about {
		t.Fatal("about key event should be treated as text while search is editing")
	}
	if app.searchDraft != "?" {
		t.Fatalf("searchDraft = %q, want ?", app.searchDraft)
	}
}

func TestAboutBackRestoresPreviousLayer(t *testing.T) {
	app := NewWithOptions(nil, Options{})
	app.detail = true
	app.chartFocus = true

	app.handleKey(context.Background(), keyEvent{name: "about", text: "?"})
	if !app.about || app.detail || app.chartFocus {
		t.Fatalf("about=%t detail=%t chart=%t, want about only", app.about, app.detail, app.chartFocus)
	}

	app.handleAboutKey(keyEvent{name: "back"})
	if app.about || !app.detail || !app.chartFocus {
		t.Fatalf("about=%t detail=%t chart=%t, want detail chart restored", app.about, app.detail, app.chartFocus)
	}

	app.settings = true
	app.handleKey(context.Background(), keyEvent{name: "about", text: "?"})
	app.handleAboutKey(keyEvent{name: "quit"})
	if app.about || !app.settings {
		t.Fatalf("about=%t settings=%t, want settings restored", app.about, app.settings)
	}
}

func TestAboutScrollAndFooterActions(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true, NoColor: true})
	app.about = true

	app.handleAboutKey(keyEvent{name: "down"})
	if app.aboutScroll != 1 {
		t.Fatalf("aboutScroll = %d, want 1", app.aboutScroll)
	}
	app.handleAboutKey(keyEvent{name: "up"})
	if app.aboutScroll != 0 {
		t.Fatalf("aboutScroll = %d, want clamped 0", app.aboutScroll)
	}

	_, height := terminalSize()
	x, ok := footerActionPosition(app, footerRefresh)
	if !ok {
		t.Fatalf("missing refresh footer action in %q", app.footerText())
	}
	app.handleKey(context.Background(), keyEvent{name: "mouse-left", x: x, y: height})
	select {
	case <-app.refreshCh:
	default:
		t.Fatal("clicking about refresh footer action should request refresh")
	}
}
