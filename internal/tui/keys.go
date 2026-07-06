package tui

import (
	"context"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

func (a *App) readKeys(ctx context.Context) {
	buf := make([]byte, 8)
	pending := make([]byte, 0, 32)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := os.Stdin.Read(buf)
		if err != nil {
			if err == io.EOF {
				continue
			}
			return
		}
		if n == 0 {
			if len(pending) > 0 {
				a.enqueueKeys(flushPendingKeys(pending))
				pending = pending[:0]
			}
			continue
		}
		pending = append(pending, buf[:n]...)
		keys, rest := parseKeysWithRemainder(pending)
		pending = rest
		a.enqueueKeys(keys)
	}
}

func (a *App) enqueueKeys(keys []keyEvent) {
	for _, key := range keys {
		select {
		case a.keyCh <- key:
		default:
		}
	}
}

func parseKeys(data []byte) []keyEvent {
	keys, rest := parseKeysWithRemainder(data)
	if len(rest) > 0 {
		keys = append(keys, flushPendingKeys(rest)...)
	}
	return keys
}

func parseKeysWithRemainder(data []byte) ([]keyEvent, []byte) {
	out := make([]keyEvent, 0, len(data))
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case 3:
			out = append(out, keyEvent{name: "force-quit"})
		case 'q', 'Q':
			out = append(out, keyEvent{name: "quit", text: string(data[i])})
		case 'r', 'R':
			out = append(out, keyEvent{name: "refresh", text: string(data[i])})
		case '\r', '\n':
			out = append(out, keyEvent{name: "open"})
		case 'd', 'D':
			out = append(out, keyEvent{name: "detail-refresh", text: string(data[i])})
		case 'f', 'F':
			out = append(out, keyEvent{name: "chart-focus", text: string(data[i])})
		case 'a', 'A':
			out = append(out, keyEvent{name: "ascii", text: string(data[i])})
		case 'o', 'O':
			out = append(out, keyEvent{name: "open", text: string(data[i])})
		case 'b', 'B':
			out = append(out, keyEvent{name: "back", text: string(data[i])})
		case 0x7f:
			out = append(out, keyEvent{name: "backspace"})
		case 'm', 'M':
			out = append(out, keyEvent{name: "mode", text: string(data[i])})
		case 's', 'S':
			out = append(out, keyEvent{name: "settings", text: string(data[i])})
		case 'u', 'U':
			out = append(out, keyEvent{name: "update-hint", text: string(data[i])})
		case '/':
			out = append(out, keyEvent{name: "search", text: "/"})
		case 'c', 'C':
			out = append(out, keyEvent{name: "sort", text: string(data[i])})
		case 'v', 'V':
			out = append(out, keyEvent{name: "filter", text: string(data[i])})
		case 'j':
			out = append(out, keyEvent{name: "down", text: string(data[i])})
		case 'k':
			out = append(out, keyEvent{name: "up", text: string(data[i])})
		case 'h':
			out = append(out, keyEvent{name: "tab-left", text: string(data[i])})
		case 'l':
			out = append(out, keyEvent{name: "tab-right", text: string(data[i])})
		case '1', '2', '3', '4', '5':
			out = append(out, keyEvent{name: "tab-" + string(data[i]), text: string(data[i])})
		case '\t':
			out = append(out, keyEvent{name: "tab-right"})
		case 'g':
			out = append(out, keyEvent{name: "top", text: string(data[i])})
		case 'G':
			out = append(out, keyEvent{name: "bottom", text: string(data[i])})
		case '[':
			out = append(out, keyEvent{name: "window-left", text: string(data[i])})
		case ']':
			out = append(out, keyEvent{name: "window-right", text: string(data[i])})
		case 0x1b:
			if i+1 >= len(data) {
				return out, append([]byte(nil), data[i:]...)
			}
			if data[i+1] == '[' {
				if i+2 >= len(data) {
					return out, append([]byte(nil), data[i:]...)
				}
				if data[i+2] == '<' {
					key, next, ok, incomplete := parseSGRMouse(data, i)
					if incomplete {
						return out, append([]byte(nil), data[i:]...)
					}
					if ok {
						out = append(out, key)
						i = next - 1
					}
					continue
				}
				switch data[i+2] {
				case 'A':
					out = append(out, keyEvent{name: "up"})
					i += 2
				case 'B':
					out = append(out, keyEvent{name: "down"})
					i += 2
				case 'C':
					out = append(out, keyEvent{name: "tab-right"})
					i += 2
				case 'D':
					out = append(out, keyEvent{name: "tab-left"})
					i += 2
				case '5':
					if i+3 >= len(data) {
						return out, append([]byte(nil), data[i:]...)
					}
					out = append(out, keyEvent{name: "pageup"})
					if data[i+3] == '~' {
						i += 3
					} else {
						i += 2
					}
				case '6':
					if i+3 >= len(data) {
						return out, append([]byte(nil), data[i:]...)
					}
					out = append(out, keyEvent{name: "pagedown"})
					if data[i+3] == '~' {
						i += 3
					} else {
						i += 2
					}
				}
			} else {
				out = append(out, keyEvent{name: "back"})
			}
		default:
			if data[i] >= utf8.RuneSelf {
				if !utf8.FullRune(data[i:]) {
					return out, append([]byte(nil), data[i:]...)
				}
				r, size := utf8.DecodeRune(data[i:])
				if r != utf8.RuneError {
					out = append(out, keyEvent{name: "char", text: string(r)})
					i += size - 1
				}
			} else if data[i] >= 32 && data[i] != 0x7f {
				out = append(out, keyEvent{name: "char", text: string(data[i])})
			}
		}
	}
	return out, nil
}

