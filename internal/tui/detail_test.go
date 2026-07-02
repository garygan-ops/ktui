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
	if !hasSection(sections, "Network Chart") {
		t.Fatalf("missing Network Chart section: %#v", sectionTitles(sections))
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
	for _, title := range []string{"CPU Chart", "RAM Chart", "Disk Chart", "Network Chart", "Connections Chart", "Process Chart"} {
		if !hasSection(sections, title) {
			t.Fatalf("missing %s section: %#v", title, sectionTitles(sections))
		}
	}
}

func TestHistoryMetricSectionsMatchWebLoadChartSet(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true})
	node := komari.Node{UUID: "node-1", Name: "node", MemTotal: 1000, DiskTotal: 2000}
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	records := []komari.Status{
		{CPU: 10, RAM: 100, RAMTotal: 1000, Disk: 300, DiskTotal: 2000, NetOut: 100, NetIn: 50, Connections: 5, Process: 40, Time: komari.NullTime{Time: now, Valid: true}},
	}

	sections := app.historyMetricSections(node, records[0], records, "Realtime")
	titles := sectionTitles(sections)
	wantPrefix := []string{"CPU Chart", "RAM Chart", "Disk Chart", "Network Chart", "Connections Chart", "Process Chart"}
	for i, want := range wantPrefix {
		if i >= len(titles) {
			t.Fatalf("missing section %d want %q; all=%#v", i, want, titles)
		}
		if titles[i] != want {
			t.Fatalf("section %d = %q, want %q; all=%#v", i, titles[i], want, titles)
		}
	}
}

func TestLoadChartRecordsThinByTimeBucket(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	records := []komari.Status{
		{CPU: 10, Time: komari.NullTime{Time: now, Valid: true}},
		{CPU: 20, Time: komari.NullTime{Time: now.Add(20 * time.Second), Valid: true}},
		{CPU: 30, Time: komari.NullTime{Time: now.Add(time.Minute), Valid: true}},
	}

	thinned := loadChartRecords(records, 4)
	if len(thinned) != 2 {
		t.Fatalf("thinned len = %d, want 2", len(thinned))
	}
	if got := statusValues(thinned, func(st komari.Status) float64 { return st.CPU }); !sameFloatSlice(got, []float64{20, 30}) {
		t.Fatalf("thinned CPUs = %#v, want [20 30]", got)
	}
}

func TestPingTaskSparkLinesSplitTasks(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	tasks := []komari.PingTask{
		{ID: 1, Name: "tokyo", Interval: 4},
		{ID: 2, Name: "london", Interval: 4},
	}
	records := []komari.PingRecord{
		{TaskID: 1, Time: komari.NullTime{Time: now, Valid: true}, Value: 20},
		{TaskID: 2, Time: komari.NullTime{Time: now.Add(500 * time.Millisecond), Valid: true}, Value: 80},
		{TaskID: 1, Time: komari.NullTime{Time: now.Add(time.Minute), Valid: true}, Value: 30},
		{TaskID: 2, Time: komari.NullTime{Time: now.Add(time.Minute + 500*time.Millisecond), Valid: true}, Value: 90},
	}

	rows := pingChartRows(tasks, records)
	if len(rows) != 2 {
		t.Fatalf("rows len = %d, want 2", len(rows))
	}
	lines := pingTaskSparkLines(tasks, records, true, 34, 4)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "tokyo") || !strings.Contains(joined, "london") {
		t.Fatalf("missing per-task spark lines: %#v", lines)
	}
	if strings.Contains(joined, "80ms") && strings.Contains(lines[0], "tokyo") {
		t.Fatalf("task values appear mixed: %#v", lines)
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

func TestAxisChartLinesRightAlignsRealtimeValues(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true})
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	lines := app.axisChartLines(axisChart{
		Values: []float64{10, 20},
		Times:  []time.Time{now.Add(-58 * time.Second), now},
		From:   "12:00",
		To:     "12:01",
		Unit:   "%",
		Window: time.Minute,
		Until:  now,
	}, 24, 4)

	firstPlot := strings.Index(lines[0], "|")
	firstStar := strings.Index(lines[0], "*")
	if firstPlot < 0 || firstStar < 0 {
		t.Fatalf("missing plot or point: %#v", lines)
	}
	if firstStar-firstPlot < 10 {
		t.Fatalf("point was not right-aligned: %#v", lines)
	}
}

func TestRealtimeChartPointsMoveLeftAsWindowClockAdvances(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	sampleTime := now.Add(-30 * time.Second)
	initial := chartPoints(axisChart{
		Values: []float64{10},
		Times:  []time.Time{sampleTime},
		Window: time.Minute,
		Until:  now,
	}, 31)
	advanced := chartPoints(axisChart{
		Values: []float64{10},
		Times:  []time.Time{sampleTime},
		Window: time.Minute,
		Until:  now.Add(2 * time.Second),
	}, 31)

	initialIndex := firstValidChartPoint(initial)
	advancedIndex := firstValidChartPoint(advanced)
	if initialIndex < 0 || advancedIndex < 0 {
		t.Fatalf("missing chart point: initial=%#v advanced=%#v", initial, advanced)
	}
	if advancedIndex >= initialIndex {
		t.Fatalf("point index = %d after clock advanced, want < %d", advancedIndex, initialIndex)
	}
}

