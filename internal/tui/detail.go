package tui

import (
	"fmt"
	"strings"
	"time"

	"ktui/internal/komari"
)

func (a *App) renderDetailBody(width int, bodyHeight int) []string {
	if bodyHeight <= 0 {
		return nil
	}
	node, ok := a.selectedNode()
	if !ok {
		if a.loading {
			return fillBody([]string{"Loading nodes..."}, width, bodyHeight)
		}
		return fillBody([]string{"No nodes returned by Komari."}, width, bodyHeight)
	}

	a.clampSelection()
	st := a.snapshot.Status[node.UUID]
	if a.chartFocus {
		return a.renderFocusedChartBody(node, st, width, bodyHeight)
	}
	chrome := a.detailChromeLines(node, st, width)
	contentHeight := bodyHeight - len(chrome)
	if contentHeight < 1 {
		contentHeight = 1
		if len(chrome) > bodyHeight-1 {
			chrome = chrome[:max(0, bodyHeight-1)]
		}
	}
	cardHeight := detailCardHeightFor(contentHeight)
	a.cardStep = cardHeight
	if contentHeight >= cardHeight {
		contentHeight = contentHeight / cardHeight * cardHeight
	}

	content := a.detailContentLines(node, st, width, cardHeight)
	if contentHeight >= cardHeight {
		a.scroll = a.scroll / cardHeight * cardHeight
	}
	maxScroll := max(0, len(content)-contentHeight)
	if contentHeight >= cardHeight {
		maxScroll = maxScroll / cardHeight * cardHeight
	}
	if a.scroll > maxScroll {
		a.scroll = maxScroll
	}
	if a.scroll < 0 {
		a.scroll = 0
	}
	end := min(len(content), a.scroll+contentHeight)
	lines := make([]string, 0, bodyHeight)
	lines = append(lines, chrome...)
	lines = append(lines, content[a.scroll:end]...)
	lines = fillBody(lines, width, bodyHeight)
	return a.withScrollIndicator(lines, width, scrollIndicator{
		Start:   len(chrome),
		Height:  contentHeight,
		Offset:  a.scroll,
		Visible: contentHeight,
		Total:   len(content),
	})
}

func (a *App) renderFocusedChartBody(node komari.Node, st komari.Status, width int, bodyHeight int) []string {
	sections := a.chartSections(node, st)
	if len(sections) == 0 {
		return a.renderFocusedChartPlaceholder(node, width, bodyHeight)
	}
	a.clampChartFocus(len(sections))
	section := sections[a.chartFocusIndex]
	title := fmt.Sprintf(" %s  %s  %s", section.Title, a.nodeLabel(node), detailWindows[a.window].Label)
	lines := []string{
		a.style.bold(fitLine(title, width)),
		a.style.dim(fitLine(" Esc/b/q back   h/l previous/next chart   [ ] window   r refresh", width)),
	}
	chartHeight := bodyHeight - len(lines)
	if chartHeight < 1 {
		chartHeight = 1
	}
	lines = append(lines, a.axisChartLinesDetailed(*section.Chart, width, chartHeight)...)
	return fillBody(lines, width, bodyHeight)
}

func (a *App) renderFocusedChartPlaceholder(node komari.Node, width int, bodyHeight int) []string {
	title := fmt.Sprintf(" Loading charts  %s  %s", a.nodeLabel(node), detailWindows[a.window].Label)
	lines := []string{
		a.style.bold(fitLine(title, width)),
		a.style.dim(fitLine(" Esc/b/q back   [ ] window   r refresh", width)),
	}
	detail := a.currentDetail(node.UUID)
	message := "Loading records..."
	switch {
	case detail.Err != nil:
		message = "Error loading records: " + detail.Err.Error()
	case !detail.Loading:
		if detailWindows[a.window].Hours == 0 {
			message = "Waiting for realtime samples..."
		} else {
			message = "No chart data yet. Press d to load details."
		}
	}
	lines = append(lines,
		"",
		" "+message,
		" Focus mode will stay open while data loads.",
	)
	return fillBody(lines, width, bodyHeight)
}

