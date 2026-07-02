package tui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ktui/internal/komari"
)

func TestNewWithOptionsTimeoutDefaults(t *testing.T) {
	app := NewWithOptions(nil, Options{})

	if app.refreshInterval != defaultRefreshInterval {
		t.Fatalf("refreshInterval = %s, want %s", app.refreshInterval, defaultRefreshInterval)
	}
	if app.fetchTimeout != defaultFetchTimeout {
		t.Fatalf("fetchTimeout = %s, want %s", app.fetchTimeout, defaultFetchTimeout)
	}
	if app.detailTimeout != defaultDetailTimeout {
		t.Fatalf("detailTimeout = %s, want %s", app.detailTimeout, defaultDetailTimeout)
	}
	if app.detailCacheTTL != defaultDetailCacheTTL {
		t.Fatalf("detailCacheTTL = %s, want %s", app.detailCacheTTL, defaultDetailCacheTTL)
	}
	if app.realtimePoints != 0 {
		t.Fatalf("realtimePoints = %d, want auto", app.realtimePoints)
	}
}

func TestNewWithOptionsUsesTimeoutOptions(t *testing.T) {
	app := NewWithOptions(nil, Options{
		RefreshInterval: 2 * time.Second,
		FetchTimeout:    3 * time.Second,
		DetailTimeout:   4 * time.Second,
		DetailCacheTTL:  5 * time.Second,
		RealtimePoints:  150,
	})

	if app.refreshInterval != 2*time.Second {
		t.Fatalf("refreshInterval = %s", app.refreshInterval)
	}
	if app.fetchTimeout != 3*time.Second {
		t.Fatalf("fetchTimeout = %s", app.fetchTimeout)
	}
	if app.detailTimeout != 4*time.Second {
		t.Fatalf("detailTimeout = %s", app.detailTimeout)
	}
	if app.detailCacheTTL != 5*time.Second {
		t.Fatalf("detailCacheTTL = %s", app.detailCacheTTL)
	}
	if app.realtimePoints != 150 {
		t.Fatalf("realtimePoints = %d", app.realtimePoints)
	}
}

