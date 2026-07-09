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
	section := a.metricSeriesChartSection(title, records, unit, []axisSeries{{Values: values}}, realtime)
	if section.Chart != nil {
		section.Chart.Values = values
		section.Chart.Series = nil
	}
	return section
}

func (a *App) metricSeriesChartSection(title string, records []komari.Status, unit string, series []axisSeries, realtime bool) detailSection {
	from := chartTimeLabel(firstRecordTime(records))
	to := chartTimeLabel(lastRecordTime(records))
	var window time.Duration
	var until time.Time
	if realtime {
		if last := lastRecordTime(records); last.Valid {
			window = a.realtimeWindowDuration()
			until = a.realtimeNowOrTime(last.Time)
			from = chartRealtimeTimeLabelFromTime(until.Add(-window))
			to = chartRealtimeTimeLabelFromTime(until)
		} else {
			from = chartRealtimeTimeLabel(firstRecordTime(records))
			to = chartRealtimeTimeLabel(lastRecordTime(records))
		}
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
			Series: series,
			Times:  statusTimes(records),
			From:   from,
			To:     to,
			Unit:   unit,
			Window: window,
			Until:  until,
		},
	}
}

func (a *App) percentChart(section detailSection) detailSection {
	if section.Chart != nil {
		section.Chart.FixedRange = a.chartYAxisMode == chartYAxisAbsolute
		if section.Chart.FixedRange {
			section.Chart.Min = 0
			section.Chart.Max = 100
		}
	}
	return section
}

func (a *App) axisChartLines(chart axisChart, width int, height int) []string {
	return a.axisChartLinesWithOptions(chart, width, height, false)
}

func (a *App) axisChartLinesDetailed(chart axisChart, width int, height int) []string {
	return a.axisChartLinesWithOptions(chart, width, height, true)
}