func (a *App) detailChromeLines(node komari.Node, st komari.Status, width int) []string {
	lines := make([]string, 0, 6)
	alert := a.alertForNode(node, st, time.Now())
	title := fmt.Sprintf(" %s  %s  %s  region %s  seen %s",
		a.text(node.Name),
		a.statusPill(st.Online),
		durationCompact(st.Uptime),
		valueOr(a.text(node.Region), "-"),
		shortTimeFromNull(st.Time),
	)
	lines = append(lines, a.style.bold(fitLine(title, width)))
	if len(alert.Reasons) > 0 {
		lines = append(lines, a.styleAlertLine(fitLine(" WARN "+alertText(alert), width), alert))
	}
	lines = append(lines, a.detailMetricStrip(node, st, width))
	lines = append(lines, a.detailTabLine(width))
	lines = append(lines, a.detailWindowLine(width))
	return lines
}

func (a *App) detailMetricStrip(node komari.Node, st komari.Status, width int) string {
	ramPct := percent(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal))
	diskPct := percent(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal))
	barWidth := 6
	if width >= 100 {
		barWidth = 12
	} else if width >= 80 {
		barWidth = 8
	}
	parts := []string{
		fmt.Sprintf(" CPU %5.1f%% %s", st.CPU, a.usageBarFor("CPU", st.CPU, barWidth)),
		fmt.Sprintf(" RAM %5.1f%% %s", ramPct, a.usageBarFor("RAM", ramPct, barWidth)),
		fmt.Sprintf(" DSK %5.1f%% %s", diskPct, a.usageBarFor("DSK", diskPct, barWidth)),
	}
	if width >= 104 {
		parts = append(parts, fmt.Sprintf(" NET %s %s %s %s", a.style.up(), speedIEC(st.NetOut), a.style.down(), speedIEC(st.NetIn)))
	}
	if width >= 128 && node.TrafficLimit > 0 {
		pct := trafficPercent(st.NetTotalUp, st.NetTotalDown, node.TrafficLimit, node.TrafficLimitType)
		parts = append(parts, fmt.Sprintf(" TRF %5.1f%% %s", pct, a.usageBarFor("TRF", pct, barWidth)))
	}
	return fitLine(strings.Join(parts, "  "), width)
}

func (a *App) detailTabLine(width int) string {
	parts := make([]string, 0, len(tabNames))
	for i, name := range tabNames {
		label := fmt.Sprintf(" %d %s ", i+1, name)
		if i == a.tab {
			if a.style.NoColor {
				label = "[" + strings.TrimSpace(label) + "]"
			} else {
				label = a.style.inverse(label)
			}
		} else {
			label = a.style.dim(label)
		}
		parts = append(parts, label)
	}
	return fitLine(strings.Join(parts, " "), width)
}

func (a *App) detailWindowLine(width int) string {
	parts := make([]string, 0, len(detailWindows)+1)
	parts = append(parts, a.style.dim(" window "))
	for i, window := range detailWindows {
		label := " " + window.Label + " "
		if i == a.window {
			if a.style.NoColor {
				label = "[" + window.Label + "]"
			} else {
				label = a.style.cyan(label)
			}
		} else {
			label = a.style.dim(label)
		}
		parts = append(parts, label)
	}
	return fitLine(strings.Join(parts, " "), width)
}

func (a *App) detailContentLines(node komari.Node, st komari.Status, width int, cardHeight int) []string {
	sections := a.detailSections(node, st)
	if len(sections) == 0 {
		return []string{fitLine("", width)}
	}
	return a.detailSectionGrid(sections, width, detailGridColumns(width), cardHeight)
}

func (a *App) detailSectionGrid(sections []detailSection, width int, columns int, cardHeight int) []string {
	if cardHeight < detailCardHeight {
		cardHeight = detailCardHeight
	}
	columns, cardWidth := detailGridLayout(width, columns)

	lines := make([]string, 0, len(sections)*(cardHeight+1))
	for row := 0; row < len(sections); row += columns {
		rowCards := make([][]string, 0, columns)
		for col := 0; col < columns; col++ {
			index := row + col
			if index >= len(sections) {
				rowCards = append(rowCards, emptyCard(cardWidth, cardHeight))
				continue
			}
			rowCards = append(rowCards, a.detailSectionCard(sections[index], cardWidth, cardHeight))
		}
		for lineIndex := 0; lineIndex < cardHeight; lineIndex++ {
			var line strings.Builder
			for col, card := range rowCards {
				if col > 0 {
					line.WriteString(strings.Repeat(" ", detailGridGap))
				}
				line.WriteString(card[lineIndex])
			}
			lines = append(lines, fitLine(line.String(), width))
		}
	}
	return lines
}

const detailGridGap = 2

