package tui

import (
	"context"
	"strings"
)

const (
	mouseHeaderRows      = 4
	sheetCardHeight      = 9
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
		if y <= mouseHeaderRows {
			return
		}
		a.clickDetailChrome(ctx, x, y)
		return
	}
	if y <= mouseHeaderRows {
		return
	}
	if len(a.snapshot.Nodes) == 0 {
		return
	}
	if a.mode == ModeLine {
		a.clickLine(ctx, y)
		return
	}
	a.clickSheet(ctx, x, y)
}

func (a *App) clickFooter(ctx context.Context, x int, y int) bool {
	_, height := terminalSize()
	if y != height {
		return false
	}
	if a.settings {
		switch {
		case footerHit(a.footerText(), "Esc/q back", x):
			a.closeSettings()
		case footerHit(a.footerText(), "←→/h/l adjust", x), footerHit(a.footerText(), "Enter toggle", x):
			a.adjustSelectedSetting(1)
		default:
			return false
		}
		return true
	}
	if a.detail {
		switch {
		case footerHit(a.footerText(), "Back", x):
			a.detail = false
			a.scroll = 0
		case footerHit(a.footerText(), "s settings", x):
			a.openSettings()
		case footerHit(a.footerText(), "r refresh", x):
			a.requestFullRefresh()
			if len(a.snapshot.Nodes) > 0 {
				a.fetchDetail(ctx, a.snapshot.Nodes[a.selected].UUID, true)
			}
		case footerHit(a.footerText(), "u update", x):
			a.showUpdateHint()
		default:
			return false
		}
		return true
	}
	switch {
	case footerHit(a.footerText(), "Enter detail", x):
		a.openSelectedDetail(ctx)
	case footerHit(a.footerText(), "s settings", x):
		a.openSettings()
	case footerHit(a.footerText(), "m mode", x):
		if a.mode == ModeLine {
			a.mode = ModeSheet
		} else {
			a.mode = ModeLine
		}
	case footerHit(a.footerText(), "r refresh", x):
		a.requestFullRefresh()
	case footerHit(a.footerText(), "a ascii", x):
		a.style.ASCII = !a.style.ASCII
	case footerHit(a.footerText(), "q quit", x):
		a.quit = true
	case footerHit(a.footerText(), "u update", x):
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
	bodyRow := y - mouseHeaderRows - 1
	row := bodyRow - lineHeaderRows
	if row < 0 {
		return
	}
	index := a.scroll + row
	if index >= 0 && index < len(a.snapshot.Nodes) {
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
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	columns, cardWidth := sheetLayout(drawWidth)
	bodyRow := y - mouseHeaderRows - 1
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
	if index >= 0 && index < len(a.snapshot.Nodes) {
		a.selected = index
		a.clampSelection()
		a.openSelectedDetail(ctx)
	}
}

func (a *App) openSelectedDetail(ctx context.Context) {
	if len(a.snapshot.Nodes) == 0 {
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
