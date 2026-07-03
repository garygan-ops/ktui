package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"ktui/internal/komari"
)

type statusSummary struct {
	CPUAvg         float64
	CPUMax         float64
	RAMAvg         float64
	RAMMax         float64
	DiskAvg        float64
	DiskMax        float64
	LoadAvg        float64
	LoadMax        float64
	NetOutAvg      float64
	NetInAvg       float64
	NetOutMax      int64
	NetInMax       int64
	ConnectionsAvg float64
	ConnectionsMax int
	ProcessAvg     float64
	ProcessMax     int
}

func summarizeStatus(records []komari.Status) statusSummary {
	return summarizeStatusWithTotals(records, 0, 0)
}

func summarizeStatusWithTotals(records []komari.Status, ramTotalFallback int64, diskTotalFallback int64) statusSummary {
	var sum statusSummary
	if len(records) == 0 {
		return sum
	}
	var cpuTotal, ramTotal, diskTotal, loadTotal, netInTotal, netOutTotal, connectionsTotal, processTotal float64
	for _, st := range records {
		cpuTotal += st.CPU
		sum.CPUMax = maxFloat(sum.CPUMax, st.CPU)
		ramPct := percent(st.RAM, firstNonZero(st.RAMTotal, ramTotalFallback))
		ramTotal += ramPct
		sum.RAMMax = maxFloat(sum.RAMMax, ramPct)
		diskPct := percent(st.Disk, firstNonZero(st.DiskTotal, diskTotalFallback))
		diskTotal += diskPct
		sum.DiskMax = maxFloat(sum.DiskMax, diskPct)
		loadTotal += st.Load
		sum.LoadMax = maxFloat(sum.LoadMax, st.Load)
		netInTotal += float64(st.NetIn)
		netOutTotal += float64(st.NetOut)
		connectionsTotal += float64(st.Connections)
		if st.Connections > sum.ConnectionsMax {
			sum.ConnectionsMax = st.Connections
		}
		processTotal += float64(st.Process)
		if st.Process > sum.ProcessMax {
			sum.ProcessMax = st.Process
		}
		if st.NetIn > sum.NetInMax {
			sum.NetInMax = st.NetIn
		}
		if st.NetOut > sum.NetOutMax {
			sum.NetOutMax = st.NetOut
		}
	}
	count := float64(len(records))
	sum.CPUAvg = cpuTotal / count
	sum.RAMAvg = ramTotal / count
	sum.DiskAvg = diskTotal / count
	sum.LoadAvg = loadTotal / count
	sum.NetInAvg = netInTotal / count
	sum.NetOutAvg = netOutTotal / count
	sum.ConnectionsAvg = connectionsTotal / count
	sum.ProcessAvg = processTotal / count
	return sum
}

func statusValues(records []komari.Status, pick func(komari.Status) float64) []float64 {
	values := make([]float64, 0, len(records))
	for _, record := range records {
		values = append(values, pick(record))
	}
	return values
}

func statusRAMPercentValues(records []komari.Status, fallbackTotal int64) []float64 {
	values := make([]float64, 0, len(records))
	for _, record := range records {
		values = append(values, percent(record.RAM, firstNonZero(record.RAMTotal, fallbackTotal)))
	}
	return values
}

func statusSwapPercentValues(records []komari.Status, fallbackTotal int64) []float64 {
	values := make([]float64, 0, len(records))
	for _, record := range records {
		values = append(values, percent(record.Swap, firstNonZero(record.SwapTotal, fallbackTotal)))
	}
	return values
}

func statusDiskPercentValues(records []komari.Status, fallbackTotal int64) []float64 {
	values := make([]float64, 0, len(records))
	for _, record := range records {
		values = append(values, percent(record.Disk, firstNonZero(record.DiskTotal, fallbackTotal)))
	}
	return values
}