func detailGridColumns(width int) int {
	if width >= 96 {
		return 2
	}
	return 1
}

func detailGridLayout(width int, columns int) (int, int) {
	if columns < 1 {
		columns = 1
	}
	for columns > 1 {
		cardWidth := (width - detailGridGap*(columns-1)) / columns
		if cardWidth >= 42 {
			break
		}
		columns--
	}
	cardWidth := width
	if columns > 1 {
		cardWidth = (width - detailGridGap*(columns-1)) / columns
	}
	return columns, cardWidth
}

func detailCardHeightFor(contentHeight int) int {
	switch {
	case contentHeight >= 45:
		return 15
	case contentHeight >= 28:
		return 10
	case contentHeight >= 21:
		return 9
	default:
		return detailCardHeight
	}
}

func (a *App) detailSectionCard(section detailSection, width int, height int) []string {
	if height < 4 {
		height = 4
	}
	lines := make([]string, 0, height)
	lines = append(lines, a.cardTop(width, false))
	title := " " + a.style.bold(section.Title)
	lines = append(lines, a.style.boxLine(title, width))
	contentHeight := height - 3
	contentLines := section.Lines
	if section.Chart != nil {
		contentLines = a.axisChartLines(*section.Chart, width-2, contentHeight)
	}
	for i := 0; i < contentHeight; i++ {
		content := ""
		if i < len(contentLines) {
			content = contentLines[i]
		}
		lines = append(lines, a.style.boxLine(content, width))
	}
	lines = append(lines, a.cardBottom(width, false))
	return lines
}

func (a *App) detailSections(node komari.Node, st komari.Status) []detailSection {
	switch a.tab {
	case 1:
		return a.nodeInfoSections(node, st)
	case 2:
		return a.historySections(node, st)
	case 3:
		return a.pingSections(node, st)
	case 4:
		return a.metaSections(node, st)
	default:
		return a.overviewSections(node, st)
	}
}

func (a *App) chartSections(node komari.Node, st komari.Status) []detailSection {
	sections := a.detailSections(node, st)
	charts := make([]detailSection, 0, len(sections))
	for _, section := range sections {
		if section.Chart != nil {
			charts = append(charts, section)
		}
	}
	return charts
}

func (a *App) focusChart(index int) bool {
	node, ok := a.selectedNode()
	if !a.detail || !ok {
		return false
	}
	st := a.snapshot.Status[node.UUID]
	charts := a.chartSections(node, st)
	if len(charts) == 0 {
		return false
	}
	if index < 0 {
		index = 0
	}
	if index >= len(charts) {
		index = len(charts) - 1
	}
	a.chartFocus = true
	a.chartFocusIndex = index
	a.scroll = 0
	return true
}

func (a *App) closeChartFocus() {
	a.chartFocus = false
	a.chartFocusIndex = 0
}

func (a *App) moveChartFocus(delta int) {
	node, ok := a.selectedNode()
	if !a.chartFocus || !ok {
		return
	}
	st := a.snapshot.Status[node.UUID]
	charts := a.chartSections(node, st)
	if len(charts) == 0 {
		return
	}
	a.chartFocusIndex = (a.chartFocusIndex + delta + len(charts)) % len(charts)
}

func (a *App) clampChartFocus(count int) {
	if count <= 0 {
		a.closeChartFocus()
		return
	}
	if a.chartFocusIndex < 0 {
		a.chartFocusIndex = 0
	}
	if a.chartFocusIndex >= count {
		a.chartFocusIndex = count - 1
	}
}