func TestRealtimeMetricChartUsesRecordRange(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true})
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	records := []komari.Status{
		{CPU: 10, Time: komari.NullTime{Time: now.Add(-30 * time.Second), Valid: true}},
		{CPU: 20, Time: komari.NullTime{Time: now, Valid: true}},
	}

	section := app.metricChartSection("CPU Chart", records, "%", statusValues(records, func(st komari.Status) float64 { return st.CPU }), true)
	if section.Chart == nil {
		t.Fatal("missing chart")
	}
	if section.Chart.Window != 0 || !section.Chart.Until.IsZero() {
		t.Fatalf("realtime chart should use record order, got window=%s until=%s", section.Chart.Window, section.Chart.Until)
	}
	if section.Chart.From != chartRealtimeTimeLabelFromTime(records[0].Time.Time) {
		t.Fatalf("chart From = %q, want %q", section.Chart.From, chartRealtimeTimeLabelFromTime(records[0].Time.Time))
	}
	if section.Chart.To != chartRealtimeTimeLabelFromTime(records[1].Time.Time) {
		t.Fatalf("chart To = %q, want %q", section.Chart.To, chartRealtimeTimeLabelFromTime(records[1].Time.Time))
	}
}

func TestSequenceChartPointsUseFullPlotWidth(t *testing.T) {
	points := chartPoints(axisChart{
		Values: []float64{1, 2, 3},
	}, 9)

	if len(points) != 9 {
		t.Fatalf("points len = %d, want 9", len(points))
	}
	if !points[0].Valid || points[0].Value != 1 {
		t.Fatalf("first point = %#v, want value 1 at left edge", points[0])
	}
	if !points[4].Valid || points[4].Value != 2 {
		t.Fatalf("middle point = %#v, want value 2 in the middle", points[4])
	}
	if !points[8].Valid || points[8].Value != 3 {
		t.Fatalf("last point = %#v, want value 3 at right edge", points[8])
	}
}

func TestSequenceChartPointsReflectRollingValues(t *testing.T) {
	before := chartPoints(axisChart{Values: []float64{1, 2, 3}}, 9)
	after := chartPoints(axisChart{Values: []float64{2, 3, 3}}, 9)

	if before[0].Value != 1 || after[0].Value != 2 {
		t.Fatalf("left edge did not roll from 1 to 2: before=%#v after=%#v", before[0], after[0])
	}
	if before[4].Value != 2 || after[4].Value != 3 {
		t.Fatalf("middle did not roll from 2 to 3: before=%#v after=%#v", before[4], after[4])
	}
	if before[8].Value != 3 || after[8].Value != 3 {
		t.Fatalf("right edge should keep latest 3: before=%#v after=%#v", before[8], after[8])
	}
}

func TestHistoryMetricChartUsesFixedTimeWindow(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true})
	app.window = 1
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	records := []komari.Status{
		{CPU: 10, Time: komari.NullTime{Time: now.Add(-30 * time.Minute), Valid: true}},
		{CPU: 20, Time: komari.NullTime{Time: now, Valid: true}},
	}

	section := app.metricChartSection("CPU Chart", records, "%", statusValues(records, func(st komari.Status) float64 { return st.CPU }), false)
	if section.Chart == nil {
		t.Fatal("missing chart")
	}
	if section.Chart.Window != 4*time.Hour {
		t.Fatalf("chart Window = %s, want 4h", section.Chart.Window)
	}
	if !section.Chart.Until.Equal(now) {
		t.Fatalf("chart Until = %s, want %s", section.Chart.Until, now)
	}
}

func TestAxisChartLinesUseStableWidth(t *testing.T) {
	app := NewWithOptions(nil, Options{ASCII: true})
	narrow := app.axisChartLines(axisChart{
		Values: []float64{1, 2, 3},
		From:   "12:00",
		To:     "12:02",
		Unit:   "%",
	}, 42, 4)
	wide := app.axisChartLines(axisChart{
		Values: []float64{100, 200, 300},
		From:   "12:00",
		To:     "12:02",
		Unit:   "%",
	}, 42, 4)

	for _, lines := range [][]string{narrow, wide} {
		for _, line := range lines {
			if displayWidth(line) != 42 {
				t.Fatalf("line width = %d, want 42: %#v", displayWidth(line), lines)
			}
		}
	}
	if strings.Index(narrow[0], "|") != strings.Index(wide[0], "|") {
		t.Fatalf("y axis moved between value scales: %#v vs %#v", narrow, wide)
	}
}

func firstValidChartPoint(points []chartPoint) int {
	for i, point := range points {
		if point.Valid {
			return i
		}
	}
	return -1
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
