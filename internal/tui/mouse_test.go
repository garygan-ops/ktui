package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"ktui/internal/komari"
)

func TestParseSGRMouseWheelAndClick(t *testing.T) {
	keys := parseKeys([]byte("\x1b[<64;10;5M\x1b[<65;10;6M\x1b[<0;12;7M\x1b[<0;12;7m"))
	if len(keys) != 4 {
		t.Fatalf("keys len = %d, want 4: %#v", len(keys), keys)
	}
	want := []string{"mouse-wheel-up", "mouse-wheel-down", "mouse-left", "mouse-ignore"}
	for i := range want {
		if keys[i].name != want[i] {
			t.Fatalf("key %d = %s, want %s", i, keys[i].name, want[i])
		}
	}
	if keys[2].x != 12 || keys[2].y != 7 {
		t.Fatalf("click position = %d,%d; want 12,7", keys[2].x, keys[2].y)
	}
}

func TestParseSGRMouseSplitSequence(t *testing.T) {
	keys, rest := parseKeysWithRemainder([]byte("\x1b[<0;12"))
	if len(keys) != 0 {
		t.Fatalf("keys = %#v, want none", keys)
	}
	if string(rest) != "\x1b[<0;12" {
		t.Fatalf("rest = %q", rest)
	}

	keys, rest = parseKeysWithRemainder(append(rest, []byte(";7M")...))
	if len(rest) != 0 {
		t.Fatalf("rest = %q, want empty", rest)
	}
	if len(keys) != 1 || keys[0].name != "mouse-left" || keys[0].x != 12 || keys[0].y != 7 {
		t.Fatalf("keys = %#v, want mouse-left at 12,7", keys)
	}
}

func TestMouseClickLineOpensNodeDetail(t *testing.T) {
	app := NewWithOptions(nil, Options{Mode: ModeLine})
	app.snapshot = komari.Snapshot{
		Nodes: []komari.Node{
			{UUID: "n1", Name: "one"},
			{UUID: "n2", Name: "two"},
			{UUID: "n3", Name: "three"},
		},
		Status: map[string]komari.Status{},
	}

	app.handleKey(context.Background(), keyEvent{name: "mouse-left", x: 2, y: mouseHeaderRows + lineHeaderRows + 2})
	if app.selected != 1 {
		t.Fatalf("selected = %d, want 1", app.selected)
	}
	if !app.detail {
		t.Fatal("clicking a node should open detail view")
	}
	if app.scroll != 0 {
		t.Fatalf("scroll = %d, want 0", app.scroll)
	}
}

func TestMouseClickLineWithSearchBarOpensCorrectNode(t *testing.T) {
	app := NewWithOptions(nil, Options{Mode: ModeLine})
	app.searchQuery = "o"
	app.snapshot = komari.Snapshot{
		Nodes: []komari.Node{
			{UUID: "n1", Name: "one"},
			{UUID: "n2", Name: "two"},
		},
		Status: map[string]komari.Status{},
	}

	app.handleKey(context.Background(), keyEvent{name: "mouse-left", x: 2, y: mouseHeaderRows + app.listSearchRows() + lineHeaderRows + 2})
	if app.selected != 1 {
		t.Fatalf("selected = %d, want 1", app.selected)
	}
	if !app.detail {
		t.Fatal("clicking a node should open detail view")
	}
}

func TestMouseClickDetailFooterBackReturnsToList(t *testing.T) {
	app := NewWithOptions(nil, Options{})
	app.detail = true
	app.scroll = 12
	_, height := terminalSize()
	x, _, ok := footerLabelBounds(app.footerText(), "Back")
	if !ok {
		t.Fatal("missing Back footer label")
	}

	app.handleKey(context.Background(), keyEvent{name: "mouse-left", x: x, y: height})
	if app.detail {
		t.Fatal("clicking detail footer back area should return to list")
	}
	if app.scroll != 0 {
		t.Fatalf("scroll = %d, want 0", app.scroll)
	}
}