func (a *App) overviewSections(node komari.Node, st komari.Status) []detailSection {
	ramTotal := firstNonZero(st.RAMTotal, node.MemTotal)
	diskTotal := firstNonZero(st.DiskTotal, node.DiskTotal)
	swapTotal := firstNonZero(st.SwapTotal, node.SwapTotal)
	ramPct := percent(st.RAM, ramTotal)
	diskPct := percent(st.Disk, diskTotal)
	swapPct := percent(st.Swap, swapTotal)
	sections := []detailSection{
		{
			Title: "Health",
			Lines: []string{
				fmt.Sprintf(" Status  %s", a.style.coloredStatus(st.Online)),
				fmt.Sprintf(" Seen    %s", timeText(st.Time)),
				fmt.Sprintf(" Uptime  %s", durationCompact(st.Uptime)),
				fmt.Sprintf(" Load    %.2f %.2f %.2f", st.Load, st.Load5, st.Load15),
				fmt.Sprintf(" Proc    %d", st.Process),
			},
		},
		{
			Title: "Resources",
			Lines: []string{
				a.detailUsageLine(" CPU", st.CPU, fmt.Sprintf("%.1f%%", st.CPU)),
				a.detailUsageLine(" RAM", ramPct, usageCompact(st.RAM, ramTotal)),
				a.detailUsageLine(" Disk", diskPct, usageCompact(st.Disk, diskTotal)),
				a.detailUsageLine(" Swap", swapPct, usageCompact(st.Swap, swapTotal)),
			},
		},
		{
			Title: "Network",
			Lines: []string{
				fmt.Sprintf(" Now     %s %s", a.style.up(), speedIEC(st.NetOut)),
				fmt.Sprintf("         %s %s", a.style.down(), speedIEC(st.NetIn)),
				fmt.Sprintf(" Total   %s %s", a.style.up(), bytesIEC(st.NetTotalUp)),
				fmt.Sprintf("         %s %s", a.style.down(), bytesIEC(st.NetTotalDown)),
				fmt.Sprintf(" Conn    TCP %d  UDP %d", st.Connections, st.ConnectionsUDP),
			},
		},
		{
			Title: "Placement",
			Lines: []string{
				fmt.Sprintf(" Region  %s", valueOr(a.text(node.Region), "-")),
				fmt.Sprintf(" Group   %s", valueOr(a.text(node.Group), "-")),
				fmt.Sprintf(" Tags    %s", valueOr(a.text(node.Tags), "-")),
				fmt.Sprintf(" OS      %s", valueOr(a.text(node.OS), "-")),
			},
		},
	}
	if node.TrafficLimit > 0 {
		pct := trafficPercent(st.NetTotalUp, st.NetTotalDown, node.TrafficLimit, node.TrafficLimitType)
		sections = append(sections, detailSection{
			Title: "Total Traffic",
			Lines: []string{
				a.detailUsageLine(" Used", pct, fmt.Sprintf("%.1f%%", pct)),
				fmt.Sprintf(" Flow    %s %s  %s %s", a.style.up(), trafficBytes(st.NetTotalUp), a.style.down(), trafficBytes(st.NetTotalDown)),
				fmt.Sprintf(" Limit   %s", trafficLimitText(node.TrafficLimit, node.TrafficLimitType)),
			},
		})
	}
	return sections
}

func (a *App) nodeInfoSections(node komari.Node, st komari.Status) []detailSection {
	sections := []detailSection{
		{
			Title: "System",
			Lines: []string{
				fmt.Sprintf(" OS      %s", valueOr(a.text(node.OS), "-")),
				fmt.Sprintf(" Arch    %s", valueOr(a.text(node.Arch), "-")),
				fmt.Sprintf(" Kernel  %s", valueOr(a.text(node.KernelVersion), "-")),
				fmt.Sprintf(" Agent   %s", valueOr(a.text(node.Version), "-")),
				fmt.Sprintf(" Virt    %s", valueOr(a.text(node.Virtualization), "-")),
			},
		},
		{
			Title: "Hardware",
			Lines: []string{
				fmt.Sprintf(" CPU     %s", valueOr(a.text(node.CPUName), "-")),
				fmt.Sprintf(" Cores   %s", coreText(node)),
				fmt.Sprintf(" GPU     %s", valueOr(a.text(node.GPUName), "-")),
				fmt.Sprintf(" Memory  %s", bytesIEC(node.MemTotal)),
				fmt.Sprintf(" Disk    %s", bytesIEC(node.DiskTotal)),
			},
		},
		{
			Title: "Live Load",
			Lines: []string{
				a.detailUsageLine(" CPU", st.CPU, fmt.Sprintf("%.1f%%", st.CPU)),
				a.detailUsageLine(" RAM", percent(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal)), usageCompact(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal))),
				a.detailUsageLine(" Disk", percent(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal)), usageCompact(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal))),
				fmt.Sprintf(" Temp    %.1f", st.Temp),
			},
		},
	}
	if node.Price != 0 || node.ExpiredAt.Valid || node.PublicRemark != "" || node.Remark != "" {
		price := "-"
		if node.Price > 0 {
			price = fmt.Sprintf("%s%.2f / %dd", valueOr(node.Currency, ""), node.Price, node.BillingCycle)
		} else if node.Price < 0 {
			price = "free"
		}
		sections = append(sections, detailSection{
			Title: "Billing",
			Lines: []string{
				fmt.Sprintf(" Price   %s", price),
				fmt.Sprintf(" Renew   %t", node.AutoRenewal),
				fmt.Sprintf(" Expire  %s", nullableDate(node.ExpiredAt)),
				fmt.Sprintf(" Public  %s", valueOr(a.text(node.PublicRemark), "-")),
				fmt.Sprintf(" Remark  %s", valueOr(a.text(node.Remark), "-")),
			},
		})
	}
	return sections
}

