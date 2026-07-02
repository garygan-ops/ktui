package tui

import (
	"fmt"
	"strings"
)

func (a *App) render() {
	width, height := terminalSize()
	drawWidth := width
	if drawWidth > 1 {
		drawWidth--
	}
	setMarquee(a.marqueeFrame, drawWidth < 100)
	defer setMarquee(0, false)

	var b strings.Builder
	b.WriteString("\x1b[?25l")
	if drawWidth < 30 || height < 8 {
		lines := []string{
			"ktui",
			"Window too small.",
			"Resize to at least 30x8.",
			"q quits.",
		}
		a.writeFrame(&b, width, height, lines)
		fmt.Print(b.String())
		return
	}

	lines := make([]string, 0, height)
	lines = append(lines, a.headerLines(drawWidth)...)

	bodyHeight := height - len(lines) - 1
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	if a.settings {
		lines = append(lines, a.renderSettingsBody(drawWidth, bodyHeight)...)
	} else if a.detail {
		lines = append(lines, a.renderDetailBody(drawWidth, bodyHeight)...)
	} else {
		lines = append(lines, a.renderListBody(drawWidth, bodyHeight)...)
	}

	lines = append(lines, a.footerLine(drawWidth))
	a.writeFrame(&b, width, height, lines)
	fmt.Print(b.String())
}

func (a *App) renderListBody(width int, bodyHeight int) []string {
	if bodyHeight <= 0 {
		return nil
	}
	remaining := bodyHeight
	lines := make([]string, 0, bodyHeight)
	if a.listSearchVisible() {
		lines = append(lines, a.listSearchLine(width))
		remaining--
	}
	if remaining > 0 {
		if a.mode == ModeLine {
			lines = append(lines, a.renderLineBody(width, remaining)...)
		} else {
			lines = append(lines, a.renderSheetBody(width, remaining)...)
		}
	}
	return fillBody(lines, width, bodyHeight)
}