func TestMouseClickDetailChartOpensFocus(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true})
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	node := komari.Node{UUID: "n1", Name: "one", MemTotal: 1000, DiskTotal: 2000}
	app.snapshot = komari.Snapshot{
		Nodes:  []komari.Node{node},
		Status: map[string]komari.Status{node.UUID: {CPU: 30, Time: komari.NullTime{Time: now, Valid: true}}},
	}
	app.detail = true
	app.tab = 2

	app.handleKey(context.Background(), keyEvent{name: "mouse-left", x: 2, y: mouseHeaderRows + 5})
	if !app.chartFocus {
		t.Fatal("clicking chart card should open chart focus")
	}
}

func TestMouseClickChartFocusFooterActions(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true})
	node := komari.Node{UUID: "n1", Name: "one", MemTotal: 1000, DiskTotal: 2000}
	app.snapshot = komari.Snapshot{
		Nodes:  []komari.Node{node},
		Status: map[string]komari.Status{node.UUID: {CPU: 30}},
	}
	app.detail = true
	app.tab = 2
	app.focusChart(0)
	_, height := terminalSize()

	x, _, ok := footerLabelBounds(app.footerText(), "l next")
	if !ok {
		t.Fatal("missing next footer label")
	}
	app.handleKey(context.Background(), keyEvent{name: "mouse-left", x: x, y: height})
	if app.chartFocusIndex != 1 {
		t.Fatalf("chartFocusIndex = %d, want 1", app.chartFocusIndex)
	}

	x, _, ok = footerLabelBounds(app.footerText(), "Esc/q back")
	if !ok {
		t.Fatal("missing focus back footer label")
	}
	app.handleKey(context.Background(), keyEvent{name: "mouse-left", x: x, y: height})
	if app.chartFocus {
		t.Fatal("clicking chart focus back should close focus")
	}
}

func TestMouseClickListFooterActions(t *testing.T) {
	app := NewWithOptions(nil, Options{})
	_, height := terminalSize()

	clickFooterAction := func(action footerAction) {
		t.Helper()
		x, ok := footerActionPosition(app, action)
		if !ok {
			t.Fatalf("missing footer action %q in %q", action, app.footerText())
		}
		app.handleKey(context.Background(), keyEvent{name: "mouse-left", x: x, y: height})
	}

	clickFooterAction(footerSettings)
	if !app.settings {
		t.Fatal("clicking settings footer action should open settings")
	}
	app.closeSettings()

	clickFooterAction(footerMode)
	if app.mode != ModeLine {
		t.Fatalf("mode = %s, want line", app.mode)
	}

	clickFooterAction(footerRefresh)
	select {
	case <-app.refreshCh:
	default:
		t.Fatal("clicking refresh footer action should request refresh")
	}

	clickFooterAction(footerASCII)
	if !app.style.ASCII {
		t.Fatal("clicking ascii footer action should toggle ASCII mode")
	}

	clickFooterAction(footerQuit)
	if !app.quit {
		t.Fatal("clicking quit footer action should set quit")
	}
}

func footerActionPosition(app *App, action footerAction) (int, bool) {
	width, _ := terminalSize()
	drawWidth := width
	if drawWidth > 1 {
		drawWidth--
	}
	variant := app.footerVariantForWidth(drawWidth)
	pos := 2
	for _, item := range app.footerItems() {
		label := item.Labels[variant]
		end := pos + displayWidth(label) - 1
		if item.Action == action {
			return pos, true
		}
		pos = end + 1
		if variant == 0 {
			pos += 3
		} else if variant == 1 {
			pos += 2
		} else {
			pos++
		}
	}
	return 0, false
}

func TestFooterFoldsInsteadOfClippingActions(t *testing.T) {
	app := NewWithOptions(nil, Options{})
	app.update.Available = true
	footer := app.footerTextForWidth(24)
	if displayWidth(footer) > 24 {
		t.Fatalf("footer width = %d, want <= 24: %q", displayWidth(footer), footer)
	}
	for _, label := range []string{"J", "O", "/", "C", "V", "S", "M", "R", "A", "Q", "U"} {
		if !strings.Contains(footer, label) {
			t.Fatalf("folded footer %q missing %q", footer, label)
		}
	}

	items := app.footerItems()
	variant := app.footerVariantForWidth(24)
	pos := 2
	for _, item := range items {
		label := item.Labels[variant]
		if action := app.footerActionAt(pos, 24); action != item.Action {
			t.Fatalf("click on %q resolved to %q, want %q in footer %q", label, action, item.Action, footer)
		}
		pos += displayWidth(label) + 1
	}
}