func (a *App) axisChartLinesWithOptions(chart axisChart, width int, height int, detailedXAxis bool) []string {
	if height <= 0 {
		return nil
	}
	series := chartSeriesList(chart)
	if !chartHasValues(series) {
		return limitRawLines([]string{" no samples"}, height)
	}
	if height < 4 || width < 18 {
		lines := make([]string, 0, height)
		for _, item := range series {
			lines = append(lines, " "+sparklineLimit(item.Values, a.style.ASCII, max(1, width-1)))
			if len(lines) >= height-1 {
				break
			}
		}
		if detailedXAxis {
			lines = append(lines, fitLine(" "+chartDetailedXAxisLabel(chart), width))
		} else {
			lines = append(lines, fmt.Sprintf(" %s -> %s", chart.From, chart.To))
		}
		return limitRawLines(lines, height)
	}

	minVal, maxVal := chartSeriesMinMax(series)
	if chart.FixedRange {
		minVal, maxVal = chart.Min, chart.Max
	}
	midVal := (minVal + maxVal) / 2
	labelWidth := chartLabelWidth(minVal, midVal, maxVal, chart.Unit)
	plotWidth := max(1, width-labelWidth-1)
	seriesPoints := make([][]chartPoint, 0, len(series))
	for _, item := range series {
		seriesPoints = append(seriesPoints, chartPointsForValues(chart, item.Values, plotWidth))
	}
	axisRows := 1
	if detailedXAxis {
		axisRows = 2
	}
	plotRows := max(1, height-axisRows)
	rows := chartYAxisRows(minVal, maxVal, plotRows, chart.Unit, labelWidth)

	out := make([]string, 0, height)
	for rowIndex, row := range rows {
		var b strings.Builder
		b.WriteString(row.label)
		if a.style.ASCII {
			b.WriteString("|")
		} else {
			b.WriteString("│")
		}
		for col := 0; col < plotWidth; col++ {
			mark := rune(0)
			for seriesIndex, points := range seriesPoints {
				if col >= len(points) {
					continue
				}
				point := points[col]
				if point.Valid && chartPointRow(point.Value, minVal, maxVal, plotRows) == rowIndex {
					next := chartSeriesMark(seriesIndex, a.style.ASCII)
					if mark != 0 && mark != next {
						mark = chartOverlapMark(a.style.ASCII)
					} else {
						mark = next
					}
				}
			}
			if mark != 0 {
				b.WriteRune(mark)
			} else if rowIndex == len(rows)-1 {
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

	out = append(out, chartXAxisLines(chart, width, labelWidth, a.style.ASCII, detailedXAxis)...)
	return limitRawLines(out, height)
}

type chartYAxisRow struct {
	label string
}

func chartYAxisRows(minVal, maxVal float64, count int, unit string, labelWidth int) []chartYAxisRow {
	if count <= 0 {
		return nil
	}
	rows := make([]chartYAxisRow, 0, count)
	for i := 0; i < count; i++ {
		value := maxVal
		if count > 1 {
			value = maxVal - (maxVal-minVal)*float64(i)/float64(count-1)
		}
		rows = append(rows, chartYAxisRow{
			label: chartValueLabel(value, unit, labelWidth),
		})
	}
	return rows
}

func chartSeriesList(chart axisChart) []axisSeries {
	if len(chart.Series) > 0 {
		return chart.Series
	}
	return []axisSeries{{Values: chart.Values}}
}

func chartHasValues(series []axisSeries) bool {
	for _, item := range series {
		if len(item.Values) > 0 {
			return true
		}
	}
	return false
}

func chartSeriesMinMax(series []axisSeries) (float64, float64) {
	var minVal, maxVal float64
	var ok bool
	for _, item := range series {
		for _, value := range item.Values {
			if !ok {
				minVal, maxVal = value, value
				ok = true
				continue
			}
			if value < minVal {
				minVal = value
			}
			if value > maxVal {
				maxVal = value
			}
		}
	}
	if !ok {
		return 0, 0
	}
	return minVal, maxVal
}

func chartSeriesMark(index int, ascii bool) rune {
	if ascii {
		marks := []rune{'*', '+', 'x', 'o'}
		return marks[index%len(marks)]
	}
	marks := []rune{'•', '×', '◆', '○'}
	return marks[index%len(marks)]
}

func chartOverlapMark(ascii bool) rune {
	if ascii {
		return '#'
	}
	return '◆'
}

func (a *App) historyMetricSections(node komari.Node, st komari.Status, records []komari.Status, label string) []detailSection {
	sum := summarizeStatusWithTotals(records, node.MemTotal, node.DiskTotal)
	realtime := label == "Realtime"
	sections := []detailSection{
		a.percentChart(a.metricChartSection("CPU Chart", records, "%", statusValues(records, func(st komari.Status) float64 { return st.CPU }), realtime)),
		a.memoryChartSection(node, records, realtime),
		a.percentChart(a.metricChartSection("Disk Chart", records, "%", statusDiskPercentValues(records, node.DiskTotal), realtime)),
	}
	sections = append(sections, a.networkChartSections(records, realtime)...)
	sections = append(sections,
		a.metricChartSection("Connections Chart", records, "", statusValues(records, func(st komari.Status) float64 { return float64(st.Connections) }), realtime),
		a.metricChartSection("Process Chart", records, "", statusValues(records, func(st komari.Status) float64 { return float64(st.Process) }), realtime),
		detailSection{
			Title: label + " Load",
			Lines: []string{
				fmt.Sprintf(" CPU  avg %.1f  max %.1f", sum.CPUAvg, sum.CPUMax),
				fmt.Sprintf(" RAM  avg %.1f  max %.1f", sum.RAMAvg, sum.RAMMax),
				fmt.Sprintf(" Disk avg %.1f  max %.1f", sum.DiskAvg, sum.DiskMax),
				fmt.Sprintf(" Load avg %.2f max %.2f", sum.LoadAvg, sum.LoadMax),
			},
		},
		detailSection{
			Title: "Network",
			Lines: []string{
				fmt.Sprintf(" Out avg %s", speedIEC(int64(sum.NetOutAvg))),
				fmt.Sprintf(" Out max %s", speedIEC(sum.NetOutMax)),
				fmt.Sprintf(" In  avg %s", speedIEC(int64(sum.NetInAvg))),
				fmt.Sprintf(" In  max %s", speedIEC(sum.NetInMax)),
			},
		},
		detailSection{
			Title: "Runtime",
			Lines: []string{
				fmt.Sprintf(" Conn avg %.0f max %d", sum.ConnectionsAvg, sum.ConnectionsMax),
				fmt.Sprintf(" Conn now TCP %d UDP %d", st.Connections, st.ConnectionsUDP),
				fmt.Sprintf(" Proc avg %.0f max %d", sum.ProcessAvg, sum.ProcessMax),
				fmt.Sprintf(" Proc now %d", st.Process),
			},
		},
		detailSection{
			Title: "Current",
			Lines: []string{
				fmt.Sprintf(" Samples %d", len(records)),
				fmt.Sprintf(" Seen    %s", timeText(st.Time)),
				fmt.Sprintf(" Net now %s %s", a.style.up(), speedIEC(st.NetOut)),
				fmt.Sprintf("         %s %s", a.style.down(), speedIEC(st.NetIn)),
			},
		},
	)
	return sections
}

func (a *App) memoryChartSection(node komari.Node, records []komari.Status, realtime bool) detailSection {
	return a.percentChart(a.metricSeriesChartSection("RAM Chart", records, "%", []axisSeries{
		{Name: "RAM", Values: statusRAMPercentValues(records, node.MemTotal)},
		{Name: "Swap", Values: statusSwapPercentValues(records, node.SwapTotal)},
	}, realtime))
}

func (a *App) networkChartSections(records []komari.Status, realtime bool) []detailSection {
	return []detailSection{
		a.metricChartSection("Network In Chart", records, "B/s", statusValues(records, func(st komari.Status) float64 { return float64(st.NetIn) }), realtime),
		a.metricChartSection("Network Out Chart", records, "B/s", statusValues(records, func(st komari.Status) float64 { return float64(st.NetOut) }), realtime),
	}
}