func loadChartRecords(records []komari.Status, hours int) []komari.Status {
	if hours <= 0 || len(records) == 0 {
		return records
	}
	interval := loadChartInterval(hours)
	if interval <= 0 {
		return records
	}
	sorted := make([]komari.Status, 0, len(records))
	for _, record := range records {
		if record.Time.Valid {
			sorted = append(sorted, record)
		}
	}
	if len(sorted) == 0 {
		return records
	}
	sortStatusSamples(sorted)
	lastTime := sorted[len(sorted)-1].Time.Time
	fromTime := lastTime.Add(-time.Duration(hours)*time.Hour - interval)
	buckets := map[int64]komari.Status{}
	for _, record := range sorted {
		ts := record.Time.Time
		if ts.Before(fromTime) {
			continue
		}
		buckets[ts.UnixNano()/interval.Nanoseconds()] = record
	}
	out := make([]komari.Status, 0, len(buckets))
	for _, record := range buckets {
		out = append(out, record)
	}
	sortStatusSamples(out)
	return out
}

func loadChartInterval(hours int) time.Duration {
	switch {
	case hours > 120:
		return time.Hour
	case hours == 4:
		return time.Minute
	default:
		return 15 * time.Minute
	}
}

func statusTimes(records []komari.Status) []time.Time {
	values := make([]time.Time, 0, len(records))
	for _, record := range records {
		if record.Time.Valid {
			values = append(values, record.Time.Time)
		} else {
			values = append(values, time.Time{})
		}
	}
	return values
}

func firstRecordTime(records []komari.Status) komari.NullTime {
	for _, record := range records {
		if record.Time.Valid {
			return record.Time
		}
	}
	return komari.NullTime{}
}

func lastRecordTime(records []komari.Status) komari.NullTime {
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Time.Valid {
			return records[i].Time
		}
	}
	return komari.NullTime{}
}

func chartTimeLabel(t komari.NullTime) string {
	if !t.Valid {
		return "-"
	}
	return chartTimeLabelFromTime(t.Time)
}

func chartRealtimeTimeLabel(t komari.NullTime) string {
	if !t.Valid {
		return "-"
	}
	return chartRealtimeTimeLabelFromTime(t.Time)
}

func chartTimeLabelFromTime(t time.Time) string {
	return t.Local().Format("01-02 15:04")
}

func chartRealtimeTimeLabelFromTime(t time.Time) string {
	return t.Local().Format("15:04:05")
}

func chartPointRow(value, minVal, maxVal float64, rowCount int) int {
	if rowCount <= 1 {
		return 0
	}
	if maxVal == minVal {
		return rowCount / 2
	}
	ratio := (value - minVal) / (maxVal - minVal)
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	row := int(math.Round((1 - ratio) * float64(rowCount-1)))
	if row < 0 {
		return 0
	}
	if row >= rowCount {
		return rowCount - 1
	}
	return row
}

func downsampleValues(values []float64, width int) []float64 {
	if width <= 0 {
		return nil
	}
	if len(values) <= width {
		out := make([]float64, len(values))
		copy(out, values)
		return out
	}
	step := float64(len(values)) / float64(width)
	out := make([]float64, 0, width)
	for i := 0; i < width; i++ {
		start := int(float64(i) * step)
		end := int(float64(i+1) * step)
		if end <= start {
			end = start + 1
		}
		if end > len(values) {
			end = len(values)
		}
		var total float64
		for _, value := range values[start:end] {
			total += value
		}
		out = append(out, total/float64(end-start))
	}
	return out
}

type chartPoint struct {
	Value float64
	Valid bool
}

func chartPoints(chart axisChart, width int) []chartPoint {
	if width <= 0 {
		return nil
	}
	if chart.Window <= 0 || chart.Until.IsZero() || len(chart.Times) != len(chart.Values) {
		return sequenceChartPoints(chart.Values, width)
	}

	from := chart.Until.Add(-chart.Window)
	totals := make([]float64, width)
	counts := make([]int, width)
	for i, value := range chart.Values {
		ts := chart.Times[i]
		if ts.IsZero() || ts.Before(from) || ts.After(chart.Until) {
			continue
		}
		index := chartTimeIndex(ts, from, chart.Window, width)
		if index < 0 {
			continue
		}
		totals[index] += value
		counts[index]++
	}
	points := make([]chartPoint, width)
	for i := range points {
		if counts[i] > 0 {
			points[i] = chartPoint{Value: totals[i] / float64(counts[i]), Valid: true}
		}
	}
	return points
}

func chartPointsForValues(chart axisChart, values []float64, width int) []chartPoint {
	chart.Values = values
	chart.Series = nil
	return chartPoints(chart, width)
}

