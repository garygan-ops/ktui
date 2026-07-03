package tui

import (
	"context"
	"strings"
)

const (
	mouseHeaderRows      = 4
	sheetCardHeight      = 10
	sheetCardGap         = 2
	sheetMinWidth        = 40
	lineHeaderRows       = 3
	detailBackClickWidth = 14
)

func (a *App) handleMouse(ctx context.Context, key keyEvent) {
	switch key.name {
	case "mouse-wheel-up":
		a.handleMouseWheel(ctx, -1)
	case "mouse-wheel-down":
		a.handleMouseWheel(ctx, 1)
	case "mouse-left":
		a.handleMouseClick(ctx, key.x, key.y)
	}
}

func (a *App) handleMouseWheel(ctx context.Context, delta int) {
	previous := a.selected
	previousTab := a.tab
	previousWindow := a.window
	if a.settings {
		a.moveSettingsSelection(delta)
		return
	}
	if a.detail {
		a.scroll += delta * a.detailScrollStep()
		if a.scroll < 0 {
			a.scroll = 0
		}
		if previous != a.selected || previousTab != a.tab || previousWindow != a.window || a.tabNeedsDetail() {
			a.ensureSelectedDetail(ctx)
		}
		return
	}
	a.selected += delta
	a.clampSelection()
	if previous != a.selected && a.detail {
		a.ensureSelectedDetail(ctx)
	}
}

func (a *App) handleMouseClick(ctx context.Context, x int, y int) {
	if x <= 0 {
		return
	}
	if a.clickFooter(ctx, x, y) {
		return
	}
	if a.settings {
		a.clickSettings(y)
		return
	}
	if a.detail {
		if a.clickDetailBack(x, y) {
			return
		}
		if a.chartFocus {
			return
		}
		if y <= mouseHeaderRows {
			return
		}
		if a.clickDetailChart(x, y) {
			return
		}
		a.clickDetailChrome(ctx, x, y)
		return
	}
	if y <= mouseHeaderRows {
		return
	}
	if rows := a.listSearchRows(); rows > 0 {
		if y <= mouseHeaderRows+rows {
			a.openSearch()
			return
		}
	}
	if len(a.viewNodes()) == 0 {
		return
	}
	if a.mode == ModeLine {
		a.clickLine(ctx, y)
		return
	}
	a.clickSheet(ctx, x, y)
}

func (a *App) clickFooter(ctx context.Context, x int, y int) bool {
	width, height := terminalSize()
	if y != height {
		return false
	}
	drawWidth := width
	if drawWidth > 1 {
		drawWidth--
	}
	action := a.footerActionAt(x, drawWidth)
	if a.searchEditing {
		if action == footerBack {
			a.searchDraft = a.searchQuery
			a.searchEditing = false
			return true
		}
		return false
	}
	if a.settings {
		switch action {
		case footerBack:
			a.closeSettings()
		case footerAdjust, footerToggle:
			a.adjustSelectedSetting(1)
		default:
			return false
		}
		return true
	}
	if a.detail {
		switch action {
		case footerBack:
			if a.chartFocus {
				a.closeChartFocus()
			} else {
				a.detail = false
				a.scroll = 0
			}
		case footerPrevChart:
			a.moveChartFocus(-1)
		case footerNextChart:
			a.moveChartFocus(1)
		case footerSettings:
			a.openSettings()
		case footerRefresh:
			a.requestFullRefresh()
			if node, ok := a.selectedNode(); ok {
				a.fetchDetail(ctx, node.UUID, true)
			}
		case footerUpdate:
			a.showUpdateHint()
		default:
			return false
		}
		return true
	}
	switch action {
	case footerOpen:
		a.openSelectedDetail(ctx)
	case footerSearch:
		a.openSearch()
	case footerSort:
		a.cycleNodeSort()
	case footerFilter:
		a.cycleNodeFilter()
	case footerSettings:
		a.openSettings()
	case footerMode:
		if a.mode == ModeLine {
			a.mode = ModeSheet
		} else {
			a.mode = ModeLine
		}
	case footerRefresh:
		a.requestFullRefresh()
	case footerASCII:
		a.style.ASCII = !a.style.ASCII
	case footerQuit:
		a.quit = true
	case footerUpdate:
		a.showUpdateHint()
	default:
		return false
	}
	return true
}

func (a *App) clickSettings(y int) {
	bodyRow := y - mouseHeaderRows - 1
	if bodyRow < 0 {
		return
	}
	itemRow := bodyRow - 1
	if a.settingsStatus != "" {
		itemRow--
	}
	if itemRow < 0 || itemRow >= a.settingsCount() {
		return
	}
	a.settingsSelected = itemRow
}

func (a *App) clickLine(ctx context.Context, y int) {
	bodyRow := y - mouseHeaderRows - 1 - a.listSearchRows()
	row := bodyRow - lineHeaderRows
	if row < 0 {
		return
	}
	index := a.scroll + row
	if index >= 0 && index < len(a.viewNodes()) {
		a.selected = index
		a.clampSelection()
		a.openSelectedDetail(ctx)
	}
}

