package tui

import (
	"context"
	"strings"
	"time"

	"ktui/internal/komari"
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
		a.detailScroll += delta * a.detailScrollStep()
		if a.detailScroll < 0 {
			a.detailScroll = 0
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
			a.cancelSearch()
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
			}
		case footerPrevChart:
			a.moveChartFocus(-1)
		case footerNextChart:
			a.moveChartFocus(1)
		case footerTabs:
			a.cycleDetailTab(ctx, 1)
		case footerWindow:
			a.cycleDetailWindow(ctx, 1)
		case footerScroll:
			a.detailScroll += a.detailScrollStep()
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
	_, height := terminalSize()
	bodyHeight := height - mouseHeaderRows - 1
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	bodyRow := y - mouseHeaderRows - 1
	a.selectSettingsAtBodyRow(bodyRow, bodyHeight)
}

func (a *App) selectSettingsAtBodyRow(bodyRow int, bodyHeight int) {
	if bodyRow < 0 {
		return
	}
	chromeRows := a.settingsChromeRows()
	visibleRows := bodyHeight - chromeRows
	if bodyRow < chromeRows || visibleRows <= 0 {
		return
	}
	a.adjustSettingsScroll(visibleRows, a.settingsCount())
	itemRow := a.settingsScroll + bodyRow - chromeRows
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
	index := a.listScroll + row
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
	nodes := a.viewNodes()
	layout := sheetBodyMetricsFor(drawWidth, bodyHeight, len(nodes), a.listScroll)
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
	columnSpan := layout.CardWidth + sheetCardGap
	col := x0 / columnSpan
	if col < 0 || col >= layout.Columns {
		return
	}
	if x0%columnSpan >= layout.CardWidth {
		return
	}
	if row >= layout.RowsVisible {
		return
	}
	index := (a.listScroll+row)*layout.Columns + col
	if index >= 0 && index < len(nodes) {
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
	a.detailScroll = 0
	a.ensureSelectedDetail(ctx)
}

func (a *App) clickDetailBack(x int, y int) bool {
	_, height := terminalSize()
	if y != height || x > detailBackClickWidth {
		return false
	}
	a.detail = false
	return true
}

func (a *App) openSettings() {
	a.settingsWasDetail = a.detail
	a.settings = true
	a.detail = false
	a.closeChartFocus()
}

func (a *App) closeSettings() {
	a.settings = false
	a.detail = a.settingsWasDetail
	a.settingsWasDetail = false
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
	node, ok := a.selectedNode()
	if !ok {
		return
	}
	st := a.snapshot.Status[node.UUID]
	_, bodyHeight := detailMouseBody()
	layout := a.detailChromeMouseLayout(node, st, bodyHeight)
	bodyRow := y - mouseHeaderRows - 1
	switch bodyRow {
	case layout.TabRow:
		if tab, ok := hitDetailTab(x); ok {
			a.tab = tab
			a.detailScroll = 0
			if a.tabNeedsDetail() {
				a.ensureSelectedDetail(ctx)
			}
		}
	case layout.WindowRow:
		if window, ok := hitDetailWindow(x); ok {
			a.window = window
			a.detailScroll = 0
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
	width, bodyHeight := detailMouseBody()
	drawWidth := width
	if drawWidth > 1 {
		drawWidth--
	}
	bodyRow := y - mouseHeaderRows - 1
	node, ok := a.selectedNode()
	if !ok {
		return 0, false
	}
	st := a.snapshot.Status[node.UUID]
	layout := a.detailChromeMouseLayout(node, st, bodyHeight)
	contentHeight := bodyHeight - layout.Rows
	if contentHeight < 1 {
		return 0, false
	}
	cardHeight := detailCardHeightFor(contentHeight)
	if contentHeight >= cardHeight {
		contentHeight = contentHeight / cardHeight * cardHeight
	}
	contentRow := bodyRow - layout.Rows
	if contentRow < 0 || contentRow >= contentHeight {
		return 0, false
	}
	contentRow += a.detailScroll
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

type detailChromeMouseLayout struct {
	Rows      int
	TabRow    int
	WindowRow int
}

func detailMouseBody() (int, int) {
	width, height := terminalSize()
	bodyHeight := height - mouseHeaderRows - 1
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	return width, bodyHeight
}

func (a *App) detailChromeMouseLayout(node komari.Node, st komari.Status, bodyHeight int) detailChromeMouseLayout {
	tabRow := 2
	if len(a.alertForNode(node, st, time.Now()).Reasons) > 0 {
		tabRow++
	}
	windowRow := tabRow + 1
	rows := windowRow + 1
	if bodyHeight > 0 && rows > bodyHeight-1 {
		rows = max(0, bodyHeight-1)
	}
	if tabRow >= rows {
		tabRow = -1
	}
	if windowRow >= rows {
		windowRow = -1
	}
	return detailChromeMouseLayout{Rows: rows, TabRow: tabRow, WindowRow: windowRow}
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

type sheetBodyMetrics struct {
	ContentWidth int
	Columns      int
	CardWidth    int
	RowsVisible  int
	TotalRows    int
	Indicator    scrollIndicator
}

func sheetBodyMetricsFor(width int, bodyHeight int, nodeCount int, offset int) sheetBodyMetrics {
	rowsVisible := max(1, bodyHeight/sheetCardHeight)
	columns, cardWidth := sheetLayout(width)
	totalRows := sheetRows(nodeCount, columns)
	indicator := scrollIndicator{
		Start:   0,
		Height:  bodyHeight,
		Offset:  offset,
		Visible: rowsVisible,
		Total:   totalRows,
	}
	contentWidth := scrollContentWidth(width, indicator)
	if contentWidth != width {
		columns, cardWidth = sheetLayout(contentWidth)
		totalRows = sheetRows(nodeCount, columns)
		indicator.Total = totalRows
	}
	return sheetBodyMetrics{
		ContentWidth: contentWidth,
		Columns:      columns,
		CardWidth:    cardWidth,
		RowsVisible:  rowsVisible,
		TotalRows:    totalRows,
		Indicator:    indicator,
	}
}

func sheetRows(nodeCount int, columns int) int {
	return (nodeCount + max(1, columns) - 1) / max(1, columns)
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
