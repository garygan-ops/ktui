package tui

import (
	"context"
	"io"
	"os"
	"strconv"
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
			continue
		}
		pending = append(pending, buf[:n]...)
		keys, rest := parseKeysWithRemainder(pending)
		pending = rest
		for _, key := range keys {
			select {
			case a.keyCh <- key:
			default:
			}
		}
	}
}

func parseKeys(data []byte) []keyEvent {
	keys, _ := parseKeysWithRemainder(data)
	return keys
}

func parseKeysWithRemainder(data []byte) ([]keyEvent, []byte) {
	out := make([]keyEvent, 0, len(data))
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case 3:
			out = append(out, keyEvent{name: "force-quit"})
		case 'q', 'Q':
			out = append(out, keyEvent{name: "quit"})
		case 'r', 'R':
			out = append(out, keyEvent{name: "refresh"})
		case '\r', '\n':
			out = append(out, keyEvent{name: "open"})
		case 'd', 'D':
			out = append(out, keyEvent{name: "detail-refresh"})
		case 'a', 'A':
			out = append(out, keyEvent{name: "ascii"})
		case 'o', 'O':
			out = append(out, keyEvent{name: "open"})
		case 'b', 'B', 0x7f:
			out = append(out, keyEvent{name: "back"})
		case 'm', 'M':
			out = append(out, keyEvent{name: "mode"})
		case 's', 'S':
			out = append(out, keyEvent{name: "settings"})
		case 'u', 'U':
			out = append(out, keyEvent{name: "update-hint"})
		case 'j':
			out = append(out, keyEvent{name: "down"})
		case 'k':
			out = append(out, keyEvent{name: "up"})
		case 'h':
			out = append(out, keyEvent{name: "tab-left"})
		case 'l':
			out = append(out, keyEvent{name: "tab-right"})
		case '1', '2', '3', '4', '5':
			out = append(out, keyEvent{name: "tab-" + string(data[i])})
		case '\t':
			out = append(out, keyEvent{name: "tab-right"})
		case 'g':
			out = append(out, keyEvent{name: "top"})
		case 'G':
			out = append(out, keyEvent{name: "bottom"})
		case '[':
			out = append(out, keyEvent{name: "window-left"})
		case ']':
			out = append(out, keyEvent{name: "window-right"})
		case 0x1b:
			if i+2 < len(data) && data[i+1] == '[' {
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
					out = append(out, keyEvent{name: "pageup"})
					if i+3 < len(data) && data[i+3] == '~' {
						i += 3
					} else {
						i += 2
					}
				case '6':
					out = append(out, keyEvent{name: "pagedown"})
					if i+3 < len(data) && data[i+3] == '~' {
						i += 3
					} else {
						i += 2
					}
				}
			} else {
				out = append(out, keyEvent{name: "back"})
			}
		}
	}
	return out, nil
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
		if a.scroll < 0 {
			a.scroll = 0
		}
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
		if a.detail {
			a.detail = false
			a.scroll = 0
		} else {
			a.quit = true
		}
	case "back":
		if a.detail {
			a.detail = false
			a.scroll = 0
		}
	case "settings":
		a.openSettings()
	case "update-hint":
		a.showUpdateHint()
	case "open":
		if len(a.snapshot.Nodes) > 0 {
			a.detail = true
			a.scroll = 0
			a.ensureSelectedDetail(ctx)
		}
	case "refresh":
		a.requestFullRefresh()
		if a.detail && len(a.snapshot.Nodes) > 0 {
			a.fetchDetail(ctx, a.snapshot.Nodes[a.selected].UUID, true)
		}
	case "detail-refresh":
		if len(a.snapshot.Nodes) > 0 {
			a.detail = true
			a.scroll = 0
			a.fetchDetail(ctx, a.snapshot.Nodes[a.selected].UUID, true)
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
		if a.detail {
			a.scroll -= a.detailScrollStep()
		} else {
			a.selected--
		}
	case "down":
		if a.detail {
			a.scroll += a.detailScrollStep()
		} else {
			a.selected++
		}
	case "pageup":
		if a.detail {
			a.scroll -= a.detailScrollStep() * 3
		} else {
			a.selected -= 10
		}
	case "pagedown":
		if a.detail {
			a.scroll += a.detailScrollStep() * 3
		} else {
			a.selected += 10
		}
	case "top":
		if a.detail {
			a.scroll = 0
		} else {
			a.selected = 0
		}
	case "bottom":
		if a.detail {
			a.scroll = 1 << 30
		} else {
			a.selected = len(a.snapshot.Nodes) - 1
		}
	case "tab-left":
		if a.detail {
			a.tab = (a.tab + len(tabNames) - 1) % len(tabNames)
			a.scroll = 0
		}
	case "tab-right":
		if a.detail {
			a.tab = (a.tab + 1) % len(tabNames)
			a.scroll = 0
		}
	case "tab-1", "tab-2", "tab-3", "tab-4", "tab-5":
		if a.detail {
			a.tab = int(key.name[len(key.name)-1] - '1')
			a.scroll = 0
		}
	case "window-left":
		if a.detail {
			a.window = (a.window + len(detailWindows) - 1) % len(detailWindows)
			a.scroll = 0
		}
	case "window-right":
		if a.detail {
			a.window = (a.window + 1) % len(detailWindows)
			a.scroll = 0
		}
	}
	a.clampSelection()
	if a.scroll < 0 {
		a.scroll = 0
	}
	if a.detail && (previous != a.selected || previousTab != a.tab || previousWindow != a.window || a.tabNeedsDetail()) {
		a.ensureSelectedDetail(ctx)
	}
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
	case "bottom":
		a.settingsSelected = max(0, a.settingsCount()-1)
	case "tab-left", "window-left":
		a.adjustSelectedSetting(-1)
	case "tab-right", "window-right", "open":
		a.adjustSelectedSetting(1)
	case "ascii":
		a.style.ASCII = !a.style.ASCII
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
	count := len(a.snapshot.Nodes)
	if count == 0 {
		a.selected = 0
		a.scroll = 0
		return
	}
	if a.selected < 0 {
		a.selected = 0
	}
	if a.selected >= count {
		a.selected = count - 1
	}
}
