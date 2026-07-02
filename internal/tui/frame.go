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
	if a.settings {
		contextParts = []string{
			a.headerChip("view", "settings"),
			a.headerChip("mode", string(a.mode)+"/"+mode),
			a.headerChip("komari", valueOr(a.snapshot.Version.Version, "-")),
			a.headerChip("rpc", valueOr(a.snapshot.RPCVersion, "-")),
		}
	} else if a.detail {
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
	} else if a.notice != "" {
		context = a.style.yellow(a.notice)
	} else if a.update.Available {
		context = a.style.yellow(fmt.Sprintf("UPDATE %s available  run `ktui update`", valueOr(a.update.Latest, "latest")))
	} else if a.update.Checking {
		context = a.style.dim("checking for updates...")
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
	return a.style.inverse(cleanLine(a.footerTextForWidth(width), width))
}

func (a *App) footerText() string {
	return a.footerTextForWidth(0)
}

func (a *App) footerTextForWidth(width int) string {
	items := a.footerItems()
	for variant := 0; variant < footerLabelVariants; variant++ {
		text := footerTextForVariant(items, variant)
		if width <= 0 || displayWidth(text) <= width {
			return text
		}
	}
	return footerTextForVariant(items, footerLabelVariants-1)
}

func (a *App) footerItems() []footerItem {
	if a.settings {
		return []footerItem{
			{Action: footerBack, Labels: [footerLabelVariants]string{"Esc/q back", "Back", "B"}},
			{Action: footerSelect, Labels: [footerLabelVariants]string{"↑↓/jk select", "Select", "Sel"}},
			{Action: footerAdjust, Labels: [footerLabelVariants]string{"←→/h/l adjust", "Adjust", "Adj"}},
			{Action: footerToggle, Labels: [footerLabelVariants]string{"Enter toggle", "Toggle", "Tog"}},
		}
	}
	if a.detail {
		items := []footerItem{
			{Action: footerBack, Labels: [footerLabelVariants]string{"Back(Esc/q)", "Back", "B"}},
			{Action: footerTabs, Labels: [footerLabelVariants]string{"1-5/h/l tabs", "Tabs", "T"}},
			{Action: footerWindow, Labels: [footerLabelVariants]string{"[ ] window", "Window", "W"}},
			{Action: footerScroll, Labels: [footerLabelVariants]string{"j/k scroll", "Scroll", "J"}},
			{Action: footerSettings, Labels: [footerLabelVariants]string{"s settings", "Settings", "S"}},
			{Action: footerRefresh, Labels: [footerLabelVariants]string{"r refresh", "Refresh", "R"}},
		}
		if a.update.Available {
			items = append(items, footerItem{Action: footerUpdate, Labels: [footerLabelVariants]string{"u update", "Update", "U"}})
		}
		return items
	}
	items := []footerItem{
		{Action: footerSelect, Labels: [footerLabelVariants]string{"↑↓/jk select", "Select", "J"}},
		{Action: footerOpen, Labels: [footerLabelVariants]string{"Enter detail", "Detail", "O"}},
		{Action: footerSettings, Labels: [footerLabelVariants]string{"s settings", "Settings", "S"}},
		{Action: footerMode, Labels: [footerLabelVariants]string{"m mode", "Mode", "M"}},
		{Action: footerRefresh, Labels: [footerLabelVariants]string{"r refresh", "Refresh", "R"}},
		{Action: footerASCII, Labels: [footerLabelVariants]string{"a ascii", "ASCII", "A"}},
		{Action: footerQuit, Labels: [footerLabelVariants]string{"q quit", "Quit", "Q"}},
	}
	if a.update.Available {
		items = append(items, footerItem{Action: footerUpdate, Labels: [footerLabelVariants]string{"u update", "Update", "U"}})
	}
	return items
}

type footerAction string

const (
	footerNone     footerAction = ""
	footerSelect   footerAction = "select"
	footerOpen     footerAction = "open"
	footerSettings footerAction = "settings"
	footerMode     footerAction = "mode"
	footerRefresh  footerAction = "refresh"
	footerASCII    footerAction = "ascii"
	footerQuit     footerAction = "quit"
	footerBack     footerAction = "back"
	footerAdjust   footerAction = "adjust"
	footerToggle   footerAction = "toggle"
	footerTabs     footerAction = "tabs"
	footerWindow   footerAction = "window"
	footerScroll   footerAction = "scroll"
	footerUpdate   footerAction = "update"
)

const footerLabelVariants = 3

type footerItem struct {
	Action footerAction
	Labels [footerLabelVariants]string
}

func footerTextForVariant(items []footerItem, variant int) string {
	if len(items) == 0 {
		return " "
	}
	if variant < 0 {
		variant = 0
	}
	if variant >= footerLabelVariants {
		variant = footerLabelVariants - 1
	}
	sep := "   "
	if variant == 1 {
		sep = "  "
	} else if variant == 2 {
		sep = " "
	}
	labels := make([]string, 0, len(items))
	for _, item := range items {
		labels = append(labels, item.Labels[variant])
	}
	return " " + strings.Join(labels, sep) + " "
}

func (a *App) footerActionAt(x int, width int) footerAction {
	if x <= 0 {
		return footerNone
	}
	items := a.footerItems()
	variant := a.footerVariantForWidth(width)
	pos := 2
	for _, item := range items {
		label := item.Labels[variant]
		end := pos + displayWidth(label) - 1
		if x >= pos && x <= end {
			return item.Action
		}
		pos = end + 1
		if variant == 0 {
			pos += 3
		} else if variant == 1 {
			pos += 2
		} else {
			pos++
		}
	}
	return footerNone
}

func (a *App) footerVariantForWidth(width int) int {
	if width <= 0 {
		return 0
	}
	items := a.footerItems()
	for variant := 0; variant < footerLabelVariants; variant++ {
		if displayWidth(footerTextForVariant(items, variant)) <= width {
			return variant
		}
	}
	return footerLabelVariants - 1
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