func TestStartUpdateCheckRecordsAvailableUpdate(t *testing.T) {
	app := NewWithOptions(nil, Options{
		CheckUpdate: func(ctx context.Context) (UpdateCheckResult, error) {
			return UpdateCheckResult{LatestVersion: "v0.2.0", AssetName: "ktui_v0.2.0_linux_amd64.tar.gz", Available: true}, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.startUpdateCheck(ctx)
	result := <-app.updateCh
	app.update.Checking = false
	app.update.Checked = true
	app.update.Err = result.err
	app.update.Available = result.result.Available
	app.update.Latest = result.result.LatestVersion

	if !app.update.Available || app.update.Latest != "v0.2.0" {
		t.Fatalf("update = %+v, want available v0.2.0", app.update)
	}
}

func TestUpdateAvailableFooterAndHint(t *testing.T) {
	app := NewWithOptions(nil, Options{})
	app.update.Available = true
	app.update.Latest = "v0.2.0"

	if !strings.Contains(app.footerText(), "u update") {
		t.Fatalf("footer = %q, want update action", app.footerText())
	}

	app.handleKey(context.Background(), keyEvent{name: "update-hint"})
	if !strings.Contains(app.notice, "ktui update") {
		t.Fatalf("notice = %q, want update command", app.notice)
	}
}

func TestUpdateFooterClickShowsHint(t *testing.T) {
	app := NewWithOptions(nil, Options{})
	app.update.Available = true
	app.update.Latest = "v0.2.0"
	_, height := terminalSize()
	x, ok := footerActionPosition(app, footerUpdate)
	if !ok {
		t.Fatal("missing update footer action")
	}

	app.handleKey(context.Background(), keyEvent{name: "mouse-left", x: x, y: height})
	if !strings.Contains(app.notice, "v0.2.0") {
		t.Fatalf("notice = %q, want update hint", app.notice)
	}
}

func TestRealtimeRecordsMergeRecentAndLiveSamples(t *testing.T) {
	app := NewWithOptions(nil, Options{RefreshInterval: 2 * time.Second})
	node := komari.Node{UUID: "node-1", Name: "node"}
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	first := komari.Status{
		CPU:  10,
		Time: komari.NullTime{Time: now.Add(-5 * time.Second), Valid: true},
	}
	second := komari.Status{
		CPU:  20,
		Time: komari.NullTime{Time: now, Valid: true},
	}

	app.snapshot = komari.Snapshot{
		Nodes:     []komari.Node{node},
		Status:    map[string]komari.Status{node.UUID: first},
		FetchedAt: now,
	}
	app.recordRealtimeSample(now)
	app.snapshot = komari.Snapshot{
		Nodes:     []komari.Node{node},
		Status:    map[string]komari.Status{node.UUID: second},
		FetchedAt: now.Add(5 * time.Second),
	}
	app.recordRealtimeSample(now.Add(5 * time.Second))

	recent := []komari.Status{
		{CPU: 1, Time: komari.NullTime{Time: now.Add(-10 * time.Minute), Valid: true}},
		{CPU: 2, Time: komari.NullTime{Time: now.Add(-9 * time.Minute), Valid: true}},
	}
	records := app.realtimeRecords(node.UUID, recent, second)
	if len(records) != 4 {
		t.Fatalf("records len = %d, want 4", len(records))
	}
	if got := statusValues(records, func(st komari.Status) float64 { return st.CPU }); !sameFloatSlice(got, []float64{1, 2, 10, 20}) {
		t.Fatalf("records CPUs = %#v; want [1 2 10 20]", got)
	}
	if !records[2].Time.Time.Equal(first.Time.Time) || !records[3].Time.Time.Equal(second.Time.Time) {
		t.Fatalf("records times = %s, %s; want live record times", records[2].Time.Time, records[3].Time.Time)
	}
}

func TestRealtimeRecordsLimitBySampleWindow(t *testing.T) {
	app := NewWithOptions(nil, Options{RefreshInterval: 20 * time.Second})
	node := komari.Node{UUID: "node-1", Name: "node"}
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 4; i++ {
		sampleTime := now.Add(time.Duration(i) * app.refreshInterval)
		app.snapshot = komari.Snapshot{
			Nodes: []komari.Node{node},
			Status: map[string]komari.Status{node.UUID: {
				CPU: float64(i + 1),
			}},
			FetchedAt: sampleTime,
		}
		app.recordRealtimeSample(sampleTime)
	}

	records := app.realtimeRecords(node.UUID, nil, komari.Status{})
	if len(records) != 3 {
		t.Fatalf("records len = %d, want 3", len(records))
	}
	if got := statusValues(records, func(st komari.Status) float64 { return st.CPU }); !sameFloatSlice(got, []float64{2, 3, 4}) {
		t.Fatalf("values = %#v, want [2 3 4]", got)
	}
}

func TestRealtimeRecordsSlideWhenNewSnapshotArrives(t *testing.T) {
	app := NewWithOptions(nil, Options{RefreshInterval: 20 * time.Second})
	node := komari.Node{UUID: "node-1", Name: "node"}
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	for i, cpu := range []float64{1, 2, 3} {
		sampleTime := now.Add(time.Duration(i) * app.refreshInterval)
		app.recordRealtimeSnapshot(komari.Snapshot{
			Nodes:     []komari.Node{node},
			Status:    map[string]komari.Status{node.UUID: {CPU: cpu}},
			FetchedAt: sampleTime,
		}, sampleTime)
	}

	records := app.realtimeRecords(node.UUID, nil, komari.Status{})
	if got := statusValues(records, func(st komari.Status) float64 { return st.CPU }); !sameFloatSlice(got, []float64{1, 2, 3}) {
		t.Fatalf("values = %#v, want [1 2 3]", got)
	}

	nextTime := now.Add(3 * app.refreshInterval)
	app.recordRealtimeSnapshot(komari.Snapshot{
		Nodes:     []komari.Node{node},
		Status:    map[string]komari.Status{node.UUID: {CPU: 3}},
		FetchedAt: nextTime,
	}, nextTime)

	records = app.realtimeRecords(node.UUID, nil, komari.Status{})
	if got := statusValues(records, func(st komari.Status) float64 { return st.CPU }); !sameFloatSlice(got, []float64{2, 3, 3}) {
		t.Fatalf("values = %#v, want [2 3 3]", got)
	}
}

func TestRealtimeSampleUsesIntervalClockWhenStatusHasNoRecordTime(t *testing.T) {
	app := NewWithOptions(nil, Options{RefreshInterval: 2 * time.Second})
	node := komari.Node{UUID: "node-1", Name: "node"}
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	app.snapshot = komari.Snapshot{
		Nodes: []komari.Node{node},
		Status: map[string]komari.Status{node.UUID: {
			CPU: 50,
		}},
		FetchedAt: now,
	}

	app.recordRealtimeSample(now)
	app.recordRealtimeSample(now.Add(2 * time.Second))
	app.recordRealtimeSample(now.Add(4 * time.Second))

	records := app.realtimeRecords(node.UUID, nil, komari.Status{})
	if len(records) != 3 {
		t.Fatalf("records len = %d, want 3", len(records))
	}
	for i, record := range records {
		want := now.Add(time.Duration(i*2) * time.Second)
		if !record.Time.Time.Equal(want) {
			t.Fatalf("record %d time = %s, want %s", i, record.Time.Time, want)
		}
	}
}

func TestRealtimeSampleLimitUsesRefreshInterval(t *testing.T) {
	app := NewWithOptions(nil, Options{RefreshInterval: 2 * time.Second})

	if got, want := app.maxRealtimeSamples(), 30; got != want {
		t.Fatalf("maxRealtimeSamples = %d, want %d", got, want)
	}
}

func TestRealtimeSampleLimitCanUseConfiguredPoints(t *testing.T) {
	app := NewWithOptions(nil, Options{
		RefreshInterval: 20 * time.Second,
		RealtimePoints:  5,
	})
	node := komari.Node{UUID: "node-1", Name: "node"}
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 6; i++ {
		sampleTime := now.Add(time.Duration(i) * app.refreshInterval)
		app.recordRealtimeSnapshot(komari.Snapshot{
			Nodes: []komari.Node{node},
			Status: map[string]komari.Status{node.UUID: {
				CPU: float64(i + 1),
			}},
			FetchedAt: sampleTime,
		}, sampleTime)
	}

	records := app.realtimeRecords(node.UUID, nil, komari.Status{})
	if len(records) != 5 {
		t.Fatalf("records len = %d, want 5", len(records))
	}
	if got := statusValues(records, func(st komari.Status) float64 { return st.CPU }); !sameFloatSlice(got, []float64{2, 3, 4, 5, 6}) {
		t.Fatalf("values = %#v, want [2 3 4 5 6]", got)
	}
}

type fakeRefreshTicker struct {
	reset time.Duration
}

func (f *fakeRefreshTicker) Reset(interval time.Duration) {
	f.reset = interval
}

func TestResetRefreshTickerWhenIntervalChanges(t *testing.T) {
	app := NewWithOptions(nil, Options{RefreshInterval: 2 * time.Second})
	app.refreshInterval = 5 * time.Second
	app.intervalChanged = true
	ticker := &fakeRefreshTicker{}

	app.resetRefreshTickerIfNeeded(ticker)

	if ticker.reset != 5*time.Second {
		t.Fatalf("ticker reset = %s, want 5s", ticker.reset)
	}
	if app.intervalChanged {
		t.Fatal("intervalChanged should be cleared after ticker reset")
	}
}

func sameFloatSlice(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestNeedsFullFetchUsesPeriodicFullRefresh(t *testing.T) {
	app := NewWithOptions(nil, Options{})
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)

	if !app.needsFullFetch(now) {
		t.Fatal("empty app should need a full fetch")
	}

	app.snapshot = komari.Snapshot{Nodes: []komari.Node{{UUID: "node-1"}}}
	app.lastFullFetch = now
	if app.needsFullFetch(now.Add(fullRefreshInterval - time.Second)) {
		t.Fatal("recent full fetch should allow fast refresh")
	}
	if !app.needsFullFetch(now.Add(fullRefreshInterval)) {
		t.Fatal("stale full fetch should require a full refresh")
	}
}

func TestFetchSnapshotFastUsesLatestStatusOnly(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			ID     int    `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		methods = append(methods, req.Method)
		if req.Method != "common:getNodesLatestStatus" {
			t.Errorf("method = %s, want common:getNodesLatestStatus", req.Method)
		}
		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result": map[string]any{
				"node-1": map[string]any{
					"cpu":    42,
					"online": true,
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := komari.NewClientWithOptions(server.URL, komari.Options{})
	if err != nil {
		t.Fatal(err)
	}
	app := NewWithOptions(client, Options{})
	previous := komari.Snapshot{
		Nodes:   []komari.Node{{UUID: "node-1", Name: "node"}},
		Public:  komari.PublicInfo{SiteName: "site"},
		Version: komari.VersionInfo{Version: "v1"},
	}

	snapshot, err := app.fetchSnapshot(context.Background(), previous, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(methods) != 1 {
		t.Fatalf("methods = %#v, want one latest-status call", methods)
	}
	if snapshot.Status["node-1"].CPU != 42 {
		t.Fatalf("cpu = %.1f, want 42", snapshot.Status["node-1"].CPU)
	}
	if snapshot.Public.SiteName != "site" || snapshot.Version.Version != "v1" {
		t.Fatalf("snapshot metadata was not preserved: %#v", snapshot)
	}
}
