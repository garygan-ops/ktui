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
