package tui

import (
	"fmt"
	"sort"
	"strings"

	"ktui/internal/komari"
)

func (a *App) realtimePingLines(st komari.Status, maxLines int) []string {
	if len(st.Ping) == 0 || maxLines <= 0 {
		return nil
	}
	keys := make([]string, 0, len(st.Ping))
	for key := range st.Ping {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, min(len(keys), maxLines))
	for _, key := range keys {
		ping := st.Ping[key]
		name := valueOr(a.text(ping.Name), key)
		lines = append(lines, fmt.Sprintf(" %-10s now %4.0f avg %4.0f loss %.1f%%",
			cleanLine(name, 10), ping.Latest, ping.Avg, ping.Loss))
		if len(lines) >= maxLines {
			break
		}
	}
	return lines
}

func (a *App) nodeTabDetails(node komari.Node, st komari.Status, width int, maxLines int) []string {
	var lines []string
	switch a.tab {
	case 1:
		lines = a.nodeInfoCardDetails(node)
	case 2:
		lines = a.historyCardDetails(node, width)
	case 3:
		lines = a.pingCardDetails(node, st)
	case 4:
		lines = a.metaCardDetails(node)
	default:
		lines = a.overviewCardDetails(node, st, width)
	}
	return limitRawLines(lines, maxLines)
}

func (a *App) overviewCardDetails(node komari.Node, st komari.Status, width int) []string {
	ramPct := percent(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal))
	diskPct := percent(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal))
	barWidth := max(6, width-14)
	return []string{
		fmt.Sprintf(" CPU  %5.1f%% %s", st.CPU, a.style.bar(st.CPU, barWidth)),
		fmt.Sprintf(" RAM  %5.1f%% %s", ramPct, a.style.bar(ramPct, barWidth)),
		fmt.Sprintf(" DISK %5.1f%% %s", diskPct, a.style.bar(diskPct, barWidth)),
		fmt.Sprintf(" NET  %s %s  %s %s", a.style.up(), speedIEC(st.NetOut), a.style.down(), speedIEC(st.NetIn)),
		fmt.Sprintf(" FLOW %s %s  %s %s", a.style.up(), bytesIEC(st.NetTotalUp), a.style.down(), bytesIEC(st.NetTotalDown)),
		fmt.Sprintf(" LOAD %.2f %.2f %.2f | UP %s", st.Load, st.Load5, st.Load15, durationCompact(st.Uptime)),
	}
}

func (a *App) nodeInfoCardDetails(node komari.Node) []string {
	return []string{
		fmt.Sprintf(" OS   %s / %s", valueOr(a.text(node.OS), "-"), valueOr(a.text(node.Arch), "-")),
		fmt.Sprintf(" KERN %s", valueOr(a.text(node.KernelVersion), "-")),
		fmt.Sprintf(" CPU  %s", valueOr(a.text(node.CPUName), "-")),
		fmt.Sprintf(" CORE %s | VIRT %s", coreText(node), valueOr(a.text(node.Virtualization), "-")),
		fmt.Sprintf(" MEM  %s | DISK %s", bytesIEC(node.MemTotal), bytesIEC(node.DiskTotal)),
		fmt.Sprintf(" GPU  %s", valueOr(a.text(node.GPUName), "-")),
	}
}

