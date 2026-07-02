package tui

import (
	"fmt"
	"strings"
)

func fillBody(lines []string, width int, height int) []string {
	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		out = append(out, fitLine(line, width))
	}
	return out
}

func (a *App) writeFrame(b *strings.Builder, width int, height int, lines []string) {
	if width <= 0 || height <= 0 {
		return
	}
	drawWidth := width
	if drawWidth > 1 {
		drawWidth--
	}
	b.WriteString("\x1b[2J")
	for row := 0; row < height; row++ {
		line := ""
		if row < len(lines) {
			line = lines[row]
		}
		b.WriteString(fmt.Sprintf("\x1b[%d;1H", row+1))
		b.WriteString(fitLine(line, drawWidth))
		b.WriteString("\x1b[K")
	}
}

func (a *App) headerLines(width int) []string {
	title := a.text(a.snapshot.Public.SiteName)
	if title == "" {
		title = "Komari TUI"
	}
	status := "READY"
	if a.loading {
		status = "LOADING"
	}
	if a.err != nil {
		status = "ERROR"
	}
	mode := "utf8"
	if a.style.ASCII {
		mode = "ascii"
	}

	head := a.style.inverse(headerAlign(" ktui  "+title, a.authText()+"  "+status+" ", width))
	onlineValue := fmt.Sprintf("%d/%d", a.snapshot.Online, a.snapshot.Total)
	switch {
	case a.snapshot.Total == 0:
		onlineValue = a.style.dim(onlineValue)
	case a.snapshot.Online == a.snapshot.Total:
		onlineValue = a.style.green(onlineValue)
	case a.snapshot.Online == 0:
		onlineValue = a.style.red(onlineValue)
	default:
		onlineValue = a.style.yellow(onlineValue)
	}
	summary := strings.Join([]string{
		a.headerChip("online", onlineValue),
		a.headerChip("regions", fmt.Sprintf("%d", len(a.snapshot.RegionList))),
		a.headerChip("updated", shortTime(a.snapshot.FetchedAt)),
	}, "  ")

	transfer := strings.Join([]string{
		a.headerChip("traffic", fmt.Sprintf("%s %s  %s %s",
			a.style.up(),
			headerCompactUnit(bytesIEC(a.snapshot.TotalUp)),
			a.style.down(),
			headerCompactUnit(bytesIEC(a.snapshot.TotalDown)),
		)),
		a.headerChip("speed", fmt.Sprintf("%s %s  %s %s",
			a.style.up(),
			headerCompactUnit(speedIEC(a.snapshot.SpeedUp)),
			a.style.down(),
			headerCompactUnit(speedIEC(a.snapshot.SpeedDown)),
		)),
	}, "  ")

	contextParts := []string{
		a.headerChip("view", "list"),
		a.headerChip("mode", string(a.mode)+"/"+mode),
		a.headerChip("komari", valueOr(a.snapshot.Version.Version, "-")),
		a.headerChip("rpc", valueOr(a.snapshot.RPCVersion, "-")),
	}
	if a.detail {
		nodeName := "-"
		if len(a.snapshot.Nodes) > 0 {
			nodeName = a.nodeLabel(a.snapshot.Nodes[a.selected])
		}
		contextParts = []string{
			a.headerChip("view", "detail"),
			a.headerChip("node", nodeName),
			a.headerChip("komari", valueOr(a.snapshot.Version.Version, "-")),
			a.headerChip("rpc", valueOr(a.snapshot.RPCVersion, "-")),
		}
	}
	context := strings.Join(contextParts, "  ")
	if a.err != nil {
		context = a.style.red("ERROR " + a.err.Error())
	}

	lines := []string{
		head,
		fitLine(summary, width),
		fitLine(transfer, width),
		fitLine(context, width),
	}
	return lines
}

func (a *App) headerChip(label string, value string) string {
	if strings.TrimSpace(value) == "" {
		value = "-"
	}
	return a.style.dim(strings.ToUpper(label)) + " " + value
}

func headerAlign(left string, right string, width int) string {
	left = cleanLine(left, width)
	right = cleanLine(right, width)
	gap := width - displayWidth(left) - displayWidth(right)
	if gap < 1 {
		return fitLine(left+" "+right, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

func headerCompactUnit(value string) string {
	return strings.ReplaceAll(value, " ", "")
}

func (a *App) footerLine(width int) string {
	footer := " ↑↓/jk select   Enter detail   m mode   r refresh   a ascii   q quit "
	if a.detail {
		footer = " Esc/q back   1-5/h/l tabs   [ ] window   j/k scroll   r refresh   d reload "
	}
	return a.style.inverse(cleanLine(footer, width))
}

func (a *App) adjustScroll(visibleRows int) {
	if a.selected < a.scroll {
		a.scroll = a.selected
	}
	if a.selected >= a.scroll+visibleRows {
		a.scroll = a.selected - visibleRows + 1
	}
	if a.scroll < 0 {
		a.scroll = 0
	}
	maxScroll := len(a.snapshot.Nodes) - visibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	if a.scroll > maxScroll {
		a.scroll = maxScroll
	}
}