func TestFooterFoldsDetailAndSettingsActions(t *testing.T) {
	app := NewWithOptions(nil, Options{})
	app.detail = true
	app.update.Available = true
	detailFooter := app.footerTextForWidth(20)
	if displayWidth(detailFooter) > 20 {
		t.Fatalf("detail footer width = %d, want <= 20: %q", displayWidth(detailFooter), detailFooter)
	}
	for _, label := range []string{"B", "T", "W", "J", "S", "R", "U"} {
		if !strings.Contains(detailFooter, label) {
			t.Fatalf("folded detail footer %q missing %q", detailFooter, label)
		}
	}

	app.detail = false
	app.settings = true
	settingsFooter := app.footerTextForWidth(20)
	if displayWidth(settingsFooter) > 20 {
		t.Fatalf("settings footer width = %d, want <= 20: %q", displayWidth(settingsFooter), settingsFooter)
	}
	for _, label := range []string{"B", "Sel", "Adj", "Tog"} {
		if !strings.Contains(settingsFooter, label) {
			t.Fatalf("folded settings footer %q missing %q", settingsFooter, label)
		}
	}
}

func TestMouseClickDetailFooterSettingsAndRefresh(t *testing.T) {
	app := NewWithOptions(nil, Options{})
	app.detail = true
	app.snapshot = komari.Snapshot{
		Nodes:  []komari.Node{{UUID: "n1", Name: "one"}},
		Status: map[string]komari.Status{},
	}
	_, height := terminalSize()

	x, _, ok := footerLabelBounds(app.footerText(), "r refresh")
	if !ok {
		t.Fatal("missing refresh footer label")
	}
	app.handleKey(context.Background(), keyEvent{name: "mouse-left", x: x, y: height})
	select {
	case <-app.refreshCh:
	default:
		t.Fatal("clicking detail refresh footer action should request refresh")
	}

	x, _, ok = footerLabelBounds(app.footerText(), "s settings")
	if !ok {
		t.Fatal("missing settings footer label")
	}
	app.handleKey(context.Background(), keyEvent{name: "mouse-left", x: x, y: height})
	if !app.settings || app.detail || !app.settingsWasDetail {
		t.Fatalf("settings=%t detail=%t settingsWasDetail=%t, want settings opened from detail", app.settings, app.detail, app.settingsWasDetail)
	}
}

func TestMouseClickSettingsFooterActions(t *testing.T) {
	app := NewWithOptions(nil, Options{Mode: ModeSheet})
	app.settings = true
	selectSetting(t, app, "mode")
	_, height := terminalSize()

	x, _, ok := footerLabelBounds(app.footerText(), "Enter toggle")
	if !ok {
		t.Fatal("missing Enter toggle footer label")
	}
	app.handleKey(context.Background(), keyEvent{name: "mouse-left", x: x, y: height})
	if app.mode != ModeLine {
		t.Fatalf("mode = %s, want line after settings footer toggle", app.mode)
	}

	x, _, ok = footerLabelBounds(app.footerText(), "Esc/q back")
	if !ok {
		t.Fatal("missing back footer label")
	}
	app.handleKey(context.Background(), keyEvent{name: "mouse-left", x: x, y: height})
	if app.settings {
		t.Fatal("clicking settings back footer action should close settings")
	}
}

func TestMouseWheelSettingsSelection(t *testing.T) {
	app := NewWithOptions(nil, Options{})
	app.settings = true

	app.handleKey(context.Background(), keyEvent{name: "mouse-wheel-down", x: 1, y: 1})
	if app.settingsSelected != 1 {
		t.Fatalf("settingsSelected = %d, want 1", app.settingsSelected)
	}
	app.handleKey(context.Background(), keyEvent{name: "mouse-wheel-up", x: 1, y: 1})
	if app.settingsSelected != 0 {
		t.Fatalf("settingsSelected = %d, want 0", app.settingsSelected)
	}
}

func TestSheetLayoutMinimumWidth(t *testing.T) {
	columns, cardWidth := sheetLayout(100)
	if columns != 2 || cardWidth != 49 {
		t.Fatalf("layout = %d columns, width %d; want 2,49", columns, cardWidth)
	}
}
