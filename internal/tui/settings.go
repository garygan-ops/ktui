package tui

import (
	"fmt"
	"strings"
	"time"
)

type settingsItem struct {
	Label string
	Value string
	Kind  settingsItemKind
}

type settingsItemKind int

const (
	settingsURL settingsItemKind = iota
	settingsAPIKey
	settingsInterval
	settingsTimeout
	settingsMode
	settingsRealtimePoints
	settingsChartYAxis
	settingsASCII
	settingsNoColor
)

func (a *App) renderSettingsBody(width int, bodyHeight int) []string {
	items := a.settingsItems()
	if len(items) == 0 {
		return fillBody([]string{" No settings"}, width, bodyHeight)
	}
	if a.settingsSelected < 0 {
		a.settingsSelected = 0
	}
	if a.settingsSelected >= len(items) {
		a.settingsSelected = len(items) - 1
	}

	lines := make([]string, 0, bodyHeight)
	lines = append(lines, "")
	if a.settingsStatus != "" {
		lines = append(lines, " "+a.settingsStatus)
	}
	for i, item := range items {
		prefix := " "
		if i == a.settingsSelected {
			prefix = ">"
		}
		line := fmt.Sprintf(" %s %-22s %s", prefix, item.Label, item.Value)
		if i == a.settingsSelected {
			line = a.style.inverse(cleanLine(line, width))
		}
		lines = append(lines, fitLine(line, width))
	}
	return fillBody(lines, width, bodyHeight)
}

func (a *App) settingsItems() []settingsItem {
	return []settingsItem{
		{Label: "url", Value: valueOr(a.settingsURL, "-"), Kind: settingsURL},
		{Label: "api_key", Value: maskedValue(a.settingsAPIKey), Kind: settingsAPIKey},
		{Label: "interval", Value: a.refreshInterval.String(), Kind: settingsInterval},
		{Label: "timeout", Value: a.fetchTimeout.String(), Kind: settingsTimeout},
		{Label: "mode", Value: string(a.mode), Kind: settingsMode},
		{Label: "realtime_points", Value: a.realtimePointsText(), Kind: settingsRealtimePoints},
		{Label: "chart_y_axis", Value: a.chartYAxisModeText(), Kind: settingsChartYAxis},
		{Label: "ascii", Value: boolText(a.style.ASCII), Kind: settingsASCII},
		{Label: "no_color", Value: boolText(a.style.NoColor), Kind: settingsNoColor},
	}
}

func (a *App) realtimePointsText() string {
	if a.realtimePoints <= 0 {
		return fmt.Sprintf("auto (%d)", a.maxRealtimeSamples())
	}
	return fmt.Sprintf("%d", a.realtimePoints)
}

func (a *App) chartYAxisModeText() string {
	if a.chartYAxisMode == chartYAxisRelative {
		return "relative"
	}
	return "absolute"
}

func (a *App) settingsCount() int {
	return len(a.settingsItems())
}

func (a *App) moveSettingsSelection(delta int) {
	count := a.settingsCount()
	if count == 0 {
		a.settingsSelected = 0
		return
	}
	a.settingsSelected += delta
	if a.settingsSelected < 0 {
		a.settingsSelected = 0
	}
	if a.settingsSelected >= count {
		a.settingsSelected = count - 1
	}
}

func (a *App) adjustSelectedSetting(delta int) {
	items := a.settingsItems()
	if a.settingsSelected < 0 || a.settingsSelected >= len(items) {
		return
	}
	switch items[a.settingsSelected].Kind {
	case settingsURL, settingsAPIKey:
		a.settingsStatus = "read only"
		return
	case settingsInterval:
		next := adjustedDuration(a.refreshInterval, []time.Duration{
			2 * time.Second,
			5 * time.Second,
			10 * time.Second,
			30 * time.Second,
			time.Minute,
		}, delta)
		if next != a.refreshInterval {
			a.refreshInterval = next
			a.intervalChanged = true
		}
	case settingsTimeout:
		timeout := adjustedDuration(a.fetchTimeout, []time.Duration{
			5 * time.Second,
			10 * time.Second,
			15 * time.Second,
			20 * time.Second,
			30 * time.Second,
			time.Minute,
		}, delta)
		a.fetchTimeout = timeout
		a.detailTimeout = timeout
	case settingsMode:
		if a.mode == ModeLine {
			a.mode = ModeSheet
		} else {
			a.mode = ModeLine
		}
	case settingsRealtimePoints:
		a.realtimePoints = adjustedRealtimePoints(a.realtimePoints, delta)
	case settingsChartYAxis:
		a.toggleChartYAxisMode()
	case settingsASCII:
		a.style.ASCII = !a.style.ASCII
	case settingsNoColor:
		a.style.NoColor = !a.style.NoColor
	}
	a.persistSettings()
}

func (a *App) persistSettings() {
	if a.saveSettings == nil {
		a.settingsStatus = "runtime only"
		return
	}
	err := a.saveSettings(PersistentSettings{
		Interval:       a.refreshInterval.String(),
		Timeout:        a.fetchTimeout.String(),
		Mode:           string(a.mode),
		RealtimePoints: a.realtimePoints,
		ChartYAxisMode: string(a.chartYAxisMode),
		ASCII:          a.style.ASCII,
		NoColor:        a.style.NoColor,
	})
	if err != nil {
		a.settingsStatus = "save failed: " + err.Error()
		return
	}
	a.settingsStatus = "saved"
}

func adjustedDuration(current time.Duration, presets []time.Duration, delta int) time.Duration {
	if len(presets) == 0 {
		return current
	}
	if delta == 0 {
		delta = 1
	}
	if delta > 0 {
		for _, preset := range presets {
			if preset > current {
				return preset
			}
		}
		return presets[len(presets)-1]
	}
	for i := len(presets) - 1; i >= 0; i-- {
		if presets[i] < current {
			return presets[i]
		}
	}
	return presets[0]
}

func adjustedRealtimePoints(current int, delta int) int {
	presets := []int{0, 30, 60, 120, 150, 300, 600, 1200}
	if delta == 0 {
		delta = 1
	}
	if delta > 0 {
		for _, preset := range presets {
			if preset > current {
				return preset
			}
		}
		return presets[len(presets)-1]
	}
	for i := len(presets) - 1; i >= 0; i-- {
		if presets[i] < current {
			return presets[i]
		}
	}
	return presets[0]
}

func (a *App) toggleChartYAxisMode() {
	if a.chartYAxisMode == chartYAxisRelative {
		a.chartYAxisMode = chartYAxisAbsolute
		return
	}
	a.chartYAxisMode = chartYAxisRelative
}

func maskedValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return "********"
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