func sequenceChartPoints(values []float64, width int) []chartPoint {
	if width <= 0 {
		return nil
	}
	points := make([]chartPoint, width)
	if len(values) == 0 {
		return points
	}
	if len(values) == 1 {
		points[width-1] = chartPoint{Value: values[0], Valid: true}
		return points
	}
	totals := make([]float64, width)
	counts := make([]int, width)
	for i, value := range values {
		ratio := float64(i) / float64(len(values)-1)
		index := int(math.Round(ratio * float64(width-1)))
		if index < 0 {
			index = 0
		}
		if index >= width {
			index = width - 1
		}
		totals[index] += value
		counts[index]++
	}
	for i := range points {
		if counts[i] > 0 {
			points[i] = chartPoint{Value: totals[i] / float64(counts[i]), Valid: true}
		}
	}
	return points
}

func chartTimeIndex(ts time.Time, from time.Time, window time.Duration, width int) int {
	if width <= 1 {
		return 0
	}
	offset := ts.Sub(from)
	if offset < 0 || offset > window {
		return -1
	}
	ratio := float64(offset) / float64(window)
	index := int(math.Round(ratio * float64(width-1)))
	if index < 0 {
		return -1
	}
	if index >= width {
		return width - 1
	}
	return index
}

func chartLabelWidth(minVal, midVal, maxVal float64, unit string) int {
	switch unit {
	case "B/s":
		return 7
	case "%":
		return 6
	default:
		return 6
	}
}

func chartValueLabel(value float64, unit string, width int) string {
	label := compactFloat(value)
	if unit == "B/s" {
		label = compactByteFloat(value)
	} else if unit != "" {
		label += unit
	}
	if width <= 0 {
		return label
	}
	return padRight(cleanLine(label, width), width)
}

func compactFloat(value float64) string {
	switch {
	case value >= 1000:
		return fmt.Sprintf("%.0f", value)
	case value >= 100:
		return fmt.Sprintf("%.0f", value)
	case value >= 10:
		return fmt.Sprintf("%.1f", value)
	default:
		return fmt.Sprintf("%.2f", value)
	}
}

func compactByteFloat(value float64) string {
	units := []string{"B/s", "K/s", "M/s", "G/s", "T/s"}
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	switch {
	case value >= 100:
		return fmt.Sprintf("%.0f%s", value, units[unit])
	case value >= 10:
		return fmt.Sprintf("%.1f%s", value, units[unit])
	default:
		return fmt.Sprintf("%.2f%s", value, units[unit])
	}
}

func chartXAxisLines(chart axisChart, width int, labelWidth int, ascii bool, detailed bool) []string {
	prefix := strings.Repeat(" ", labelWidth)
	if ascii {
		prefix += "+"
	} else {
		prefix += "└"
	}
	plotWidth := max(1, width-displayWidth(prefix))
	axis := strings.Repeat("-", plotWidth)
	if !ascii {
		axis = strings.Repeat("─", plotWidth)
	}
	line := prefix + axis
	if detailed {
		if labels := chartDetailedXAxisLabels(chart, plotWidth); len(labels) > 0 {
			return []string{
				cleanLine(prefix+placeAxisTicks(labels, plotWidth, ascii), width),
				cleanLine(strings.Repeat(" ", displayWidth(prefix))+placeAxisLabels(labels, plotWidth), width),
			}
		}
	}
	label := chart.From + " -> " + chart.To
	if displayWidth(label) <= plotWidth {
		line = prefix + padRight(label, plotWidth)
	}
	return []string{cleanLine(line, width)}
}

type chartAxisLabel struct {
	Pos   int
	Label string
}

func chartDetailedXAxisLabels(chart axisChart, plotWidth int) []chartAxisLabel {
	from, to, ok := chartTimeRange(chart)
	if !ok || plotWidth < 8 {
		return nil
	}
	count := detailedXAxisTickCount(plotWidth, chartAxisTimeLayout(to.Sub(from)))
	if count < 2 {
		return nil
	}
	labels := make([]chartAxisLabel, 0, count)
	layout := chartAxisTimeLayout(to.Sub(from))
	for i := 0; i < count; i++ {
		ratio := 0.0
		if count > 1 {
			ratio = float64(i) / float64(count-1)
		}
		ts := from.Add(time.Duration(float64(to.Sub(from)) * ratio))
		pos := int(math.Round(ratio * float64(plotWidth-1)))
		labels = append(labels, chartAxisLabel{Pos: pos, Label: ts.Local().Format(layout)})
	}
	return labels
}

