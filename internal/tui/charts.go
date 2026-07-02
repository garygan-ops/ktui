package tui

import (
	"fmt"
	"strings"
	"time"

	"ktui/internal/komari"
)

func (a *App) historyChartSection(title string, records []komari.Status, unit string, values []float64) detailSection {
	return a.metricChartSection(title, records, unit, values, false)
}

func (a *App) metricChartSection(title string, records []komari.Status, unit string, values []float64, realtime bool) detailSection {
	from := chartTimeLabel(firstRecordTime(records))
	to := chartTimeLabel(lastRecordTime(records))
	var window time.Duration
	var until time.Time
	if realtime {
		from = chartRealtimeTimeLabel(firstRecordTime(records))
		to = chartRealtimeTimeLabel(lastRecordTime(records))
	} else if detailWindows[a.window].Hours > 0 {
		if last := lastRecordTime(records); last.Valid {
			window = time.Duration(detailWindows[a.window].Hours) * time.Hour
			until = last.Time
			from = chartTimeLabelFromTime(until.Add(-window))
			to = chartTimeLabelFromTime(until)
		}
	}
	return detailSection{
		Title: title,
		Chart: &axisChart{
			Values: values,
			Times:  statusTimes(records),
			From:   from,
			To:     to,
			Unit:   unit,
			Window: window,
			Until:  until,
		},
	}
}

func (a *App) axisChartLines(chart axisChart, width int, height int) []string {
	if height <= 0 {
		return nil
	}
	if len(chart.Values) == 0 {
		return limitRawLines([]string{" no samples"}, height)
	}
	if height < 4 || width < 18 {
		return limitRawLines([]string{
			" " + sparklineLimit(chart.Values, a.style.ASCII, max(1, width-1)),
			fmt.Sprintf(" %s -> %s", chart.From, chart.To),
		}, height)
	}

	minVal, maxVal := minMaxFloat(chart.Values)
	midVal := (minVal + maxVal) / 2
	labelWidth := chartLabelWidth(minVal, midVal, maxVal, chart.Unit)
	plotWidth := max(1, width-labelWidth-1)
	points := chartPoints(chart, plotWidth)
	rows := []struct {
		value float64
		label string
	}{
		{maxVal, chartValueLabel(maxVal, chart.Unit, labelWidth)},
		{midVal, chartValueLabel(midVal, chart.Unit, labelWidth)},
		{minVal, chartValueLabel(minVal, chart.Unit, labelWidth)},
	}

	out := make([]string, 0, height)
	for rowIndex, row := range rows {
		var b strings.Builder
		b.WriteString(row.label)
		if a.style.ASCII {
			b.WriteString("|")
		} else {
			b.WriteString("│")
		}
		for _, point := range points {
			if point.Valid && chartPointRow(point.Value, minVal, maxVal) == rowIndex {
				if a.style.ASCII {
					b.WriteByte('*')
				} else {
					b.WriteRune('•')
				}
			} else if rowIndex == 2 {
				if a.style.ASCII {
					b.WriteByte('-')
				} else {
					b.WriteRune('─')
				}
			} else {
				b.WriteByte(' ')
			}
		}
		out = append(out, fitLine(b.String(), width))
	}

	axis := chartXAxisLine(chart.From, chart.To, width, labelWidth, a.style.ASCII)
	out = append(out, axis)
	return limitRawLines(out, height)
}

func (a *App) historyMetricSections(node komari.Node, st komari.Status, records []komari.Status, label string) []detailSection {
	sum := summarizeStatusWithTotals(records, node.MemTotal, node.DiskTotal)
	realtime := label == "Realtime"
	return []detailSection{
		a.metricChartSection("CPU Chart", records, "%", statusValues(records, func(st komari.Status) float64 { return st.CPU }), realtime),
		a.metricChartSection("RAM Chart", records, "%", statusRAMPercentValues(records, node.MemTotal), realtime),
		a.metricChartSection("Disk Chart", records, "%", statusDiskPercentValues(records, node.DiskTotal), realtime),
		a.networkChartSection(records),
		a.metricChartSection("Connections Chart", records, "", statusValues(records, func(st komari.Status) float64 { return float64(st.Connections) }), realtime),
		a.metricChartSection("Process Chart", records, "", statusValues(records, func(st komari.Status) float64 { return float64(st.Process) }), realtime),
		{
			Title: label + " Load",
			Lines: []string{
				fmt.Sprintf(" CPU  avg %.1f  max %.1f", sum.CPUAvg, sum.CPUMax),
				fmt.Sprintf(" RAM  avg %.1f  max %.1f", sum.RAMAvg, sum.RAMMax),
				fmt.Sprintf(" Disk avg %.1f  max %.1f", sum.DiskAvg, sum.DiskMax),
				fmt.Sprintf(" Load avg %.2f max %.2f", sum.LoadAvg, sum.LoadMax),
			},
		},
		{
			Title: "Network",
			Lines: []string{
				fmt.Sprintf(" Out avg %s", speedIEC(int64(sum.NetOutAvg))),
				fmt.Sprintf(" Out max %s", speedIEC(sum.NetOutMax)),
				fmt.Sprintf(" In  avg %s", speedIEC(int64(sum.NetInAvg))),
				fmt.Sprintf(" In  max %s", speedIEC(sum.NetInMax)),
			},
		},
		{
			Title: "Runtime",
			Lines: []string{
				fmt.Sprintf(" Conn avg %.0f max %d", sum.ConnectionsAvg, sum.ConnectionsMax),
				fmt.Sprintf(" Conn now TCP %d UDP %d", st.Connections, st.ConnectionsUDP),
				fmt.Sprintf(" Proc avg %.0f max %d", sum.ProcessAvg, sum.ProcessMax),
				fmt.Sprintf(" Proc now %d", st.Process),
			},
		},
		{
			Title: "Current",
			Lines: []string{
				fmt.Sprintf(" Samples %d", len(records)),
				fmt.Sprintf(" Seen    %s", timeText(st.Time)),
				fmt.Sprintf(" Net now %s %s", a.style.up(), speedIEC(st.NetOut)),
				fmt.Sprintf("         %s %s", a.style.down(), speedIEC(st.NetIn)),
			},
		},
	}
}

func (a *App) networkChartSection(records []komari.Status) detailSection {
	outValues := statusValues(records, func(st komari.Status) float64 { return float64(st.NetOut) })
	inValues := statusValues(records, func(st komari.Status) float64 { return float64(st.NetIn) })
	return detailSection{
		Title: "Network Chart",
		Lines: []string{
			" Out " + sparklineLimit(outValues, a.style.ASCII, 28),
			" In  " + sparklineLimit(inValues, a.style.ASCII, 28),
			fmt.Sprintf(" Out now %s", speedIEC(lastStatusInt64(records, func(st komari.Status) int64 { return st.NetOut }))),
			fmt.Sprintf(" In  now %s", speedIEC(lastStatusInt64(records, func(st komari.Status) int64 { return st.NetIn }))),
		},
	}
}

func lastStatusInt64(records []komari.Status, pick func(komari.Status) int64) int64 {
	if len(records) == 0 {
		return 0
	}
	return pick(records[len(records)-1])
}
