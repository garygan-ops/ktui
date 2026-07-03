package tui

import (
	"fmt"
	"strings"
	"time"

	"ktui/internal/komari"
)

func (a *App) renderLineBody(width int, bodyHeight int) []string {
	if bodyHeight <= 0 {
		return nil
	}
	nodes := a.viewNodes()
	if len(nodes) == 0 {
		if a.loading {
			return fillBody([]string{"Loading nodes..."}, width, bodyHeight)
		}
		if len(a.snapshot.Nodes) > 0 {
			return fillBody([]string{"No nodes match the current search/filter."}, width, bodyHeight)
		}
		return fillBody([]string{"No nodes returned by Komari."}, width, bodyHeight)
	}

	a.clampSelection()
	header := a.lineTableHeader(width)
	visibleRows := max(1, bodyHeight-len(header))
	a.adjustScroll(visibleRows)

	lines := make([]string, 0, bodyHeight)
	lines = append(lines, header...)
	end := min(len(nodes), a.scroll+visibleRows)
	for i := a.scroll; i < end; i++ {
		lines = append(lines, a.lineTableRow(i, nodes[i], width))
	}
	lines = fillBody(lines, width, bodyHeight)
	return a.withScrollIndicator(lines, width, scrollIndicator{
		Start:   len(header),
		Height:  visibleRows,
		Offset:  a.scroll,
		Visible: visibleRows,
		Total:   len(nodes),
	})
}

func (a *App) lineTableHeader(width int) []string {
	title := a.lineTableColumns(width, false, komari.Node{}, komari.Status{}, false)
	return []string{
		a.style.bold(fitLine(a.listTitle(), width)),
		a.style.dim(fitLine(title, width)),
		a.style.dim(fitLine(a.style.separator(width), width)),
	}
}

func (a *App) lineTableRow(index int, node komari.Node, width int) string {
	st := a.snapshot.Status[node.UUID]
	selected := index == a.selected
	line := a.lineTableColumns(width, true, node, st, selected)
	if index == a.selected {
		return a.style.inverse(fitLine(line, width))
	}
	alert := a.alertForNode(node, st, time.Now())
	if alert.Critical || alert.Warning {
		return a.styleAlertLine(fitLine(line, width), alert)
	}
	if !st.Online {
		return a.style.dim(fitLine(line, width))
	}
	return fitLine(line, width)
}

