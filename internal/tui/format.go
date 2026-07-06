package tui

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

var marqueeState struct {
	frame   int
	enabled bool
}

type Style struct {
	ASCII   bool
	NoColor bool
}

func setMarquee(frame int, enabled bool) {
	marqueeState.frame = frame
	marqueeState.enabled = enabled
}

func bytesIEC(n int64) string {
	negative := n < 0
	if negative {
		n = -n
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	value := float64(n)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}

	var out string
	switch {
	case unit == 0:
		out = fmt.Sprintf("%d %s", n, units[unit])
	case value >= 100:
		out = fmt.Sprintf("%.0f %s", value, units[unit])
	case value >= 10:
		out = fmt.Sprintf("%.1f %s", value, units[unit])
	default:
		out = fmt.Sprintf("%.2f %s", value, units[unit])
	}
	if negative {
		return "-" + out
	}
	return out
}

func speedIEC(n int64) string {
	return bytesIEC(n) + "/s"
}

func percent(used, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(used) / float64(total) * 100
}

func usage(used, total int64) string {
	if total <= 0 {
		return "-"
	}
	return fmt.Sprintf("%s / %s (%.1f%%)", bytesIEC(used), bytesIEC(total), percent(used, total))
}

func usageCompact(used, total int64) string {
	if total <= 0 {
		return "-"
	}
	return fmt.Sprintf("%s/%s", bytesIEC(used), bytesIEC(total))
}

func durationCompact(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	days := seconds / 86400
	seconds %= 86400
	hours := seconds / 3600
	seconds %= 3600
	minutes := seconds / 60

	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case minutes > 0:
		return fmt.Sprintf("%dm", minutes)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}

func shortTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("15:04:05")
}

func shortDate(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02")
}

func cleanLine(s string, width int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if width <= 0 {
		return ""
	}
	if strings.Contains(s, "\x1b[") {
		return cleanLineANSI(s, width)
	}
	if displayWidth(s) <= width {
		return s
	}
	runes := []rune(s)
	if width <= 1 {
		return string(runes[:min(len(runes), width)])
	}
	out := strings.Builder{}
	current := 0
	for _, r := range runes {
		w := runeWidth(r)
		if current+w > width-1 {
			break
		}
		out.WriteRune(r)
		current += w
	}
	out.WriteRune('…')
	return out.String()
}

func cleanLineANSI(s string, width int) string {
	if visibleWidth(s) <= width {
		return s
	}
	if width <= 1 {
		return ""
	}

	var out strings.Builder
	visible := 0
	ansiOpen := false
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			end := i + 2
			for end < len(s) && ((s[end] < 'A' || s[end] > 'Z') && (s[end] < 'a' || s[end] > 'z')) {
				end++
			}
			if end < len(s) {
				out.WriteString(s[i : end+1])
				ansiOpen = true
				i = end + 1
				continue
			}
		}

		r, size := rune(s[i]), 1
		if r >= 0x80 {
			r, size = utf8.DecodeRuneInString(s[i:])
		}
		rw := runeWidth(r)
		if visible+rw > width-1 {
			break
		}
		out.WriteRune(r)
		visible += rw
		i += size
	}
	out.WriteRune('…')
	if ansiOpen {
		out.WriteString("\x1b[0m")
	}
	return out.String()
}

func fitLine(s string, width int) string {
	if marqueeState.enabled && displayWidth(s) > width {
		s = marqueeLine(s, width, marqueeState.frame)
	}
	return padRight(cleanLine(s, width), width)
}

func padRight(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := displayWidth(s)
	if w >= width {
		return cleanLine(s, width)
	}
	return s + strings.Repeat(" ", width-w)
}

