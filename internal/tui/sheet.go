package tui

import (
	"fmt"
	"strings"
	"time"

	"ktui/internal/komari"
)

func (a *App) renderSheetBody(width int, bodyHeight int) []string {
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

	cardHeight := sheetCardHeight
	columns, cardWidth := sheetLayout(width)

	rowsVisible := max(1, bodyHeight/cardHeight)
	selectedRow := a.selected / columns
	if selectedRow < a.scroll {
		a.scroll = selectedRow
	}
	if selectedRow >= a.scroll+rowsVisible {
		a.scroll = selectedRow - rowsVisible + 1
	}
	maxScroll := max(0, (len(nodes)+columns-1)/columns-rowsVisible)
	if a.scroll > maxScroll {
		a.scroll = maxScroll
	}
	if a.scroll < 0 {
		a.scroll = 0
	}

	lines := make([]string, 0, bodyHeight)
	startRow := a.scroll
	endRow := min((len(nodes)+columns-1)/columns, startRow+rowsVisible)
	for row := startRow; row < endRow; row++ {
		rowCards := make([][]string, 0, columns)
		for col := 0; col < columns; col++ {
			index := row*columns + col
			if index >= len(nodes) {
				rowCards = append(rowCards, emptyCard(cardWidth, cardHeight))
				continue
			}
			rowCards = append(rowCards, a.nodeCard(index, nodes[index], cardWidth, cardHeight))
		}
		for lineIndex := 0; lineIndex < cardHeight; lineIndex++ {
			var line strings.Builder
			for col, card := range rowCards {
				if col > 0 {
					line.WriteString(strings.Repeat(" ", sheetCardGap))
				}
				line.WriteString(card[lineIndex])
			}
			lines = append(lines, fitLine(line.String(), width))
		}
	}
	return fillBody(lines, width, bodyHeight)
}

func (a *App) nodeCard(index int, node komari.Node, width int, height int) []string {
	if height < 5 {
		height = 5
	}
	st := a.snapshot.Status[node.UUID]
	alert := a.alertForNode(node, st, time.Now())
	contentHeight := height - 2
	body := a.overviewCard(index, node, st, max(0, width-2), contentHeight)

	lines := make([]string, 0, height)
	lines = append(lines, a.cardTopForAlert(width, index == a.selected, alert))
	for i := 0; i < contentHeight; i++ {
		content := ""
		if i < len(body) {
			content = body[i]
		}
		line := a.style.boxLine(content, width)
		if index == a.selected && i == 0 && !a.style.NoColor {
			line = a.style.bold(line)
		} else if alert.Critical || alert.Warning {
			line = a.styleAlertLine(line, alert)
		} else if !st.Online {
			line = a.style.dim(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, a.cardBottomForAlert(width, index == a.selected, alert))
	return lines
}

func (a *App) overviewCard(index int, node komari.Node, st komari.Status, width int, maxLines int) []string {
	ramPct := percent(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal))
	diskPct := percent(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal))
	nameWidth := max(8, width-16)
	status := a.statusPill(st.Online)
	marker := " "
	if index == a.selected {
		marker = ">"
	}
	title := fmt.Sprintf(" %s %-*s %s", marker, nameWidth, cleanLine(a.nodeLabel(node), nameWidth), status)
	alert := a.alertForNode(node, st, time.Now())
	lines := []string{
		title,
		a.metricLine("CPU", st.CPU, "load "+fmt.Sprintf("%.2f %.2f %.2f", st.Load, st.Load5, st.Load15), width),
		a.metricLine("RAM", ramPct, usageCompact(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal)), width),
		a.metricLine("DSK", diskPct, usageCompact(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal)), width),
		fmt.Sprintf(" NET  %s %s  %s %s", a.style.up(), speedIEC(st.NetOut), a.style.down(), speedIEC(st.NetIn)),
		fmt.Sprintf(" FLOW %s %s  %s %s", a.style.up(), bytesIEC(st.NetTotalUp), a.style.down(), bytesIEC(st.NetTotalDown)),
		fmt.Sprintf(" UP   %-10s EXP %s", durationCompact(st.Uptime), expiryText(node, time.Now())),
	}
	if len(alert.Reasons) > 0 {
		lines = append([]string{title, fmt.Sprintf(" WARN %s", alertText(alert))}, lines[1:]...)
	}
	if node.Tags != "" || node.Group != "" {
		lines = append(lines, fmt.Sprintf(" OS   %s  TAG %s %s", valueOr(a.text(node.OS), "-"), valueOr(a.text(node.Group), "-"), valueOr(a.text(node.Tags), "-")))
	}
	return limitRawLines(lines, maxLines)
}

func (a *App) metricLine(label string, pct float64, value string, width int) string {
	barWidth := min(12, max(6, width-30))
	return fmt.Sprintf(" %-4s %5.1f%% %s  %s", label, pct, a.usageBarFor(label, pct, barWidth), value)
}

func (a *App) usageBar(pct float64, width int) string {
	return a.usageBarWithThreshold(pct, 90, width)
}

func (a *App) usageBarFor(label string, pct float64, width int) string {
	threshold := 90.0
	switch strings.ToUpper(strings.TrimSpace(label)) {
	case "CPU":
		threshold = a.warnCPU
	case "RAM":
		threshold = a.warnRAM
	case "DSK", "DISK":
		threshold = a.warnDisk
	}
	return a.usageBarWithThreshold(pct, threshold, width)
}

func (a *App) usageBarWithThreshold(pct float64, threshold float64, width int) string {
	bar := a.style.miniBar(pct, width)
	switch {
	case pct >= threshold:
		return a.style.red(bar)
	case pct >= threshold*0.85:
		return a.style.yellow(bar)
	default:
		return a.style.cyan(bar)
	}
}

func (a *App) statusPill(online bool) string {
	if a.style.ASCII {
		if online {
			return a.style.green("ONLINE")
		}
		return a.style.red("OFFLINE")
	}
	if online {
		return a.style.green("● online")
	}
	return a.style.red("● offline")
}

func (a *App) cardTop(width int, selected bool) string {
	return a.cardTopForAlert(width, selected, nodeAlert{})
}

func (a *App) cardTopForAlert(width int, selected bool, alert nodeAlert) string {
	line := a.style.boxTop(width)
	if selected {
		return a.style.cyan(line)
	}
	if alert.Critical || alert.Warning {
		return a.styleAlertLine(line, alert)
	}
	return a.style.dim(line)
}

func (a *App) cardBottom(width int, selected bool) string {
	return a.cardBottomForAlert(width, selected, nodeAlert{})
}

func (a *App) cardBottomForAlert(width int, selected bool, alert nodeAlert) string {
	line := a.style.boxBottom(width)
	if selected {
		return a.style.cyan(line)
	}
	if alert.Critical || alert.Warning {
		return a.styleAlertLine(line, alert)
	}
	return a.style.dim(line)
}