func (a *App) historySections(node komari.Node, st komari.Status) []detailSection {
	window := detailWindows[a.window]
	detail := a.currentDetail(node.UUID)
	infoSections := []detailSection{
		{
			Title: "Window",
			Lines: []string{
				fmt.Sprintf(" Range   %s", window.Label),
				fmt.Sprintf(" Records load %d recent %d", len(detail.Load.Records), len(detail.Recent.Records)),
				fmt.Sprintf(" From    %s", nullableDateTime(detail.Load.From)),
				fmt.Sprintf(" To      %s", nullableDateTime(detail.Load.To)),
			},
		},
	}
	if detail.Loading {
		infoSections = append(infoSections, detailSection{
			Title: "Status",
			Lines: []string{" Loading records...", " Press d to force reload."},
		})
	}
	if detail.Err != nil {
		infoSections = append(infoSections, detailSection{
			Title: "Error",
			Lines: []string{" " + detail.Err.Error(), " Press d to retry."},
		})
	}
	if window.Hours == 0 {
		records := a.realtimeRecords(node.UUID, detail.Recent.Records, st)
		sections := a.historyMetricSections(node, st, records, "Realtime")
		return append(sections, infoSections...)
	}
	if len(detail.Load.Records) == 0 {
		return append(infoSections, detailSection{
			Title: "History",
			Lines: []string{" No load history yet.", " Press d to load details."},
		})
	}

	records := loadChartRecords(detail.Load.Records, window.Hours)
	sections := a.historyMetricSections(node, st, records, "History")
	return append(sections, infoSections...)
}

func (a *App) pingSections(node komari.Node, st komari.Status) []detailSection {
	window := detailWindows[a.window]
	detail := a.currentDetail(node.UUID)
	sections := []detailSection{
		{
			Title: "Window",
			Lines: []string{
				fmt.Sprintf(" Range   %s", window.Label),
				fmt.Sprintf(" Tasks   %d", len(detail.Ping.Tasks)),
				fmt.Sprintf(" Records %d", len(detail.Ping.Records)),
				fmt.Sprintf(" From    %s", nullableDateTime(detail.Ping.From)),
				fmt.Sprintf(" To      %s", nullableDateTime(detail.Ping.To)),
			},
		},
	}
	realtime := a.realtimePingLines(st, 5)
	if len(realtime) == 0 {
		realtime = []string{" No realtime ping data."}
	}
	sections = append(sections, detailSection{Title: "Realtime Ping", Lines: realtime})
	if window.Hours == 0 {
		records := a.realtimeRecords(node.UUID, detail.Recent.Records, st)
		if values := realtimePingStatusValues(records, func(ping komari.Ping) float64 { return ping.Latest }); len(values) > 0 {
			sections = append(sections, a.sparkSection("Latency Spark", values))
		}
		if values := realtimePingStatusValues(records, func(ping komari.Ping) float64 { return ping.Loss }); len(values) > 0 {
			sections = append(sections, a.sparkSection("Loss Spark", values))
		}
		return sections
	}
	if detail.Loading {
		sections = append(sections, detailSection{
			Title: "Status",
			Lines: []string{" Loading ping records...", " Press d to force reload."},
		})
	}
	if detail.Err != nil {
		sections = append(sections, detailSection{
			Title: "Error",
			Lines: []string{" " + detail.Err.Error(), " Press d to retry."},
		})
	}
	if len(detail.Ping.Tasks) == 0 && len(detail.Ping.Records) == 0 {
		return append(sections, detailSection{
			Title: "History Ping",
			Lines: []string{" No ping history yet.", " Press d to load details."},
		})
	}
	if len(detail.Ping.Tasks) > 0 {
		taskLines := make([]string, 0, len(detail.Ping.Tasks))
		for _, task := range detail.Ping.Tasks {
			taskLines = append(taskLines, fmt.Sprintf(" %-10s avg %4.0f min %4.0f max %4.0f",
				cleanLine(a.text(task.Name), 10), task.Avg, task.Min, task.Max))
			if len(taskLines) >= 5 {
				break
			}
		}
		sections = append(sections, detailSection{Title: "History Ping", Lines: taskLines})
		lossLines := make([]string, 0, len(detail.Ping.Tasks))
		for _, task := range detail.Ping.Tasks {
			lossLines = append(lossLines, fmt.Sprintf(" %-10s loss %.1f%% total %d", cleanLine(a.text(task.Name), 10), task.Loss, task.Total))
			if len(lossLines) >= 5 {
				break
			}
		}
		sections = append(sections, detailSection{Title: "Loss", Lines: lossLines})
	} else {
		sections = append(sections, detailSection{
			Title: "History Ping",
			Lines: []string{
				fmt.Sprintf(" Records %d", len(detail.Ping.Records)),
				" Tasks   unavailable",
			},
		})
	}
	if lines := pingTaskSparkLines(detail.Ping.Tasks, detail.Ping.Records, a.style.ASCII, 34, 5); len(lines) > 0 {
		sections = append(sections, detailSection{Title: "Latency by Task", Lines: lines})
	} else if values := pingRecordValues(detail.Ping.Records); len(values) > 0 {
		sections = append(sections, a.sparkSection("Latency Spark", values))
	}
	return sections
}

