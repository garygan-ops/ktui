package tui

import (
	"strings"
	"testing"
)

func TestWriteFrameClearsAndWritesAllRowsOnFirstFrame(t *testing.T) {
	app := NewWithOptions(nil, Options{})
	var b strings.Builder

	app.writeFrame(&b, 12, 3, []string{"one", "two"})

	out := b.String()
	if !strings.Contains(out, "\x1b[2J") {
		t.Fatal("first frame should clear the screen")
	}
	for _, cursor := range []string{"\x1b[1;1H", "\x1b[2;1H", "\x1b[3;1H"} {
		if !strings.Contains(out, cursor) {
			t.Fatalf("first frame missing cursor move %q in %q", cursor, out)
		}
	}
}

func TestWriteFrameOnlyWritesChangedRowsOnSameSize(t *testing.T) {
	app := NewWithOptions(nil, Options{})
	var first strings.Builder
	app.writeFrame(&first, 12, 3, []string{"one", "two", "three"})

	var second strings.Builder
	app.writeFrame(&second, 12, 3, []string{"one", "TWO", "three"})

	out := second.String()
	if strings.Contains(out, "\x1b[2J") {
		t.Fatalf("unchanged size should not clear screen: %q", out)
	}
	if strings.Contains(out, "\x1b[1;1H") || strings.Contains(out, "\x1b[3;1H") {
		t.Fatalf("unchanged rows should not be rewritten: %q", out)
	}
	if !strings.Contains(out, "\x1b[2;1H") || !strings.Contains(out, "TWO") {
		t.Fatalf("changed row should be rewritten: %q", out)
	}
}

func TestWriteFrameClearsAgainAfterResize(t *testing.T) {
	app := NewWithOptions(nil, Options{})
	var first strings.Builder
	app.writeFrame(&first, 12, 3, []string{"one", "two", "three"})

	var second strings.Builder
	app.writeFrame(&second, 14, 3, []string{"one", "two", "three"})

	out := second.String()
	if !strings.Contains(out, "\x1b[2J") {
		t.Fatalf("resize should force a full clear: %q", out)
	}
}
