package tui

import (
	"strings"
	"testing"
	"time"

	"ktui/internal/komari"
)

func TestLineTableWideColumns(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true, NoColor: true})
	width := 230
	node := komari.Node{
		UUID:      "node-1",
		Name:      "server",
		Region:    "US",
		OS:        "debian",
		Group:     "prod",
		Tags:      "edge",
		MemTotal:  1000,
		DiskTotal: 2000,
		ExpiredAt: komari.NullTime{
			Time:  time.Now().Add(72 * time.Hour),
			Valid: true,
		},
	}
	st := komari.Status{
		Online:       true,
		CPU:          12.3,
		RAM:          500,
		RAMTotal:     1000,
		Disk:         700,
		DiskTotal:    2000,
		Load:         0.1,
		Load5:        0.2,
		Load15:       0.3,
		NetOut:       1024,
		NetIn:        2048,
		NetTotalUp:   1024 * 1024,
		NetTotalDown: 2 * 1024 * 1024,
		Uptime:       3600,
		Connections:  12,
		Process:      34,
	}

	header := app.lineTableColumns(width, false, komari.Node{}, komari.Status{}, false)
	for _, column := range []string{"LOAD", "TRAFFIC", "CONN", "PROC", "EXP", "OS", "TAG"} {
		if !strings.Contains(header, column) {
			t.Fatalf("header missing %s: %q", column, header)
		}
	}

	row := app.lineTableColumns(width, true, node, st, false)
	for _, value := range []string{"0.10 0.20 0.30", "debian", "prod edge"} {
		if !strings.Contains(row, value) {
			t.Fatalf("row missing %s: %q", value, row)
		}
	}
	if displayWidth(row) > width {
		t.Fatalf("row width = %d, want <= %d: %q", displayWidth(row), width, row)
	}
}