func flushPendingKeys(data []byte) []keyEvent {
	if len(data) == 0 {
		return nil
	}
	if data[0] == 0x1b {
		return []keyEvent{{name: "back"}}
	}
	keys, _ := parseKeysWithRemainder(data)
	return keys
}

func parseSGRMouse(data []byte, start int) (keyEvent, int, bool, bool) {
	i := start + 3
	fieldStart := i
	values := make([]int, 0, 3)
	for ; i < len(data); i++ {
		switch data[i] {
		case ';':
			value, ok := parseMouseNumber(data[fieldStart:i])
			if !ok {
				return keyEvent{}, i + 1, false, false
			}
			values = append(values, value)
			fieldStart = i + 1
		case 'M', 'm':
			value, ok := parseMouseNumber(data[fieldStart:i])
			if !ok {
				return keyEvent{}, i + 1, false, false
			}
			values = append(values, value)
			if len(values) != 3 {
				return keyEvent{}, i + 1, false, false
			}
			return mouseKey(values[0], values[1], values[2], data[i] == 'm'), i + 1, true, false
		}
	}
	return keyEvent{}, len(data), false, true
}

func parseMouseNumber(data []byte) (int, bool) {
	if len(data) == 0 {
		return 0, false
	}
	value, err := strconv.Atoi(string(data))
	if err != nil {
		return 0, false
	}
	return value, true
}

func mouseKey(button int, x int, y int, release bool) keyEvent {
	switch button & 0b11_000000 {
	case 64:
		if button&1 == 0 {
			return keyEvent{name: "mouse-wheel-up", x: x, y: y}
		}
		return keyEvent{name: "mouse-wheel-down", x: x, y: y}
	}
	if !release && button&0b11 == 0 {
		return keyEvent{name: "mouse-left", x: x, y: y}
	}
	return keyEvent{name: "mouse-ignore", x: x, y: y}
}

