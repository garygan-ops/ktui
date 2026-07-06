package tui

import (
	"strings"
	"testing"

	"ktui/internal/komari"
)

func TestSheetBodyReservesColumnForScrollIndicator(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true, NoColor: true, Mode: ModeSheet})
	app.snapshot = komari.Snapshot{
		Nodes: []komari.Node{
			{UUID: "n1", Name: "one"},
			{UUID: "n2", Name: "two"},
			{UUID: "n3", Name: "three"},
		},
		Status: map[string]komari.Status{},
	}

	lines := app.renderSheetBody(82, sheetCardHeight)
	if len(lines) == 0 {
		t.Fatal("renderSheetBody returned no lines")
	}
	top := stripANSI(lines[0])
	if displayWidth(top) != 82 {
		t.Fatalf("top line width = %d, want 82: %q", displayWidth(top), top)
	}
	if !strings.HasSuffix(top, "#") {
		t.Fatalf("top line = %q, want scroll thumb in final column", top)
	}
	if top[len(top)-2] != '+' {
		t.Fatalf("top line = %q, want card border before scroll indicator", top)
	}

	bottom := stripANSI(lines[sheetCardHeight-1])
	if !strings.HasSuffix(bottom, "v") {
		t.Fatalf("bottom line = %q, want down scroll indicator in final column", bottom)
	}
	if bottom[len(bottom)-2] != '+' {
		t.Fatalf("bottom line = %q, want card border before scroll indicator", bottom)
	}
}

func TestCardBoxLineUsesModeSpecificStyledBorders(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ascii bool
		glyph string
	}{
		{name: "utf8", glyph: "│"},
		{name: "ascii", ascii: true, glyph: "|"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := NewWithOptions(nil, Options{ASCII: tc.ascii})
			line := app.cardBoxLine(app.style.green("ok"), 8, false, nodeAlert{})
			border := app.style.dim(tc.glyph)
			if !strings.HasPrefix(line, border) || !strings.HasSuffix(line, border) {
				t.Fatalf("card body borders = %q, want dim %q borders", line, tc.glyph)
			}
			if displayWidth(line) != 8 {
				t.Fatalf("visible width = %d, want 8: %q", displayWidth(line), line)
			}
		})
	}
}

func TestCardBoxLineBorderStyleMatchesCardState(t *testing.T) {
	for _, mode := range []struct {
		name  string
		ascii bool
		glyph string
	}{
		{name: "utf8", glyph: "│"},
		{name: "ascii", ascii: true, glyph: "|"},
	} {
		mode := mode
		t.Run(mode.name, func(t *testing.T) {
			for _, tc := range []struct {
				name     string
				selected bool
				alert    nodeAlert
				border   func(Style, string) string
			}{
				{name: "default", border: func(s Style, value string) string { return s.dim(value) }},
				{name: "selected", selected: true, border: func(s Style, value string) string { return s.cyan(value) }},
				{name: "critical", alert: nodeAlert{Critical: true}, border: func(s Style, value string) string { return s.red(value) }},
				{name: "warning", alert: nodeAlert{Warning: true}, border: func(s Style, value string) string { return s.yellow(value) }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					app := NewWithOptions(nil, Options{ASCII: mode.ascii})
					line := app.cardBoxLine(app.style.green("ok"), 8, tc.selected, tc.alert)
					want := tc.border(app.style, mode.glyph)
					if !strings.HasPrefix(line, want) || !strings.HasSuffix(line, want) {
						t.Fatalf("%s %s card body borders = %q, want %q", mode.name, tc.name, line, want)
					}
				})
			}
		})
	}
}

func TestNodeCardBorderStylesMatchModeAndState(t *testing.T) {
	for _, mode := range []struct {
		name   string
		ascii  bool
		top    string
		side   string
		bottom string
	}{
		{name: "utf8", top: "┌", side: "│", bottom: "└"},
		{name: "ascii", ascii: true, top: "+", side: "|", bottom: "+"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			app := NewWithOptions(nil, Options{ASCII: mode.ascii})
			node := komari.Node{UUID: "n1", Name: "one", MemTotal: 1000, DiskTotal: 1000}
			app.snapshot = komari.Snapshot{
				Nodes:  []komari.Node{node},
				Status: map[string]komari.Status{node.UUID: {Online: true}},
			}

			lines := app.nodeCard(1, node, 12, 5)
			if !strings.HasPrefix(lines[0], "\x1b[2m"+mode.top) {
				t.Fatalf("top border = %q, want dim %q", lines[0], mode.top)
			}
			if !strings.HasPrefix(lines[1], app.style.dim(mode.side)) || !strings.HasSuffix(lines[1], app.style.dim(mode.side)) {
				t.Fatalf("body border = %q, want dim %q", lines[1], mode.side)
			}
			if !strings.HasPrefix(lines[4], "\x1b[2m"+mode.bottom) {
				t.Fatalf("bottom border = %q, want dim %q", lines[4], mode.bottom)
			}

			app.selected = 0
			lines = app.nodeCard(0, node, 12, 5)
			if !strings.HasPrefix(lines[0], "\x1b[36m"+mode.top) {
				t.Fatalf("selected top border = %q, want cyan %q", lines[0], mode.top)
			}
			if !strings.HasPrefix(lines[1], app.style.cyan(mode.side)) || !strings.HasSuffix(lines[1], app.style.cyan(mode.side)) {
				t.Fatalf("selected body border = %q, want cyan %q", lines[1], mode.side)
			}
		})
	}
}
