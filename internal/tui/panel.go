package tui

import (
	"fmt"
	"sort"

	"ktui/internal/komari"
)

func (a *App) panelLines(width int, maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	switch a.tab {
	case 0:
		return a.overviewLines(width, maxLines)
	case 1:
		return a.detailLines(width, maxLines)
	case 2:
		return a.historyLines(width, maxLines)
	case 3:
		return a.pingLines(width, maxLines)
	default:
		return a.metaLines(width, maxLines)
	}
}

func (a *App) overviewLines(width int, maxLines int) []string {
	node, ok := a.selectedNode()
	if !ok {
		lines := []string{a.style.bold("Overview")}
		return limitLines(append(lines, "No data yet."), width, maxLines)
	}
	st := a.snapshot.Status[node.UUID]
	ramTotal := firstNonZero(st.RAMTotal, node.MemTotal)
	diskTotal := firstNonZero(st.DiskTotal, node.DiskTotal)
	lines := []string{a.style.bold("Overview: " + a.text(node.Name))}
	lines = append(lines,
		fmt.Sprintf("Status      %s", a.style.coloredStatus(st.Online)),
		fmt.Sprintf("Seen        %s", timeText(st.Time)),
		fmt.Sprintf("Uptime      %s", durationCompact(st.Uptime)),
		fmt.Sprintf("Region      %s", valueOr(a.text(node.Region), "-")),
		fmt.Sprintf("Group       %s", valueOr(a.text(node.Group), "-")),
		"",
		fmt.Sprintf("CPU         %5.1f%%  %s", st.CPU, a.style.bar(st.CPU, fitBar(width, 28))),
		fmt.Sprintf("RAM         %s", usage(st.RAM, ramTotal)),
		fmt.Sprintf("Disk        %s", usage(st.Disk, diskTotal)),
		fmt.Sprintf("Swap        %s", usage(st.Swap, firstNonZero(st.SwapTotal, node.SwapTotal))),
		fmt.Sprintf("Load        %.2f %.2f %.2f", st.Load, st.Load5, st.Load15),
		fmt.Sprintf("Net         %s %s  %s %s", a.style.up(), speedIEC(st.NetOut), a.style.down(), speedIEC(st.NetIn)),
		fmt.Sprintf("Traffic     %s %s  %s %s", a.style.up(), bytesIEC(st.NetTotalUp), a.style.down(), bytesIEC(st.NetTotalDown)),
		fmt.Sprintf("Processes   %d", st.Process),
		fmt.Sprintf("Conn        TCP %d UDP %d", st.Connections, st.ConnectionsUDP),
	)
	if node.TrafficLimit > 0 {
		pct := trafficPercent(st.NetTotalUp, st.NetTotalDown, node.TrafficLimit, node.TrafficLimitType)
		lines = append(lines, fmt.Sprintf("Limit       %.1f%% of %s", pct, trafficLimitText(node.TrafficLimit, node.TrafficLimitType)))
	}
	return limitLines(lines, width, maxLines)
}