func (a *App) clickSheet(ctx context.Context, x int, y int) {
	width, height := terminalSize()
	drawWidth := width
	if drawWidth > 1 {
		drawWidth--
	}
	bodyHeight := height - mouseHeaderRows - 1
	bodyHeight -= a.listSearchRows()
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	columns, cardWidth := sheetLayout(drawWidth)
	bodyRow := y - mouseHeaderRows - 1 - a.listSearchRows()
	if bodyRow < 0 {
		return
	}
	row := bodyRow / sheetCardHeight
	lineInCard := bodyRow % sheetCardHeight
	if lineInCard >= sheetCardHeight {
		return
	}
	x0 := x - 1
	columnSpan := cardWidth + sheetCardGap
	col := x0 / columnSpan
	if col < 0 || col >= columns {
		return
	}
	if x0%columnSpan >= cardWidth {
		return
	}
	rowsVisible := max(1, bodyHeight/sheetCardHeight)
	if row >= rowsVisible {
		return
	}
	index := (a.scroll+row)*columns + col
	if index >= 0 && index < len(a.viewNodes()) {
		a.selected = index
		a.clampSelection()
		a.openSelectedDetail(ctx)
	}
}

func (a *App) openSelectedDetail(ctx context.Context) {
	if len(a.viewNodes()) == 0 {
		return
	}
	a.detail = true
	a.scroll = 0
	a.ensureSelectedDetail(ctx)
}

func (a *App) clickDetailBack(x int, y int) bool {
	_, height := terminalSize()
	if y != height || x > detailBackClickWidth {
		return false
	}
	a.detail = false
	a.scroll = 0
	return true
}

func (a *App) openSettings() {
	a.settingsWasDetail = a.detail
	a.settings = true
	a.detail = false
	a.closeChartFocus()
	a.scroll = 0
}

func (a *App) closeSettings() {
	a.settings = false
	a.detail = a.settingsWasDetail
	a.settingsWasDetail = false
	a.scroll = 0
}

func (a *App) showUpdateHint() {
	if !a.update.Available {
		return
	}
	latest := valueOr(a.update.Latest, "latest")
	a.notice = "update available: " + latest + "  quit and run `ktui update`"
}

func footerHit(footer string, label string, x int) bool {
	start, end, ok := footerLabelBounds(footer, label)
	return ok && x >= start && x <= end
}

func footerLabelBounds(footer string, label string) (int, int, bool) {
	index := strings.Index(footer, label)
	if index < 0 {
		return 0, 0, false
	}
	start := displayWidth(footer[:index]) + 1
	end := start + displayWidth(label) - 1
	return start, end, true
}

func (a *App) clickDetailChrome(ctx context.Context, x int, y int) {
	bodyRow := y - mouseHeaderRows - 1
	switch bodyRow {
	case 2:
		if tab, ok := hitDetailTab(x); ok {
			a.tab = tab
			a.scroll = 0
			if a.tabNeedsDetail() {
				a.ensureSelectedDetail(ctx)
			}
		}
	case 3:
		if window, ok := hitDetailWindow(x); ok {
			a.window = window
			a.scroll = 0
			a.ensureSelectedDetail(ctx)
		}
	}
}

func (a *App) clickDetailChart(x int, y int) bool {
	index, ok := a.chartIndexAt(x, y)
	if !ok {
		return false
	}
	return a.focusChart(index)
}

func (a *App) chartIndexAt(x int, y int) (int, bool) {
	if len(a.viewNodes()) == 0 {
		return 0, false
	}
	width, height := terminalSize()
	drawWidth := width
	if drawWidth > 1 {
		drawWidth--
	}
	bodyHeight := height - mouseHeaderRows - 1
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	chromeRows := 4
	contentHeight := bodyHeight - chromeRows
	if contentHeight < 1 {
		return 0, false
	}
	cardHeight := detailCardHeightFor(contentHeight)
	if contentHeight >= cardHeight {
		contentHeight = contentHeight / cardHeight * cardHeight
	}
	bodyRow := y - mouseHeaderRows - 1
	contentRow := bodyRow - chromeRows
	if contentRow < 0 || contentRow >= contentHeight {
		return 0, false
	}
	contentRow += a.scroll
	columns, cardWidth := detailGridLayout(drawWidth, detailGridColumns(drawWidth))
	row := contentRow / cardHeight
	lineInCard := contentRow % cardHeight
	if lineInCard >= cardHeight {
		return 0, false
	}
	x0 := x - 1
	columnSpan := cardWidth + detailGridGap
	col := x0 / columnSpan
	if col < 0 || col >= columns {
		return 0, false
	}
	if x0%columnSpan >= cardWidth {
		return 0, false
	}
	sectionIndex := row*columns + col
	node, ok := a.selectedNode()
	if !ok {
		return 0, false
	}
	st := a.snapshot.Status[node.UUID]
	sections := a.detailSections(node, st)
	if sectionIndex < 0 || sectionIndex >= len(sections) || sections[sectionIndex].Chart == nil {
		return 0, false
	}
	chartIndex := 0
	for i := 0; i < sectionIndex; i++ {
		if sections[i].Chart != nil {
			chartIndex++
		}
	}
	return chartIndex, true
}

func sheetLayout(width int) (int, int) {
	columns := max(1, (width+sheetCardGap)/(sheetMinWidth+sheetCardGap))
	for columns > 1 {
		cardWidth := (width - sheetCardGap*(columns-1)) / columns
		if cardWidth >= sheetMinWidth {
			break
		}
		columns--
	}
	cardWidth := width
	if columns > 1 {
		cardWidth = (width - sheetCardGap*(columns-1)) / columns
	}
	return columns, cardWidth
}

func hitDetailTab(x int) (int, bool) {
	pos := 1
	for i, name := range tabNames {
		width := len(" ") + len("1 ") + len(name) + len(" ")
		if x >= pos && x < pos+width {
			return i, true
		}
		pos += width + 1
	}
	return 0, false
}

func hitDetailWindow(x int) (int, bool) {
	pos := len(" window ") + 1
	for i, window := range detailWindows {
		width := len(" ") + len(window.Label) + len(" ")
		if x >= pos && x < pos+width {
			return i, true
		}
		pos += width + 1
	}
	return 0, false
}
