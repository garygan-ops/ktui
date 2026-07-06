package tui

import (
	"strings"
	"testing"
	"time"

	"ktui/internal/komari"
)

func TestCleanLineANSIPreservesReset(t *testing.T) {
	line := "\x1b[31mhello world\x1b[0m"
	got := cleanLine(line, 8)
	if visibleWidth(got) > 8 {
		t.Fatalf("visible width = %d, want <= 8: %q", visibleWidth(got), got)
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("truncated ANSI line does not reset style: %q", got)
	}
}

func TestFitLinePadsToWidth(t *testing.T) {
	got := fitLine("abc", 8)
	if visibleWidth(got) != 8 {
		t.Fatalf("visible width = %d, want 8: %q", visibleWidth(got), got)
	}
}

func TestFitLineTruncatesWideText(t *testing.T) {
	got := fitLine("abcdef", 4)
	if visibleWidth(got) != 4 {
		t.Fatalf("visible width = %d, want 4: %q", visibleWidth(got), got)
	}
}

func TestStyleWrapReappliesAfterNestedReset(t *testing.T) {
	style := Style{}
	got := style.dim("left " + style.green("ok") + " right")
	if !strings.Contains(got, "\x1b[0m\x1b[2m right") {
		t.Fatalf("outer style was not restored after nested reset: %q", got)
	}
}

func TestBoxLineCanStyleBordersIndependently(t *testing.T) {
	style := Style{}
	got := style.boxLineWithBorder(style.green("ok"), 8, style.dim)
	border := style.dim("│")
	if !strings.HasPrefix(got, border) || !strings.HasSuffix(got, border) {
		t.Fatalf("box borders are not independently styled: %q", got)
	}
	if visibleWidth(got) != 8 {
		t.Fatalf("visible width = %d, want 8: %q", visibleWidth(got), got)
	}
}

func TestBoxLineCanStyleASCIIBordersIndependently(t *testing.T) {
	style := Style{ASCII: true}
	got := style.boxLineWithBorder(style.green("ok"), 8, style.dim)
	border := style.dim("|")
	if !strings.HasPrefix(got, border) || !strings.HasSuffix(got, border) {
		t.Fatalf("ASCII box borders are not independently styled: %q", got)
	}
	if strings.Contains(got, "│") {
		t.Fatalf("ASCII box line should not use UTF-8 border glyphs: %q", got)
	}
	if visibleWidth(got) != 8 {
		t.Fatalf("visible width = %d, want 8: %q", visibleWidth(got), got)
	}
}

func TestBoxLineWidthOneUsesCurrentModeGlyph(t *testing.T) {
	for _, tc := range []struct {
		name  string
		style Style
		want  string
	}{
		{name: "utf8", style: Style{}, want: "│"},
		{name: "ascii", style: Style{ASCII: true}, want: "|"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.style.boxLineWithBorder("ignored", 1, tc.style.dim)
			if got != tc.style.dim(tc.want) {
				t.Fatalf("boxLineWithBorder width 1 = %q, want %q", got, tc.style.dim(tc.want))
			}
		})
	}
}

func TestStyleNoColorSuppressesANSI(t *testing.T) {
	style := Style{NoColor: true}
	got := style.dim("left " + style.green("ok") + " right")
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("NoColor style emitted ANSI: %q", got)
	}
}

func TestExpiryText(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		node komari.Node
		want string
	}{
		{name: "unknown", node: komari.Node{}, want: "-"},
		{name: "free", node: komari.Node{Price: -1, ExpiredAt: komari.NullTime{Time: now.Add(200 * 365 * 24 * time.Hour), Valid: true}}, want: "free"},
		{name: "lifetime", node: komari.Node{ExpiredAt: komari.NullTime{Time: now.Add(200 * 365 * 24 * time.Hour), Valid: true}}, want: "lifetime"},
		{name: "expired", node: komari.Node{ExpiredAt: komari.NullTime{Time: now.Add(-time.Hour), Valid: true}}, want: "expired"},
		{name: "today", node: komari.Node{ExpiredAt: komari.NullTime{Time: now.Add(2 * time.Hour), Valid: true}}, want: "today"},
		{name: "days", node: komari.Node{ExpiredAt: komari.NullTime{Time: now.Add(49 * time.Hour), Valid: true}}, want: "3d"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := expiryText(tc.node, now); got != tc.want {
				t.Fatalf("expiryText = %q, want %q", got, tc.want)
			}
		})
	}
}