func chartDetailedXAxisLabel(chart axisChart) string {
	from, to, ok := chartTimeRange(chart)
	if !ok {
		return chart.From + " -> " + chart.To
	}
	layout := chartAxisTimeLayout(to.Sub(from))
	return from.Local().Format(layout) + " -> " + to.Local().Format(layout)
}

func chartTimeRange(chart axisChart) (time.Time, time.Time, bool) {
	if chart.Window > 0 && !chart.Until.IsZero() {
		return chart.Until.Add(-chart.Window), chart.Until, true
	}
	var from time.Time
	var to time.Time
	for _, ts := range chart.Times {
		if ts.IsZero() {
			continue
		}
		if from.IsZero() || ts.Before(from) {
			from = ts
		}
		if to.IsZero() || ts.After(to) {
			to = ts
		}
	}
	if from.IsZero() || to.IsZero() {
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}

func chartAxisTimeLayout(span time.Duration) string {
	if span <= 2*time.Hour {
		return "15:04:05"
	}
	if span > 24*time.Hour {
		return "01-02"
	}
	return "15:04"
}

func detailedXAxisTickCount(plotWidth int, layout string) int {
	labelWidth := len(time.Date(2026, 7, 2, 15, 4, 5, 0, time.Local).Format(layout))
	count := plotWidth / (labelWidth + 2)
	if count < 2 {
		return 2
	}
	if count > 9 {
		return 9
	}
	return count
}

func placeAxisTicks(labels []chartAxisLabel, plotWidth int, ascii bool) string {
	tick := '|'
	if !ascii {
		tick = '│'
	}
	fill := '-'
	if !ascii {
		fill = '─'
	}
	cells := make([]rune, plotWidth)
	for i := range cells {
		cells[i] = fill
	}
	for _, item := range labels {
		if item.Pos >= 0 && item.Pos < plotWidth {
			cells[item.Pos] = tick
		}
	}
	return string(cells)
}

func placeAxisLabels(labels []chartAxisLabel, plotWidth int) string {
	cells := make([]byte, plotWidth)
	for i := range cells {
		cells[i] = ' '
	}
	lastEnd := -1
	for _, item := range labels {
		if item.Label == "" {
			continue
		}
		start := item.Pos - displayWidth(item.Label)/2
		if start < 0 {
			start = 0
		}
		if end := start + displayWidth(item.Label); end > plotWidth {
			start = plotWidth - displayWidth(item.Label)
		}
		if start <= lastEnd+1 {
			start = lastEnd + 2
		}
		if start < 0 || start+displayWidth(item.Label) > plotWidth {
			continue
		}
		copy(cells[start:], item.Label)
		lastEnd = start + displayWidth(item.Label) - 1
	}
	return string(cells)
}

func appendStatusSample(records []komari.Status, current komari.Status) []komari.Status {
	if len(records) > 0 && sameStatusSample(records[len(records)-1], current) {
		records[len(records)-1] = current
		return records
	}
	return append(records, current)
}

func sameStatusSample(a, b komari.Status) bool {
	if a.Time.Valid && b.Time.Valid {
		return a.Time.Time.Equal(b.Time.Time)
	}
	return a.CPU == b.CPU &&
		a.RAM == b.RAM &&
		a.Disk == b.Disk &&
		a.NetIn == b.NetIn &&
		a.NetOut == b.NetOut
}

func realtimePingStatusValues(records []komari.Status, pick func(komari.Ping) float64) []float64 {
	values := make([]float64, 0, len(records))
	for _, record := range records {
		if len(record.Ping) == 0 {
			continue
		}
		var total float64
		var count int
		for _, ping := range record.Ping {
			total += pick(ping)
			count++
		}
		if count > 0 {
			values = append(values, total/float64(count))
		}
	}
	return values
}

func pingRecordValues(records []komari.PingRecord) []float64 {
	values := make([]float64, 0, len(records))
	for _, record := range records {
		if record.Value >= 0 {
			values = append(values, record.Value)
		}
	}
	return values
}

func pingTaskSparkLines(tasks []komari.PingTask, records []komari.PingRecord, ascii bool, width int, maxLines int) []string {
	if len(tasks) == 0 || len(records) == 0 || maxLines <= 0 {
		return nil
	}
	if width < 12 {
		width = 12
	}
	rows := pingChartRows(tasks, records)
	lines := make([]string, 0, min(len(tasks), maxLines))
	for _, task := range tasks {
		label := cleanLine(task.Name, 10)
		values := pingTaskRowValues(rows, task.ID)
		if len(values) == 0 {
			continue
		}
		sparkWidth := max(6, width-displayWidth(label)-8)
		latest := values[len(values)-1]
		line := fmt.Sprintf(" %-10s %s %4.0fms", label, sparklineLimit(values, ascii, sparkWidth), latest)
		lines = append(lines, line)
		if len(lines) >= maxLines {
			break
		}
	}
	return lines
}

type pingChartRow struct {
	Time   time.Time
	Values map[int]float64
}

func pingChartRows(tasks []komari.PingTask, records []komari.PingRecord) []pingChartRow {
	if len(records) == 0 {
		return nil
	}
	tolerance := pingAnchorTolerance(tasks)
	sorted := make([]komari.PingRecord, 0, len(records))
	for _, record := range records {
		if record.Time.Valid {
			sorted = append(sorted, record)
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Time.Time.Before(sorted[j].Time.Time)
	})
	rows := make([]pingChartRow, 0, len(sorted))
	for _, record := range sorted {
		anchorIndex := -1
		for i, row := range rows {
			delta := row.Time.Sub(record.Time.Time)
			if delta < 0 {
				delta = -delta
			}
			if delta <= tolerance {
				anchorIndex = i
				break
			}
		}
		if anchorIndex < 0 {
			rows = append(rows, pingChartRow{
				Time:   record.Time.Time,
				Values: map[int]float64{},
			})
			anchorIndex = len(rows) - 1
		}
		if record.Value >= 0 {
			rows[anchorIndex].Values[record.TaskID] = record.Value
		}
	}
	return rows
}

func pingAnchorTolerance(tasks []komari.PingTask) time.Duration {
	minInterval := 60 * time.Second
	for _, task := range tasks {
		if task.Interval <= 0 {
			continue
		}
		interval := time.Duration(task.Interval) * time.Second
		if interval < minInterval {
			minInterval = interval
		}
	}
	tolerance := minInterval / 4
	if tolerance < 800*time.Millisecond {
		return 800 * time.Millisecond
	}
	if tolerance > 6*time.Second {
		return 6 * time.Second
	}
	return tolerance
}

func pingTaskRowValues(rows []pingChartRow, taskID int) []float64 {
	values := make([]float64, 0, len(rows))
	for _, row := range rows {
		if value, ok := row.Values[taskID]; ok {
			values = append(values, value)
		}
	}
	return values
}

func sparkline(values []float64, ascii bool) string {
	if len(values) == 0 {
		return "-"
	}
	if len(values) > 48 {
		step := float64(len(values)) / 48
		downsampled := make([]float64, 0, 48)
		for i := 0; i < 48; i++ {
			downsampled = append(downsampled, values[int(float64(i)*step)])
		}
		values = downsampled
	}
	minVal, maxVal := values[0], values[0]
	for _, value := range values {
		if value < minVal {
			minVal = value
		}
		if value > maxVal {
			maxVal = value
		}
	}
	if maxVal == minVal {
		if ascii {
			return strings.Repeat("-", len(values))
		}
		return strings.Repeat("▁", len(values))
	}
	blocks := []rune("▁▂▃▄▅▆▇█")
	if ascii {
		blocks = []rune("._-=+*#@")
	}
	var b strings.Builder
	for _, value := range values {
		idx := int((value - minVal) / (maxVal - minVal) * float64(len(blocks)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		b.WriteRune(blocks[idx])
	}
	return b.String()
}

func sparklineLimit(values []float64, ascii bool, width int) string {
	if width <= 0 {
		return ""
	}
	if len(values) > width {
		step := float64(len(values)) / float64(width)
		downsampled := make([]float64, 0, width)
		for i := 0; i < width; i++ {
			downsampled = append(downsampled, values[int(float64(i)*step)])
		}
		values = downsampled
	}
	return cleanLine(sparkline(values, ascii), width)
}