func (a *App) handleKey(ctx context.Context, key keyEvent) {
	previous := a.selected
	previousTab := a.tab
	previousWindow := a.window
	if isMouseKey(key.name) {
		a.handleMouse(ctx, key)
		a.clampSelection()
		if a.listScroll < 0 {
			a.listScroll = 0
		}
		if a.detailScroll < 0 {
			a.detailScroll = 0
		}
		if a.settingsScroll < 0 {
			a.settingsScroll = 0
		}
		return
	}
	if a.searchEditing {
		a.handleSearchKey(key)
		return
	}
	if a.settings {
		a.handleSettingsKey(key)
		return
	}
	switch key.name {
	case "force-quit":
		a.quit = true
	case "quit":
		if a.chartFocus {
			a.closeChartFocus()
		} else if a.detail {
			a.detail = false
		} else if a.clearListScope() {
			// q backs out of a narrowed list before it quits the app.
		} else {
			a.quit = true
		}
	case "back", "backspace":
		if a.chartFocus {
			a.closeChartFocus()
		} else if a.detail {
			a.detail = false
		} else {
			a.clearListScope()
		}
	case "settings":
		a.openSettings()
	case "update-hint":
		a.showUpdateHint()
	case "search":
		if !a.detail {
			a.openSearch()
		}
	case "sort":
		if !a.detail {
			a.cycleNodeSort()
		}
	case "filter":
		if !a.detail {
			a.cycleNodeFilter()
		}
	case "open":
		if a.chartFocus {
			a.closeChartFocus()
		} else if a.detail {
			a.focusChart(a.chartFocusIndex)
		} else if _, ok := a.selectedNode(); ok {
			a.detail = true
			a.detailScroll = 0
			a.ensureSelectedDetail(ctx)
		}
	case "chart-focus":
		a.focusChart(0)
	case "refresh":
		a.requestFullRefresh()
		if a.detail {
			if node, ok := a.selectedNode(); ok {
				a.fetchDetail(ctx, node.UUID, true)
			}
		}
	case "detail-refresh":
		if node, ok := a.selectedNode(); ok {
			a.detail = true
			a.detailScroll = 0
			a.fetchDetail(ctx, node.UUID, true)
		}
	case "ascii":
		a.style.ASCII = !a.style.ASCII
	case "mode":
		if !a.detail {
			if a.mode == ModeLine {
				a.mode = ModeSheet
			} else {
				a.mode = ModeLine
			}
		}
	case "up":
		if a.chartFocus {
			// no-op; focused charts use h/l for chart selection.
		} else if a.detail {
			a.detailScroll -= a.detailScrollStep()
		} else {
			a.selected--
		}
	case "down":
		if a.chartFocus {
			// no-op; focused charts use h/l for chart selection.
		} else if a.detail {
			a.detailScroll += a.detailScrollStep()
		} else {
			a.selected++
		}
	case "pageup":
		if a.chartFocus {
			a.moveChartFocus(-1)
		} else if a.detail {
			a.detailScroll -= a.detailScrollStep() * 3
		} else {
			a.selected -= 10
		}
	case "pagedown":
		if a.chartFocus {
			a.moveChartFocus(1)
		} else if a.detail {
			a.detailScroll += a.detailScrollStep() * 3
		} else {
			a.selected += 10
		}
	case "top":
		if a.detail {
			a.detailScroll = 0
		} else {
			a.selected = 0
		}
	case "bottom":
		if a.detail {
			a.detailScroll = 1 << 30
		} else {
			a.selected = len(a.viewNodes()) - 1
		}
	case "tab-left":
		if a.chartFocus {
			a.moveChartFocus(-1)
		} else if a.detail {
			a.cycleDetailTab(ctx, -1)
		}
	case "tab-right":
		if a.chartFocus {
			a.moveChartFocus(1)
		} else if a.detail {
			a.cycleDetailTab(ctx, 1)
		}
	case "tab-1", "tab-2", "tab-3", "tab-4", "tab-5":
		if a.detail {
			a.tab = int(key.name[len(key.name)-1] - '1')
			a.detailScroll = 0
			a.closeChartFocus()
		}
	case "window-left":
		if a.chartFocus {
			a.cycleDetailWindow(ctx, -1)
		} else if a.detail {
			a.cycleDetailWindow(ctx, -1)
		}
	case "window-right":
		if a.chartFocus {
			a.cycleDetailWindow(ctx, 1)
		} else if a.detail {
			a.cycleDetailWindow(ctx, 1)
		}
	}
	a.clampSelection()
	if a.listScroll < 0 {
		a.listScroll = 0
	}
	if a.detailScroll < 0 {
		a.detailScroll = 0
	}
	if a.detail && (previous != a.selected || previousTab != a.tab || previousWindow != a.window || a.tabNeedsDetail()) {
		a.ensureSelectedDetail(ctx)
	}
}

