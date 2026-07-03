package tui

import (
	"strings"
	"unicode/utf8"
)

type scrollIndicator struct {
	Start   int
	Height  int
	Offset  int
	Visible int
	Total   int
}

func (a *App) withScrollIndicator(lines []string, width int, indicator scrollIndicator) []string {
	if width < 4 || indicator.Height <= 0 || indicator.Visible <= 0 || indicator.Total <= indicator.Visible {
		return lines
	}
	start := max(0, indicator.Start)
	end := min(len(lines), start+indicator.Height)
	if end <= start {
		return lines
	}

	out := append([]string(nil), lines...)
	height := end - start
	offset := indicator.Offset
	maxOffset := indicator.Total - indicator.Visible
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}

	thumbHeight := (indicator.Visible*height + indicator.Total - 1) / indicator.Total
	if thumbHeight < 1 {
		thumbHeight = 1
	}
	if thumbHeight > height {
		thumbHeight = height
	}
	thumbStart := 0
	if maxOffset > 0 && height > thumbHeight {
		thumbStart = offset * (height - thumbHeight) / maxOffset
	}

	track, thumb, up, down := a.scrollIndicatorGlyphs()
	moreAbove := offset > 0
	moreBelow := offset+indicator.Visible < indicator.Total
	for i := start; i < end; i++ {
		row := i - start
		mark := track
		if row >= thumbStart && row < thumbStart+thumbHeight {
			mark = thumb
		}
		if moreAbove && row == 0 {
			mark = up
		}
		if moreBelow && row == height-1 {
			mark = down
		}
		out[i] = fitLineNoEllipsis(out[i], width-1) + mark
	}
	return out
}

func (a *App) scrollIndicatorGlyphs() (string, string, string, string) {
	if a.style.ASCII {
		return a.style.dim("|"), a.style.cyan("#"), a.style.yellow("^"), a.style.yellow("v")
	}
	return a.style.dim("│"), a.style.cyan("█"), a.style.yellow("▲"), a.style.yellow("▼")
}

func fitLineNoEllipsis(value string, width int) string {
	if width <= 0 {
		return ""
	}
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	if strings.Contains(value, "\x1b[") {
		value = truncateANSINoEllipsis(value, width)
	} else {
		value = truncateNoEllipsis(value, width)
	}
	visible := displayWidth(value)
	if visible >= width {
		return value
	}
	return value + strings.Repeat(" ", width-visible)
}

func truncateNoEllipsis(value string, width int) string {
	if displayWidth(value) <= width {
		return value
	}
	var out strings.Builder
	visible := 0
	for _, r := range value {
		rw := runeWidth(r)
		if visible+rw > width {
			break
		}
		out.WriteRune(r)
		visible += rw
	}
	return out.String()
}

func truncateANSINoEllipsis(value string, width int) string {
	if visibleWidth(value) <= width {
		return value
	}
	var out strings.Builder
	visible := 0
	ansiOpen := false
	for i := 0; i < len(value); {
		if value[i] == 0x1b && i+1 < len(value) && value[i+1] == '[' {
			end := i + 2
			for end < len(value) && ((value[end] < 'A' || value[end] > 'Z') && (value[end] < 'a' || value[end] > 'z')) {
				end++
			}
			if end < len(value) {
				out.WriteString(value[i : end+1])
				ansiOpen = true
				i = end + 1
				continue
			}
		}

		r, size := rune(value[i]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(value[i:])
		}
		rw := runeWidth(r)
		if visible+rw > width {
			break
		}
		out.WriteRune(r)
		visible += rw
		i += size
	}
	if ansiOpen {
		out.WriteString("\x1b[0m")
	}
	return out.String()
}
