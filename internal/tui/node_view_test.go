package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"ktui/internal/komari"
)

func testNodeViewApp() *App {
	now := time.Now()
	app := NewWithOptions(nil, Options{WarnCPU: 90, WarnRAM: 85, WarnDisk: 90, WarnExpiryDays: 7})
	nodes := []komari.Node{
		{UUID: "n1", Name: "alpha", Region: "tokyo", Tags: "edge", IPv4: "192.0.2.1", MemTotal: 1000, DiskTotal: 1000, ExpiredAt: komari.NullTime{Time: now.Add(30 * 24 * time.Hour), Valid: true}},
		{UUID: "n2", Name: "beta", Region: "london", Tags: "db", IPv6: "2001:db8::2", MemTotal: 1000, DiskTotal: 1000, ExpiredAt: komari.NullTime{Time: now.Add(3 * 24 * time.Hour), Valid: true}},
		{UUID: "n3", Name: "gamma", Region: "new-york", Tags: "cache", MemTotal: 1000, DiskTotal: 1000},
	}
	app.snapshot = komari.Snapshot{
		Nodes: nodes,
		Status: map[string]komari.Status{
			"n1": {Online: true, CPU: 20, RAM: 200, RAMTotal: 1000, Disk: 200, DiskTotal: 1000, NetTotalUp: 100, NetTotalDown: 100},
			"n2": {Online: false, CPU: 50, RAM: 500, RAMTotal: 1000, Disk: 500, DiskTotal: 1000, NetTotalUp: 1000, NetTotalDown: 2000},
			"n3": {Online: true, CPU: 95, RAM: 900, RAMTotal: 1000, Disk: 100, DiskTotal: 1000, NetTotalUp: 500, NetTotalDown: 500},
		},
	}
	return app
}

func TestNodeSearchMatchesNameRegionTagsAndIP(t *testing.T) {
	app := testNodeViewApp()
	for _, tc := range []struct {
		query string
		want  string
	}{
		{query: "alp", want: "n1"},
		{query: "london", want: "n2"},
		{query: "cache", want: "n3"},
		{query: "2001:db8", want: "n2"},
	} {
		app.searchQuery = tc.query
		nodes := app.viewNodes()
		if len(nodes) != 1 || nodes[0].UUID != tc.want {
			t.Fatalf("query %q nodes = %#v, want %s", tc.query, nodes, tc.want)
		}
	}
}

func TestNodeFilters(t *testing.T) {
	app := testNodeViewApp()

	app.nodeFilter = nodeFilterOffline
	nodes := app.viewNodes()
	if len(nodes) != 1 || nodes[0].UUID != "n2" {
		t.Fatalf("offline nodes = %#v, want n2", nodes)
	}

	app.nodeFilter = nodeFilterHighLoad
	nodes = app.viewNodes()
	if len(nodes) != 1 || nodes[0].UUID != "n3" {
		t.Fatalf("high-load nodes = %#v, want n3", nodes)
	}

	app.nodeFilter = nodeFilterExpiring
	nodes = app.viewNodes()
	if len(nodes) != 1 || nodes[0].UUID != "n2" {
		t.Fatalf("expiring nodes = %#v, want n2", nodes)
	}
}

func TestCycleNodeFilterDoesNotEnterEmptyFilter(t *testing.T) {
	app := NewWithOptions(nil, Options{WarnCPU: 90, WarnRAM: 85, WarnDisk: 90, WarnExpiryDays: 7})
	app.snapshot = komari.Snapshot{
		Nodes: []komari.Node{
			{UUID: "n1", Name: "alpha", MemTotal: 1000, DiskTotal: 1000},
			{UUID: "n2", Name: "beta", MemTotal: 1000, DiskTotal: 1000},
		},
		Status: map[string]komari.Status{
			"n1": {Online: true, CPU: 20, RAM: 200, RAMTotal: 1000, Disk: 200, DiskTotal: 1000},
			"n2": {Online: true, CPU: 30, RAM: 300, RAMTotal: 1000, Disk: 300, DiskTotal: 1000},
		},
	}

	app.handleKey(context.Background(), keyEvent{name: "filter", text: "v"})
	if app.nodeFilter != nodeFilterAll {
		t.Fatalf("nodeFilter = %q, want all when every special filter is empty", app.nodeFilter)
	}
	if len(app.viewNodes()) != 2 {
		t.Fatalf("visible nodes = %d, want full list", len(app.viewNodes()))
	}
	if app.notice == "" {
		t.Fatal("notice should explain that special filters have no matches")
	}
}

