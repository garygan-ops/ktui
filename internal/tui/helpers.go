package tui

import (
	"fmt"
	"strings"
	"time"

	"ktui/internal/komari"
)

func maxCountForWindow(hours int) int {
	switch {
	case hours <= 4:
		return 480
	case hours <= 24:
		return 720
	case hours <= 24*7:
		return 1000
	default:
		return 1200
	}
}

func limitRawLines(lines []string, maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	if len(lines) > maxLines {
		return lines[:maxLines]
	}
	return lines
}

func coreText(node komari.Node) string {
	switch {
	case node.CPUCores > 0 && node.CPUPhysicalCores > 0:
		return fmt.Sprintf("%d/%d", node.CPUCores, node.CPUPhysicalCores)
	case node.CPUCores > 0:
		return fmt.Sprintf("%d", node.CPUCores)
	case node.CPUPhysicalCores > 0:
		return fmt.Sprintf("%d", node.CPUPhysicalCores)
	default:
		return "-"
	}
}

func shortID(value string) string {
	if len(value) <= 8 {
		return valueOr(value, "-")
	}
	return value[:8]
}

func (a *App) authText() string {
	if a.snapshot.Me.LoggedIn {
		return "auth:" + valueOr(a.snapshot.Me.Username, "api-key")
	}
	return "guest"
}

func firstNonZero(a, b int64) int64 {
	if a != 0 {
		return a
	}
	return b
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func nullableDate(t komari.NullTime) string {
	if !t.Valid {
		return "-"
	}
	return shortDate(t.Time)
}

func expiryText(node komari.Node, now time.Time) string {
	if node.Price < 0 {
		return "free"
	}
	if !node.ExpiredAt.Valid {
		return "-"
	}
	until := node.ExpiredAt.Time.Sub(now)
	if until < 0 {
		return "expired"
	}
	if until >= 20*365*24*time.Hour {
		return "lifetime"
	}
	if until < 24*time.Hour {
		return "today"
	}
	days := int(until.Hours() / 24)
	if until > time.Duration(days)*24*time.Hour {
		days++
	}
	return fmt.Sprintf("%dd", days)
}

func nullableDateTime(t komari.NullTime) string {
	if !t.Valid {
		return "-"
	}
	return t.Time.Local().Format("01-02 15:04")
}

func timeText(t komari.NullTime) string {
	if !t.Valid {
		return "-"
	}
	return t.Time.Local().Format("2006-01-02 15:04:05")
}

func shortTimeFromNull(t komari.NullTime) string {
	if !t.Valid {
		return "-"
	}
	return shortTime(t.Time)
}

func fitBar(width int, reserved int) int {
	barWidth := width - reserved
	if barWidth < 8 {
		return 8
	}
	if barWidth > 28 {
		return 28
	}
	return barWidth
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