func (a *App) metaSections(node komari.Node, st komari.Status) []detailSection {
	window := detailWindows[a.window]
	detail := a.currentDetail(node.UUID)
	sections := []detailSection{
		{
			Title: "Identity",
			Lines: []string{
				fmt.Sprintf(" UUID    %s", node.UUID),
				fmt.Sprintf(" Name    %s", a.text(node.Name)),
				fmt.Sprintf(" IPv4    %s", a.nodeMetaValue(node.IPv4)),
				fmt.Sprintf(" IPv6    %s", a.nodeMetaValue(node.IPv6)),
				fmt.Sprintf(" Agent   %s", valueOr(a.text(node.Version), "-")),
			},
		},
		{
			Title: "Komari",
			Lines: []string{
				fmt.Sprintf(" Version %s", valueOr(a.snapshot.Version.Version, "-")),
				fmt.Sprintf(" Hash    %s", valueOr(a.snapshot.Version.Hash, "-")),
				fmt.Sprintf(" RPC     %s", valueOr(a.snapshot.RPCVersion, "-")),
				fmt.Sprintf(" Auth    %s", a.authText()),
				fmt.Sprintf(" Methods %d", len(a.snapshot.Methods)),
			},
		},
		{
			Title: "Site",
			Lines: []string{
				fmt.Sprintf(" Private %t", a.snapshot.Public.PrivateSite),
				fmt.Sprintf(" Records %t", a.snapshot.Public.RecordEnabled),
				fmt.Sprintf(" CORS    %t", a.snapshot.Public.CORSOriginCheckEnabled),
				fmt.Sprintf(" OAuth   %t %s", a.snapshot.Public.OAuthEnable, valueOr(a.snapshot.Public.OAuthProvider, "-")),
			},
		},
		{
			Title: "Detail Cache",
			Lines: []string{
				fmt.Sprintf(" Window  %s", window.Label),
				fmt.Sprintf(" Load    %d", len(detail.Load.Records)),
				fmt.Sprintf(" Ping    %d", len(detail.Ping.Records)),
				fmt.Sprintf(" Online  %t", st.Online),
				fmt.Sprintf(" Seen    %s", timeText(st.Time)),
			},
		},
	}
	if len(a.snapshot.Methods) > 0 {
		methods := make([]string, 0, min(len(a.snapshot.Methods), 5))
		for _, method := range a.snapshot.Methods {
			methods = append(methods, " "+method)
			if len(methods) >= 5 {
				break
			}
		}
		sections = append(sections, detailSection{Title: "RPC Methods", Lines: methods})
	}
	return sections
}

func (a *App) detailUsageLine(label string, pct float64, value string) string {
	cleanLabel := strings.TrimSpace(label)
	return fmt.Sprintf(" %-5s %5.1f%% %s  %s", cleanLabel, pct, a.usageBarFor(cleanLabel, pct, 10), value)
}

func (a *App) sparkSection(title string, values []float64) detailSection {
	return detailSection{
		Title: title,
		Lines: []string{
			" " + sparklineLimit(values, a.style.ASCII, 36),
			fmt.Sprintf(" Samples %d", len(values)),
		},
	}
}