func (s Style) bar(value float64, width int) string {
	if width <= 0 {
		return ""
	}
	value = math.Max(0, math.Min(100, value))
	filled := int(math.Round(value / 100 * float64(width)))
	if filled > width {
		filled = width
	}
	if s.ASCII {
		return strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func (s Style) miniBar(value float64, width int) string {
	if width <= 0 {
		return ""
	}
	value = math.Max(0, math.Min(100, value))
	filled := int(math.Round(value / 100 * float64(width)))
	if filled > width {
		filled = width
	}
	if s.ASCII {
		return strings.Repeat("=", filled) + strings.Repeat(".", width-filled)
	}
	return strings.Repeat("●", filled) + strings.Repeat("·", width-filled)
}

func (s Style) up() string {
	if s.ASCII {
		return "up"
	}
	return "↑"
}

func (s Style) down() string {
	if s.ASCII {
		return "down"
	}
	return "↓"
}

func (s Style) separator(width int) string {
	if s.ASCII {
		return strings.Repeat("-", width)
	}
	return strings.Repeat("─", width)
}

func (s Style) boxTop(width int) string {
	if width <= 0 {
		return ""
	}
	if s.ASCII {
		return "+" + strings.Repeat("-", max(0, width-2)) + "+"
	}
	return "┌" + strings.Repeat("─", max(0, width-2)) + "┐"
}

func (s Style) boxBottom(width int) string {
	if width <= 0 {
		return ""
	}
	if s.ASCII {
		return "+" + strings.Repeat("-", max(0, width-2)) + "+"
	}
	return "└" + strings.Repeat("─", max(0, width-2)) + "┘"
}

func (s Style) boxLine(content string, width int) string {
	return s.boxLineWithBorder(content, width, func(value string) string { return value })
}

func (s Style) boxLineWithBorder(content string, width int, borderStyle func(string) string) string {
	if width <= 0 {
		return ""
	}
	if width == 1 {
		if s.ASCII {
			return borderStyle("|")
		}
		return borderStyle("│")
	}
	left, right := "│", "│"
	if s.ASCII {
		left, right = "|", "|"
	}
	return borderStyle(left) + fitLine(content, max(0, width-2)) + borderStyle(right)
}

func (s Style) vertical() string {
	if s.ASCII {
		return " | "
	}
	return " │ "
}

func (s Style) sanitizeText(value string) string {
	if !s.ASCII {
		return value
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r == '\n' || r == '\r':
			b.WriteByte(' ')
		case r >= 32 && r <= 126:
			b.WriteRune(r)
		case r >= 0x1f1e6 && r <= 0x1f1ff:
			// Drop regional indicator symbols used for flag emoji.
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}

func trafficPercent(up, down, limit int64, limitType string) float64 {
	if limit <= 0 {
		return 0
	}
	var used int64
	switch strings.ToLower(limitType) {
	case "max":
		if up > down {
			used = up
		} else {
			used = down
		}
	case "min":
		if up < down {
			used = up
		} else {
			used = down
		}
	case "up":
		used = up
	case "down":
		used = down
	default:
		used = up + down
	}
	return float64(used) / float64(limit) * 100
}

func trafficLimitKind(limitType string) string {
	switch strings.ToLower(strings.TrimSpace(limitType)) {
	case "up":
		return "Up"
	case "down":
		return "Down"
	case "max":
		return "Max"
	case "min":
		return "Min"
	default:
		return "Sum"
	}
}

func trafficLimitText(limit int64, limitType string) string {
	if limit <= 0 {
		return "-"
	}
	return fmt.Sprintf("%s(%s)", trafficLimitKind(limitType), trafficBytes(limit))
}

func trafficBytes(n int64) string {
	negative := n < 0
	if negative {
		n = -n
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	value := float64(n)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	var out string
	if unit == 0 {
		out = fmt.Sprintf("%d %s", n, units[unit])
	} else {
		out = fmt.Sprintf("%.2f %s", value, units[unit])
	}
	if negative {
		return "-" + out
	}
	return out
}

func displayWidth(s string) int {
	s = ansiRE.ReplaceAllString(s, "")
	width := 0
	for _, r := range s {
		width += runeWidth(r)
	}
	return width
}

func visibleWidth(s string) int {
	return displayWidth(s)
}

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

func marqueeLine(s string, width int, frame int) string {
	if width <= 0 {
		return ""
	}
	plain := stripANSI(strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", " "))
	if displayWidth(plain) <= width {
		return plain
	}
	if width <= 1 {
		return cleanLine(plain, width)
	}
	padded := plain + strings.Repeat(" ", max(3, width/4))
	total := displayWidth(padded)
	if total <= width {
		return plain
	}
	start := frame % total
	return sliceVisible(padded+padded, start, width)
}

func sliceVisible(s string, start int, width int) string {
	var out strings.Builder
	pos := 0
	written := 0
	for _, r := range s {
		rw := runeWidth(r)
		next := pos + rw
		if next <= start {
			pos = next
			continue
		}
		if written+rw > width {
			break
		}
		out.WriteRune(r)
		written += rw
		pos = next
	}
	return out.String()
}

func (s Style) inverse(value string) string {
	return s.wrapSGR("7", value)
}

func (s Style) bold(value string) string {
	return s.wrapSGR("1", value)
}

func (s Style) dim(value string) string {
	return s.wrapSGR("2", value)
}

func (s Style) red(value string) string {
	return s.wrapSGR("31", value)
}

func (s Style) green(value string) string {
	return s.wrapSGR("32", value)
}

func (s Style) yellow(value string) string {
	return s.wrapSGR("33", value)
}

func (s Style) blue(value string) string {
	return s.wrapSGR("34", value)
}

func (s Style) cyan(value string) string {
	return s.wrapSGR("36", value)
}

func (s Style) color256(code int, value string) string {
	return s.wrapSGR(fmt.Sprintf("38;5;%d", code), value)
}

func (s Style) bg256(code int, value string) string {
	return s.wrapSGR(fmt.Sprintf("48;5;%d", code), value)
}

func (s Style) wrapSGR(code string, value string) string {
	if s.NoColor {
		return value
	}
	prefix := "\x1b[" + code + "m"
	return prefix + strings.ReplaceAll(value, "\x1b[0m", "\x1b[0m"+prefix) + "\x1b[0m"
}

func (s Style) coloredStatus(online bool) string {
	if online {
		return s.green("● ONLINE")
	}
	return s.red("● OFFLINE")
}

func runeWidth(r rune) int {
	if r == 0 {
		return 0
	}
	if r < 32 || (r >= 0x7f && r < 0xa0) {
		return 0
	}
	if r >= 0x1100 &&
		(r <= 0x115f ||
			r == 0x2329 ||
			r == 0x232a ||
			(r >= 0x2e80 && r <= 0xa4cf) ||
			(r >= 0xac00 && r <= 0xd7a3) ||
			(r >= 0xf900 && r <= 0xfaff) ||
			(r >= 0xfe10 && r <= 0xfe19) ||
			(r >= 0xfe30 && r <= 0xfe6f) ||
			(r >= 0xff00 && r <= 0xff60) ||
			(r >= 0xffe0 && r <= 0xffe6) ||
			(r >= 0x1f300 && r <= 0x1faff)) {
		return 2
	}
	return 1
}