func TestCycleNodeFilterSkipsEmptyFilters(t *testing.T) {
	app := NewWithOptions(nil, Options{WarnCPU: 90, WarnRAM: 85, WarnDisk: 90, WarnExpiryDays: 7})
	app.snapshot = komari.Snapshot{
		Nodes: []komari.Node{
			{UUID: "n1", Name: "alpha", MemTotal: 1000, DiskTotal: 1000},
			{UUID: "n2", Name: "beta", MemTotal: 1000, DiskTotal: 1000},
		},
		Status: map[string]komari.Status{
			"n1": {Online: true, CPU: 20, RAM: 200, RAMTotal: 1000, Disk: 200, DiskTotal: 1000},
			"n2": {Online: true, CPU: 95, RAM: 300, RAMTotal: 1000, Disk: 300, DiskTotal: 1000},
		},
	}

	app.handleKey(context.Background(), keyEvent{name: "filter", text: "v"})
	if app.nodeFilter != nodeFilterHighLoad {
		t.Fatalf("nodeFilter = %q, want high-load after skipping empty filters", app.nodeFilter)
	}
	nodes := app.viewNodes()
	if len(nodes) != 1 || nodes[0].UUID != "n2" {
		t.Fatalf("high-load nodes = %#v, want n2", nodes)
	}
}

func TestNodeSorts(t *testing.T) {
	app := testNodeViewApp()

	app.nodeSort = nodeSortCPU
	nodes := app.viewNodes()
	if nodes[0].UUID != "n3" {
		t.Fatalf("CPU sort first = %s, want n3", nodes[0].UUID)
	}

	app.nodeSort = nodeSortTraffic
	nodes = app.viewNodes()
	if nodes[0].UUID != "n2" {
		t.Fatalf("traffic sort first = %s, want n2", nodes[0].UUID)
	}

	app.nodeSort = nodeSortExpiry
	nodes = app.viewNodes()
	if nodes[0].UUID != "n2" {
		t.Fatalf("expiry sort first = %s, want n2", nodes[0].UUID)
	}
}

func TestSearchEditingTreatsCommandKeysAsText(t *testing.T) {
	app := testNodeViewApp()

	app.handleKey(context.Background(), keyEvent{name: "search", text: "/"})
	app.handleKey(context.Background(), keyEvent{name: "quit", text: "q"})
	app.handleKey(context.Background(), keyEvent{name: "settings", text: "s"})
	app.handleKey(context.Background(), keyEvent{name: "open", text: "o"})
	app.handleKey(context.Background(), keyEvent{name: "open"})

	if app.quit || app.settings {
		t.Fatalf("search input triggered command: quit=%t settings=%t", app.quit, app.settings)
	}
	if app.searchQuery != "qso" {
		t.Fatalf("searchQuery = %q, want qso", app.searchQuery)
	}
}

func TestSearchEditingRendersVisibleInputLine(t *testing.T) {
	app := testNodeViewApp()
	app.style.NoColor = true
	app.handleKey(context.Background(), keyEvent{name: "search", text: "/"})
	app.handleKey(context.Background(), keyEvent{name: "char", text: "alpha"})

	lines := app.renderListBody(80, 8)
	if len(lines) == 0 {
		t.Fatal("renderListBody returned no lines")
	}
	if !strings.Contains(lines[0], "Search") || !strings.Contains(lines[0], "alpha|") {
		t.Fatalf("search input line = %q, want visible search draft with cursor", lines[0])
	}

	app.handleKey(context.Background(), keyEvent{name: "open"})
	lines = app.renderListBody(80, 8)
	if !strings.Contains(lines[0], "Search") || !strings.Contains(lines[0], "alpha") {
		t.Fatalf("active search line = %q, want visible applied search", lines[0])
	}
}

func TestQuitBacksOutOfSearchResultsBeforeExiting(t *testing.T) {
	app := testNodeViewApp()
	app.searchQuery = "beta"

	nodes := app.viewNodes()
	if len(nodes) != 1 || nodes[0].UUID != "n2" {
		t.Fatalf("search nodes = %#v, want n2 only", nodes)
	}

	app.handleKey(context.Background(), keyEvent{name: "open"})
	if !app.detail {
		t.Fatal("open should enter detail")
	}

	app.handleKey(context.Background(), keyEvent{name: "quit", text: "q"})
	if app.detail {
		t.Fatal("first q should leave detail")
	}
	if app.searchQuery != "beta" {
		t.Fatalf("searchQuery = %q, want beta after leaving detail", app.searchQuery)
	}

	app.handleKey(context.Background(), keyEvent{name: "quit", text: "q"})
	if app.quit {
		t.Fatal("second q should clear search results instead of exiting")
	}
	if app.searchQuery != "" {
		t.Fatalf("searchQuery = %q, want cleared", app.searchQuery)
	}
	if len(app.viewNodes()) != len(app.snapshot.Nodes) {
		t.Fatalf("visible nodes = %d, want full list %d", len(app.viewNodes()), len(app.snapshot.Nodes))
	}

	app.handleKey(context.Background(), keyEvent{name: "quit", text: "q"})
	if !app.quit {
		t.Fatal("third q from full list should exit")
	}
}