func (a *App) openSearch() {
	a.searchEditing = true
	a.searchDraft = a.searchQuery
	a.searchAnchorUUID = a.selectedNodeUUID()
	a.notice = ""
}

func (a *App) handleSearchKey(key keyEvent) {
	selected := a.selectedNodeUUID()
	if selected == "" {
		selected = a.searchAnchorUUID
	}
	switch key.name {
	case "force-quit":
		a.quit = true
	case "open":
		if key.text == "" {
			a.searchQuery = strings.TrimSpace(a.searchDraft)
			a.searchDraft = a.searchQuery
			a.searchEditing = false
			a.searchAnchorUUID = ""
		} else {
			a.searchDraft += key.text
		}
	case "back":
		if key.text == "" {
			a.cancelSearch()
			return
		} else {
			a.searchDraft += key.text
		}
	case "backspace":
		a.searchDraft = dropLastRune(a.searchDraft)
	default:
		if key.text != "" {
			a.searchDraft += key.text
		}
	}
	a.restoreSelection(selected)
}

func (a *App) cancelSearch() {
	selected := a.searchAnchorUUID
	if selected == "" {
		selected = a.selectedNodeUUID()
	}
	a.searchDraft = a.searchQuery
	a.searchEditing = false
	a.searchAnchorUUID = ""
	a.restoreSelection(selected)
}

func dropLastRune(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	return string(runes[:len(runes)-1])
}

func isMouseKey(name string) bool {
	switch name {
	case "mouse-left", "mouse-wheel-up", "mouse-wheel-down", "mouse-ignore":
		return true
	default:
		return false
	}
}

func (a *App) handleSettingsKey(key keyEvent) {
	switch key.name {
	case "force-quit":
		a.quit = true
	case "quit", "back", "settings":
		a.closeSettings()
	case "up":
		a.moveSettingsSelection(-1)
	case "down":
		a.moveSettingsSelection(1)
	case "top":
		a.settingsSelected = 0
		a.settingsScroll = 0
	case "bottom":
		a.settingsSelected = max(0, a.settingsCount()-1)
		a.settingsScroll = 1 << 30
	case "tab-left", "window-left":
		a.adjustSelectedSetting(-1)
	case "tab-right", "window-right", "open":
		a.adjustSelectedSetting(1)
	case "ascii":
		a.style.ASCII = !a.style.ASCII
		a.persistSettings()
	}
}

func (a *App) cycleDetailTab(ctx context.Context, delta int) {
	if len(tabNames) == 0 {
		return
	}
	a.tab = (a.tab + delta + len(tabNames)) % len(tabNames)
	a.detailScroll = 0
	a.closeChartFocus()
	if a.detail && a.tabNeedsDetail() {
		a.ensureSelectedDetail(ctx)
	}
}

func (a *App) cycleDetailWindow(ctx context.Context, delta int) {
	if len(detailWindows) == 0 {
		return
	}
	a.window = (a.window + delta + len(detailWindows)) % len(detailWindows)
	a.detailScroll = 0
	if a.detail {
		a.ensureSelectedDetail(ctx)
	}
}

func (a *App) tabNeedsDetail() bool {
	return a.tab == 2 || a.tab == 3
}

func (a *App) detailScrollStep() int {
	if a.cardStep > 0 {
		return a.cardStep
	}
	return detailCardHeight
}

func (a *App) clampSelection() {
	count := len(a.viewNodes())
	if count == 0 {
		a.selected = 0
		a.listScroll = 0
		return
	}
	if a.selected < 0 {
		a.selected = 0
	}
	if a.selected >= count {
		a.selected = count - 1
	}
}