func (a *App) detailLines(width int, maxLines int) []string {
	node, ok := a.selectedNode()
	if !ok {
		return []string{}
	}
	st := a.snapshot.Status[node.UUID]
	lines := []string{a.style.bold("Node")}

	lines = append(lines,
		fmt.Sprintf("%s  %s", a.text(node.Name), a.style.coloredStatus(st.Online)),
		fmt.Sprintf("UUID    %s", node.UUID),
		fmt.Sprintf("Region  %s", valueOr(a.text(node.Region), "-")),
		fmt.Sprintf("Group   %s", valueOr(a.text(node.Group), "-")),
		fmt.Sprintf("OS      %s / %s", valueOr(a.text(node.OS), "-"), valueOr(a.text(node.Arch), "-")),
		fmt.Sprintf("Kernel  %s", valueOr(a.text(node.KernelVersion), "-")),
		fmt.Sprintf("CPU     %s (%d cores, %d physical)", valueOr(a.text(node.CPUName), "-"), node.CPUCores, node.CPUPhysicalCores),
		fmt.Sprintf("GPU     %s", valueOr(a.text(node.GPUName), "-")),
		fmt.Sprintf("Virt    %s", valueOr(a.text(node.Virtualization), "-")),
		fmt.Sprintf("Agent   %s", valueOr(a.text(node.Version), "-")),
		"",
		fmt.Sprintf("CPU     %5.1f%%  %s", st.CPU, a.style.bar(st.CPU, fitBar(width, 28))),
		fmt.Sprintf("RAM     %s", usage(firstNonZero(st.RAM, 0), firstNonZero(st.RAMTotal, node.MemTotal))),
		fmt.Sprintf("Disk    %s", usage(firstNonZero(st.Disk, 0), firstNonZero(st.DiskTotal, node.DiskTotal))),
		fmt.Sprintf("Swap    %s", usage(st.Swap, firstNonZero(st.SwapTotal, node.SwapTotal))),
		fmt.Sprintf("Load    %.2f %.2f %.2f", st.Load, st.Load5, st.Load15),
		fmt.Sprintf("Net     %s %s  %s %s", a.style.up(), speedIEC(st.NetOut), a.style.down(), speedIEC(st.NetIn)),
		fmt.Sprintf("Traffic %s %s  %s %s", a.style.up(), bytesIEC(st.NetTotalUp), a.style.down(), bytesIEC(st.NetTotalDown)),
		fmt.Sprintf("Uptime  %s", durationCompact(st.Uptime)),
		fmt.Sprintf("Proc    %d   Conn TCP %d UDP %d", st.Process, st.Connections, st.ConnectionsUDP),
		fmt.Sprintf("Seen    %s", timeText(st.Time)),
	)

	if node.IPv4 != "" || node.IPv6 != "" {
		lines = append(lines,
			"",
			fmt.Sprintf("IPv4    %s", valueOr(node.IPv4, "-")),
			fmt.Sprintf("IPv6    %s", valueOr(node.IPv6, "-")),
		)
	}
	if node.TrafficLimit > 0 {
		pct := trafficPercent(st.NetTotalUp, st.NetTotalDown, node.TrafficLimit, node.TrafficLimitType)
		lines = append(lines, fmt.Sprintf("Limit   %.1f%% of %s", pct, trafficLimitText(node.TrafficLimit, node.TrafficLimitType)))
	}
	if node.Price != 0 || node.ExpiredAt.Valid || node.Tags != "" || node.PublicRemark != "" || node.Remark != "" {
		price := "-"
		if node.Price > 0 {
			price = fmt.Sprintf("%s%.2f / %dd", valueOr(node.Currency, ""), node.Price, node.BillingCycle)
		} else if node.Price < 0 {
			price = "free"
		}
		lines = append(lines,
			"",
			fmt.Sprintf("Billing %s auto=%t", price, node.AutoRenewal),
			fmt.Sprintf("Expire  %s", nullableDate(node.ExpiredAt)),
			fmt.Sprintf("Tags    %s", valueOr(a.text(node.Tags), "-")),
			fmt.Sprintf("Public  %s", valueOr(a.text(node.PublicRemark), "-")),
			fmt.Sprintf("Remark  %s", valueOr(a.text(node.Remark), "-")),
		)
	}

	return limitLines(lines, width, maxLines)
}

