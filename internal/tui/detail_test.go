package tui

import (
	"strings"
	"testing"
	"time"

	"ktui/internal/komari"
)

func TestRealtimeHistorySectionsIncludeCharts(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true})
	node := komari.Node{UUID: "node-1", Name: "node", MemTotal: 1000, DiskTotal: 2000}
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	current := komari.Status{
		CPU:         35,
		RAM:         500,
		RAMTotal:    1000,
		Disk:        700,
		DiskTotal:   2000,
		NetOut:      300,
		NetIn:       100,
		Connections: 11,
		Process:     80,
		Time:        komari.NullTime{Time: now, Valid: true},
	}
	app.window = 0
	app.nodeDetail[detailKey{UUID: node.UUID, Window: app.window}] = nodeDetail{
		Recent: komari.RecentStatusResp{Records: []komari.Status{
			{CPU: 10, RAM: 100, RAMTotal: 1000, Disk: 300, DiskTotal: 2000, NetOut: 100, NetIn: 50, Connections: 5, Process: 40, Time: komari.NullTime{Time: now.Add(-2 * time.Minute), Valid: true}},
			{CPU: 20, RAM: 300, RAMTotal: 1000, Disk: 500, DiskTotal: 2000, NetOut: 200, NetIn: 80, Connections: 8, Process: 60, Time: komari.NullTime{Time: now.Add(-1 * time.Minute), Valid: true}},
		}},
	}

	app.tab = 2
	sections := app.historySections(node, current)
	if !hasSection(sections, "CPU Chart") {
		t.Fatalf("missing CPU Chart section: %#v", sectionTitles(sections))
	}
	if !hasSection(sections, "RAM Chart") {
		t.Fatalf("missing RAM Chart section: %#v", sectionTitles(sections))
	}
	if !hasSection(sections, "Disk Chart") {
		t.Fatalf("missing Disk Chart section: %#v", sectionTitles(sections))
	}
	if !hasSection(sections, "Net Out Chart") {
		t.Fatalf("missing Net Out Chart section: %#v", sectionTitles(sections))
	}
	if !hasSection(sections, "Net In Chart") {
		t.Fatalf("missing Net In Chart section: %#v", sectionTitles(sections))
	}
	if !hasSection(sections, "Connections Chart") {
		t.Fatalf("missing Connections Chart section: %#v", sectionTitles(sections))
	}
	if !hasSection(sections, "Process Chart") {
		t.Fatalf("missing Process Chart section: %#v", sectionTitles(sections))
	}
}

func TestHistorySectionsIncludeAllMetricCharts(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true})
	node := komari.Node{UUID: "node-1", Name: "node", MemTotal: 1000, DiskTotal: 2000}
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	current := komari.Status{CPU: 35, RAM: 500, RAMTotal: 1000, Disk: 700, DiskTotal: 2000, NetOut: 300, NetIn: 100, Connections: 11, Process: 80, Time: komari.NullTime{Time: now, Valid: true}}
	app.window = 1
	app.nodeDetail[detailKey{UUID: node.UUID, Window: app.window}] = nodeDetail{
		Load: komari.LoadRecordsResp{Records: []komari.Status{
			{CPU: 10, RAM: 100, RAMTotal: 1000, Disk: 300, DiskTotal: 2000, NetOut: 100, NetIn: 50, Connections: 5, Process: 40, Time: komari.NullTime{Time: now.Add(-2 * time.Hour), Valid: true}},
			{CPU: 20, RAM: 300, RAMTotal: 1000, Disk: 500, DiskTotal: 2000, NetOut: 200, NetIn: 80, Connections: 8, Process: 60, Time: komari.NullTime{Time: now.Add(-1 * time.Hour), Valid: true}},
			current,
		}},
	}

	app.tab = 2
	sections := app.historySections(node, current)
	if len(sections) == 0 || sections[0].Title != "CPU Chart" {
		t.Fatalf("first history section = %#v, want CPU Chart first", sectionTitles(sections))
	}
	for _, title := range []string{"CPU Chart", "RAM Chart", "Disk Chart", "Net Out Chart", "Net In Chart", "Connections Chart", "Process Chart"} {
		if !hasSection(sections, title) {
			t.Fatalf("missing %s section: %#v", title, sectionTitles(sections))
		}
	}
}

func TestAxisChartLinesIncludeYAxisAndTimeAxis(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true})
	lines := app.axisChartLines(axisChart{
		Values: []float64{10, 20, 5, 30},
		From:   "06-30 10:00",
		To:     "06-30 11:00",
		Unit:   "%",
	}, 42, 4)

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "|") {
		t.Fatalf("missing y axis: %#v", lines)
	}
	if !strings.Contains(joined, "06-30 10:00") || !strings.Contains(joined, "06-30 11:00") {
		t.Fatalf("missing time axis labels: %#v", lines)
	}
}

func TestDetailScrollStepMatchesCardHeight(t *testing.T) {
	if detailScrollStep() != detailCardHeight {
		t.Fatalf("detailScrollStep = %d, want %d", detailScrollStep(), detailCardHeight)
	}
}

func TestRealtimePingSectionsIncludeSparks(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true})
	node := komari.Node{UUID: "node-1", Name: "node"}
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	current := komari.Status{
		Time: komari.NullTime{Time: now, Valid: true},
		Ping: map[string]komari.Ping{
			"tokyo": {Name: "tokyo", Latest: 30, Avg: 25, Loss: 0},
		},
	}
	app.window = 0
	app.nodeDetail[detailKey{UUID: node.UUID, Window: app.window}] = nodeDetail{
		Recent: komari.RecentStatusResp{Records: []komari.Status{
			{
				Time: komari.NullTime{Time: now.Add(-2 * time.Minute), Valid: true},
				Ping: map[string]komari.Ping{"tokyo": {Name: "tokyo", Latest: 10, Avg: 12, Loss: 0}},
			},
			{
				Time: komari.NullTime{Time: now.Add(-1 * time.Minute), Valid: true},
				Ping: map[string]komari.Ping{"tokyo": {Name: "tokyo", Latest: 20, Avg: 18, Loss: 1}},
			},
		}},
	}

	app.tab = 3
	sections := app.pingSections(node, current)
	if !hasSection(sections, "Latency Spark") {
		t.Fatalf("missing Latency Spark section: %#v", sectionTitles(sections))
	}
	if !hasSection(sections, "Loss Spark") {
		t.Fatalf("missing Loss Spark section: %#v", sectionTitles(sections))
	}
}

func hasSection(sections []detailSection, title string) bool {
	for _, section := range sections {
		if section.Title == title {
			return true
		}
	}
	return false
}

func sectionTitles(sections []detailSection) []string {
	titles := make([]string, 0, len(sections))
	for _, section := range sections {
		titles = append(titles, section.Title)
	}
	return titles
}