func (a *App) historyCardDetails(node komari.Node, width int) []string {
	detail := a.currentDetail(node.UUID)
	st := a.snapshot.Status[node.UUID]
	var records []komari.Status
	if detailWindows[a.window].Hours == 0 {
		records = a.realtimeRecords(node.UUID, detail.Recent.Records, st)
	} else if len(detail.Load.Records) > 0 {
		records = detail.Load.Records
	} else {
		if detail.Loading {
			return []string{
				" Loading load history...",
				" Press d to force refresh.",
			}
		}
		if detail.Err != nil {
			return []string{
				" ERR " + detail.Err.Error(),
				" Press d to retry.",
			}
		}
		return []string{
			" No history loaded.",
			" Select this node or press d.",
		}
	}

	sum := summarizeStatusWithTotals(records, node.MemTotal, node.DiskTotal)
	sparkWidth := max(6, width-9)
	return []string{
		fmt.Sprintf(" SAMPLES %-4d CPU %.1f RAM %.1f", len(records), sum.CPUAvg, sum.RAMAvg),
		fmt.Sprintf(" DISK %.1f NET %s/%s", sum.DiskAvg, speedIEC(int64(sum.NetOutAvg)), speedIEC(int64(sum.NetInAvg))),
		fmt.Sprintf(" CONN avg %.0f max %d", sum.ConnectionsAvg, sum.ConnectionsMax),
		fmt.Sprintf(" PROC avg %.0f max %d", sum.ProcessAvg, sum.ProcessMax),
		" CPU  " + sparklineLimit(statusValues(records, func(st komari.Status) float64 { return st.CPU }), a.style.ASCII, sparkWidth),
		" NET  " + sparklineLimit(statusValues(records, func(st komari.Status) float64 { return float64(st.NetOut + st.NetIn) }), a.style.ASCII, sparkWidth),
	}
}

func (a *App) pingCardDetails(node komari.Node, st komari.Status) []string {
	lines := make([]string, 0, 8)
	if len(st.Ping) > 0 {
		keys := make([]string, 0, len(st.Ping))
		for key := range st.Ping {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			ping := st.Ping[key]
			name := valueOr(a.text(ping.Name), key)
			lines = append(lines, fmt.Sprintf(" NOW %-8s %4.0fms avg %4.0f loss %.1f%%",
				cleanLine(name, 8), ping.Latest, ping.Avg, ping.Loss))
			if len(lines) >= 3 {
				break
			}
		}
	}

	detail := a.currentDetail(node.UUID)
	if detailWindows[a.window].Hours == 0 {
		records := a.realtimeRecords(node.UUID, detail.Recent.Records, st)
		sparkWidth := 18
		if values := realtimePingStatusValues(records, func(ping komari.Ping) float64 { return ping.Latest }); len(values) > 0 {
			lines = append(lines, " LAT "+sparklineLimit(values, a.style.ASCII, sparkWidth))
		}
		if values := realtimePingStatusValues(records, func(ping komari.Ping) float64 { return ping.Loss }); len(values) > 0 {
			lines = append(lines, " LOS "+sparklineLimit(values, a.style.ASCII, sparkWidth))
		}
		if len(lines) == 0 {
			lines = append(lines, " No ping data. Press d.")
		}
		return lines
	}
	if detail.Loading {
		lines = append(lines, " Loading 6h ping history...")
	}
	if detail.Err != nil {
		lines = append(lines, " ERR "+detail.Err.Error())
	}
	if len(detail.Ping.Tasks) > 0 {
		for _, task := range detail.Ping.Tasks {
			lines = append(lines, fmt.Sprintf(" 6H  %-8s avg %4.0f loss %.1f%%",
				cleanLine(a.text(task.Name), 8), task.Avg, task.Loss))
			if len(lines) >= 6 {
				break
			}
		}
	} else if len(lines) == 0 {
		lines = append(lines, " No ping data. Press d.")
	}
	return lines
}

func (a *App) metaCardDetails(node komari.Node) []string {
	return []string{
		" UUID " + shortID(node.UUID),
		" IPv4 " + a.nodeMetaValue(node.IPv4),
		" IPv6 " + a.nodeMetaValue(node.IPv6),
		" AGENT " + valueOr(a.text(node.Version), "-"),
		fmt.Sprintf(" KOMARI %s RPC %s", valueOr(a.snapshot.Version.Version, "-"), valueOr(a.snapshot.RPCVersion, "-")),
		fmt.Sprintf(" AUTH %s | METHODS %d", a.authText(), len(a.snapshot.Methods)),
	}
}

func (a *App) nodeMetaValue(value string) string {
	if strings.TrimSpace(value) != "" {
		return a.text(value)
	}
	if !a.client.HasAPIKey() {
		return "api-key required"
	}
	if a.snapshot.NodeDetailErr != nil {
		return "detail unavailable"
	}
	return "-"
}

func (a *App) currentDetail(uuid string) nodeDetail {
	return a.nodeDetail[detailKey{UUID: uuid, Window: a.window}]
}