func (a *App) historyLines(width int, maxLines int) []string {
	node, ok := a.selectedNode()
	if !ok {
		return nil
	}
	window := detailWindows[a.window]
	detail := a.currentDetail(node.UUID)
	lines := []string{a.style.bold(fmt.Sprintf("History: %s  window %s", a.text(node.Name), window.Label))}
	if detail.Loading {
		lines = append(lines, "Loading records...")
	}
	if detail.Err != nil {
		lines = append(lines, a.style.red("detail error: "+detail.Err.Error()))
	}
	var records []komari.Status
	if window.Hours == 0 {
		st := a.snapshot.Status[node.UUID]
		records = a.realtimeRecords(node.UUID, detail.Recent.Records, st)
	} else {
		records = loadChartRecords(detail.Load.Records, window.Hours)
	}
	if len(records) == 0 {
		lines = append(lines, "No load history loaded yet. Press d to refresh details.")
		return limitLines(lines, width, maxLines)
	}

	sum := summarizeStatusWithTotals(records, node.MemTotal, node.DiskTotal)
	sparkWidth := fitBar(width, 10)
	lines = append(lines,
		fmt.Sprintf("Window  %s - %s   samples %d", nullableDateTime(detail.Load.From), nullableDateTime(detail.Load.To), len(records)),
		fmt.Sprintf("CPU     avg %5.1f%%  max %5.1f%%", sum.CPUAvg, sum.CPUMax),
		fmt.Sprintf("RAM     avg %5.1f%%  max %5.1f%%", sum.RAMAvg, sum.RAMMax),
		fmt.Sprintf("Disk    avg %5.1f%%  max %5.1f%%", sum.DiskAvg, sum.DiskMax),
		fmt.Sprintf("Load    avg %.2f  max %.2f", sum.LoadAvg, sum.LoadMax),
		fmt.Sprintf("Net     avg %s %s  %s %s", a.style.up(), speedIEC(int64(sum.NetOutAvg)), a.style.down(), speedIEC(int64(sum.NetInAvg))),
		fmt.Sprintf("Conn    avg %.0f  max %d", sum.ConnectionsAvg, sum.ConnectionsMax),
		fmt.Sprintf("Proc    avg %.0f  max %d", sum.ProcessAvg, sum.ProcessMax),
		"",
		"CPU spark",
		"  "+sparklineLimit(statusValues(records, func(st komari.Status) float64 { return st.CPU }), a.style.ASCII, sparkWidth),
		"RAM spark",
		"  "+sparklineLimit(statusRAMPercentValues(records, node.MemTotal), a.style.ASCII, sparkWidth),
		"Disk spark",
		"  "+sparklineLimit(statusDiskPercentValues(records, node.DiskTotal), a.style.ASCII, sparkWidth),
		"Net out spark",
		"  "+sparklineLimit(statusValues(records, func(st komari.Status) float64 { return float64(st.NetOut) }), a.style.ASCII, sparkWidth),
		"Net in spark",
		"  "+sparklineLimit(statusValues(records, func(st komari.Status) float64 { return float64(st.NetIn) }), a.style.ASCII, sparkWidth),
		"Connections spark",
		"  "+sparklineLimit(statusValues(records, func(st komari.Status) float64 { return float64(st.Connections) }), a.style.ASCII, sparkWidth),
		"Process spark",
		"  "+sparklineLimit(statusValues(records, func(st komari.Status) float64 { return float64(st.Process) }), a.style.ASCII, sparkWidth),
	)
	return limitLines(lines, width, maxLines)
}

func (a *App) pingLines(width int, maxLines int) []string {
	node, ok := a.selectedNode()
	if !ok {
		return nil
	}
	st := a.snapshot.Status[node.UUID]
	window := detailWindows[a.window]
	detail := a.currentDetail(node.UUID)
	lines := []string{a.style.bold(fmt.Sprintf("Ping: %s  window %s", a.text(node.Name), window.Label))}

	if len(st.Ping) > 0 {
		lines = append(lines, "Realtime")
		keys := make([]string, 0, len(st.Ping))
		for key := range st.Ping {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			ping := st.Ping[key]
			lines = append(lines, fmt.Sprintf("  %-14s latest %.0fms avg %.0fms min %.0f max %.0f loss %.1f%%",
				cleanLine(a.text(ping.Name), 14), ping.Latest, ping.Avg, ping.Min, ping.Max, ping.Loss))
		}
	}
	if window.Hours == 0 {
		if len(st.Ping) == 0 {
			lines = append(lines, "No realtime ping data.")
		} else {
			records := a.realtimeRecords(node.UUID, detail.Recent.Records, st)
			sparkWidth := fitBar(width, 10)
			if values := realtimePingStatusValues(records, func(ping komari.Ping) float64 { return ping.Latest }); len(values) > 0 {
				lines = append(lines, "", "Latency spark", "  "+sparklineLimit(values, a.style.ASCII, sparkWidth))
			}
			if values := realtimePingStatusValues(records, func(ping komari.Ping) float64 { return ping.Loss }); len(values) > 0 {
				lines = append(lines, "Loss spark", "  "+sparklineLimit(values, a.style.ASCII, sparkWidth))
			}
		}
		return limitLines(lines, width, maxLines)
	}

	if detail.Loading {
		lines = append(lines, "", "Loading ping records...")
	}
	if detail.Err != nil {
		lines = append(lines, a.style.red("detail error: "+detail.Err.Error()))
	}
	if len(detail.Ping.Tasks) == 0 && len(detail.Ping.Records) == 0 {
		lines = append(lines, "", "No ping history loaded yet. Press d to refresh details.")
		return limitLines(lines, width, maxLines)
	}
	lines = append(lines, "", fmt.Sprintf("History samples %d", len(detail.Ping.Records)))
	for _, task := range detail.Ping.Tasks {
		lines = append(lines, fmt.Sprintf("  %-16s avg %.0fms min %.0f max %.0f loss %.1f%% total %d",
			cleanLine(a.text(task.Name), 16), task.Avg, task.Min, task.Max, task.Loss, task.Total))
	}
	if taskSparkLines := pingTaskSparkLines(detail.Ping.Tasks, detail.Ping.Records, a.style.ASCII, fitBar(width, 10)+20, 4); len(taskSparkLines) > 0 {
		lines = append(lines, "", "Latency by task")
		lines = append(lines, taskSparkLines...)
	} else if values := pingRecordValues(detail.Ping.Records); len(values) > 0 {
		lines = append(lines, "", "Latency spark", "  "+sparklineLimit(values, a.style.ASCII, fitBar(width, 10)))
	}
	return limitLines(lines, width, maxLines)
}