func (a *App) lineTableColumns(width int, row bool, node komari.Node, st komari.Status, selected bool) string {
	nameWidth := 28
	regionWidth := 10
	netWidth := 19
	trafficWidth := 24
	uptimeWidth := 10
	loadWidth := 16
	osWidth := 12
	expWidth := 8
	tagWidth := 16
	showDisk := width >= 72
	showRegion := width >= 86
	showNet := width >= 104
	showLoad := width >= 122
	showUptime := width >= 136
	showTraffic := width >= 162
	showRuntime := width >= 180
	showExp := width >= 192
	showOS := width >= 208
	showTags := width >= 226
	if width < 64 {
		nameWidth = max(12, width-34)
	} else if width < 86 {
		nameWidth = 24
	}

	if !row {
		parts := []string{
			fmt.Sprintf("   %-3s %-*s %7s %7s", "ST", nameWidth, "NODE", "CPU", "RAM"),
		}
		if showDisk {
			parts = append(parts, fmt.Sprintf(" %7s", "DISK"))
		}
		if showRegion {
			parts = append(parts, fmt.Sprintf(" %-*s", regionWidth, "REGION"))
		}
		if showNet {
			parts = append(parts, fmt.Sprintf(" %-*s", netWidth, "NET"))
		}
		if showLoad {
			parts = append(parts, fmt.Sprintf(" %-*s", loadWidth, "LOAD"))
		}
		if showUptime {
			parts = append(parts, fmt.Sprintf(" %-*s", uptimeWidth, "UPTIME"))
		}
		if showTraffic {
			parts = append(parts, fmt.Sprintf(" %-*s", trafficWidth, "TRAFFIC"))
		}
		if showRuntime {
			parts = append(parts, fmt.Sprintf(" %5s %4s", "CONN", "PROC"))
		}
		if showExp {
			parts = append(parts, fmt.Sprintf(" %-*s", expWidth, "EXP"))
		}
		if showOS {
			parts = append(parts, fmt.Sprintf(" %-*s", osWidth, "OS"))
		}
		if showTags {
			parts = append(parts, fmt.Sprintf(" %-*s", tagWidth, "TAG"))
		}
		return strings.Join(parts, "")
	}

	marker := " "
	if selected {
		marker = ">"
	}
	alert := a.alertForNode(node, st, time.Now())
	state := "on"
	if !st.Online {
		state = "off"
	} else if len(alert.Reasons) > 0 {
		state = "!"
	}
	if !a.style.ASCII {
		if len(alert.Reasons) > 0 && st.Online {
			state = "!"
		} else if st.Online {
			state = "●"
		} else {
			state = "●"
		}
		if !selected {
			if alert.Critical || alert.Warning {
				state = a.styleAlertLine(state, alert)
			} else if st.Online {
				state = a.style.green(state)
			} else {
				state = a.style.red(state)
			}
		}
	}
	stateCell := padRight(state, 3)
	parts := []string{
		fmt.Sprintf(" %s %s %-*s %6.1f%% %6.1f%%",
			marker,
			stateCell,
			nameWidth,
			cleanLine(a.nodeLabel(node), nameWidth),
			st.CPU,
			percent(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal)),
		),
	}
	if showDisk {
		parts = append(parts, fmt.Sprintf(" %6.1f%%", percent(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal))))
	}
	if showRegion {
		parts = append(parts, fmt.Sprintf(" %-*s", regionWidth, cleanLine(valueOr(a.text(node.Region), "-"), regionWidth)))
	}
	if showNet {
		net := fmt.Sprintf("%s %s %s %s", a.style.up(), speedIEC(st.NetOut), a.style.down(), speedIEC(st.NetIn))
		parts = append(parts, fmt.Sprintf(" %-*s", netWidth, cleanLine(net, netWidth)))
	}
	if showLoad {
		load := fmt.Sprintf("%.2f %.2f %.2f", st.Load, st.Load5, st.Load15)
		parts = append(parts, fmt.Sprintf(" %-*s", loadWidth, cleanLine(load, loadWidth)))
	}
	if showUptime {
		parts = append(parts, fmt.Sprintf(" %-*s", uptimeWidth, durationCompact(st.Uptime)))
	}
	if showTraffic {
		traffic := fmt.Sprintf("%s %s %s %s", a.style.up(), bytesIEC(st.NetTotalUp), a.style.down(), bytesIEC(st.NetTotalDown))
		if node.TrafficLimit > 0 {
			pct := trafficPercent(st.NetTotalUp, st.NetTotalDown, node.TrafficLimit, node.TrafficLimitType)
			traffic = fmt.Sprintf("%.1f%% %s", pct, trafficLimitText(node.TrafficLimit, node.TrafficLimitType))
		}
		parts = append(parts, fmt.Sprintf(" %-*s", trafficWidth, cleanLine(traffic, trafficWidth)))
	}
	if showRuntime {
		parts = append(parts, fmt.Sprintf(" %5d %4d", st.Connections, st.Process))
	}
	if showExp {
		parts = append(parts, fmt.Sprintf(" %-*s", expWidth, cleanLine(expiryText(node, time.Now()), expWidth)))
	}
	if showOS {
		parts = append(parts, fmt.Sprintf(" %-*s", osWidth, cleanLine(valueOr(a.text(node.OS), "-"), osWidth)))
	}
	if showTags {
		tag := strings.TrimSpace(valueOr(a.text(node.Group), "") + " " + valueOr(a.text(node.Tags), ""))
		parts = append(parts, fmt.Sprintf(" %-*s", tagWidth, cleanLine(valueOr(tag, "-"), tagWidth)))
	}
	return strings.Join(parts, "")
}
