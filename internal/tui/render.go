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

	if a.detail {
		lines = append(lines, a.renderDetailBody(drawWidth, bodyHeight)...)
	} else if a.mode == ModeLine {
		lines = append(lines, a.renderLineBody(drawWidth, bodyHeight)...)
	} else {
		lines = append(lines, a.renderSheetBody(drawWidth, bodyHeight)...)
	}

	lines = append(lines, a.footerLine(drawWidth))
	a.writeFrame(&b, width, height, lines)
	fmt.Print(b.String())
}