func (a *App) metaLines(width int, maxLines int) []string {
	node, ok := a.selectedNode()
	if !ok {
		return nil
	}
	st := a.snapshot.Status[node.UUID]
	window := detailWindows[a.window]
	detail := a.currentDetail(node.UUID)
	lines := []string{a.style.bold(fmt.Sprintf("Meta: %s  window %s", a.text(node.Name), window.Label))}
	lines = append(lines,
		fmt.Sprintf("UUID       %s", node.UUID),
		fmt.Sprintf("IPv4       %s", a.nodeMetaValue(node.IPv4)),
		fmt.Sprintf("IPv6       %s", a.nodeMetaValue(node.IPv6)),
		fmt.Sprintf("Agent      %s", valueOr(a.text(node.Version), "-")),
		fmt.Sprintf("Seen       %s", timeText(st.Time)),
		fmt.Sprintf("Online     %t", st.Online),
		"",
		fmt.Sprintf("Komari     %s", valueOr(a.snapshot.Version.Version, "-")),
		fmt.Sprintf("Hash       %s", valueOr(a.snapshot.Version.Hash, "-")),
		fmt.Sprintf("RPC        %s", valueOr(a.snapshot.RPCVersion, "-")),
		fmt.Sprintf("Auth       logged_in=%t user=%s uuid=%s", a.snapshot.Me.LoggedIn, valueOr(a.snapshot.Me.Username, "-"), valueOr(a.snapshot.Me.UUID, "-")),
		fmt.Sprintf("Private    %t", a.snapshot.Public.PrivateSite),
		fmt.Sprintf("CORS       %t", a.snapshot.Public.CORSOriginCheckEnabled),
		fmt.Sprintf("OAuth      %t %s", a.snapshot.Public.OAuthEnable, valueOr(a.snapshot.Public.OAuthProvider, "-")),
		fmt.Sprintf("Methods    %d", len(a.snapshot.Methods)),
	)
	if window.Hours > 0 {
		lines = append(lines,
			"",
			fmt.Sprintf("Window     %s", window.Label),
			fmt.Sprintf("Load recs  %d", len(detail.Load.Records)),
			fmt.Sprintf("Ping recs  %d", len(detail.Ping.Records)),
			fmt.Sprintf("From       %s", nullableDateTime(detail.Load.From)),
			fmt.Sprintf("To         %s", nullableDateTime(detail.Load.To)),
		)
		if detail.Loading {
			lines = append(lines, "Detail     loading")
		}
		if detail.Err != nil {
			lines = append(lines, "Detail     "+detail.Err.Error())
		}
	} else {
		lines = append(lines,
			"",
			"Realtime status",
			fmt.Sprintf("CPU        %.1f%%", st.CPU),
			fmt.Sprintf("RAM        %s", usage(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal))),
			fmt.Sprintf("Disk       %s", usage(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal))),
			fmt.Sprintf("Net        %s %s  %s %s", a.style.up(), speedIEC(st.NetOut), a.style.down(), speedIEC(st.NetIn)),
		)
	}
	if len(a.snapshot.Methods) > 0 {
		lines = append(lines, "")
		for _, method := range a.snapshot.Methods {
			lines = append(lines, "  "+method)
		}
	}
	return limitLines(lines, width, maxLines)
}

func (a *App) nodeLabel(node komari.Node) string {
	name := a.text(node.Name)
	region := a.text(node.Region)
	if a.style.ASCII || region == "" {
		return name
	}
	return region + " " + name
}

func (a *App) text(value string) string {
	return a.style.sanitizeText(value)
}

func limitLines(lines []string, width int, maxLines int) []string {
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	for i := range lines {
		lines[i] = fitLine(lines[i], width)
	}
	return lines
}
