package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"ktui/internal/komari"
)

type settingsItem struct {
	Label    string
	Value    string
	Kind     settingsItemKind
	ReadOnly bool
}

type settingsItemKind int

const (
	settingsProfile settingsItemKind = iota
	settingsRenameProfile
	settingsSite
	settingsURL
	settingsAPIKey
	settingsInterval
	settingsTimeout
	settingsMode
	settingsRealtimeWindow
	settingsChartYAxis
	settingsASCII
	settingsNoColor
	settingsWarnCPU
	settingsWarnRAM
	settingsWarnDisk
	settingsWarnExpiryDays
	settingsAbout
)

func (a *App) renderSettingsBody(width int, bodyHeight int) []string {
	items := a.settingsItems()
	if len(items) == 0 {
		return fillBody([]string{" No settings"}, width, bodyHeight)
	}
	a.clampSettingsSelection(len(items))

	lines := make([]string, 0, bodyHeight)
	lines = append(lines, "")
	if a.settingsStatus != "" {
		lines = append(lines, " "+a.settingsStatus)
	}
	if len(lines) > bodyHeight {
		return fillBody(lines[:max(0, bodyHeight)], width, bodyHeight)
	}

	visibleRows := bodyHeight - len(lines)
	if visibleRows <= 0 {
		return fillBody(lines, width, bodyHeight)
	}
	a.adjustSettingsScroll(visibleRows, len(items))

	end := min(len(items), a.settingsScroll+visibleRows)
	for i := a.settingsScroll; i < end; i++ {
		item := items[i]
		prefix := " "
		if i == a.settingsSelected {
			prefix = ">"
		}
		value := item.Value
		if item.ReadOnly {
			value = value + "  read only"
		}
		line := fmt.Sprintf(" %s %-22s %s", prefix, item.Label, value)
		if i == a.settingsSelected {
			line = a.style.inverse(cleanLine(line, width))
		}
		lines = append(lines, fitLine(line, width))
	}
	lines = fillBody(lines, width, bodyHeight)
	return a.withScrollIndicator(lines, width, scrollIndicator{
		Start:   a.settingsChromeRows(),
		Height:  visibleRows,
		Offset:  a.settingsScroll,
		Visible: visibleRows,
		Total:   len(items),
	})
}

func (a *App) settingsItems() []settingsItem {
	return []settingsItem{
		{Label: "profile", Value: a.profileSettingText(), Kind: settingsProfile, ReadOnly: len(a.profiles) <= 1},
		{Label: "rename_profile", Value: valueOr(a.profileName, "-"), Kind: settingsRenameProfile},
		{Label: "site", Value: valueOr(a.text(a.snapshot.Public.SiteName), "-"), Kind: settingsSite, ReadOnly: true},
		{Label: "url", Value: valueOr(a.settingsURL, "-"), Kind: settingsURL, ReadOnly: true},
		{Label: "api_key", Value: maskedValue(a.settingsAPIKey), Kind: settingsAPIKey, ReadOnly: true},
		{Label: "interval", Value: a.refreshInterval.String(), Kind: settingsInterval},
		{Label: "timeout", Value: a.fetchTimeout.String(), Kind: settingsTimeout},
		{Label: "mode", Value: string(a.mode), Kind: settingsMode},
		{Label: "realtime_window", Value: a.realtimeWindowText(), Kind: settingsRealtimeWindow},
		{Label: "chart_y_axis", Value: a.chartYAxisModeText(), Kind: settingsChartYAxis},
		{Label: "ascii", Value: boolText(a.style.ASCII), Kind: settingsASCII},
		{Label: "no_color", Value: boolText(a.style.NoColor), Kind: settingsNoColor},
		{Label: "warn_cpu", Value: percentSettingText(a.warnCPU), Kind: settingsWarnCPU},
		{Label: "warn_ram", Value: percentSettingText(a.warnRAM), Kind: settingsWarnRAM},
		{Label: "warn_disk", Value: percentSettingText(a.warnDisk), Kind: settingsWarnDisk},
		{Label: "warn_expiry_days", Value: fmt.Sprintf("%d", a.warnExpiryDays), Kind: settingsWarnExpiryDays},
		{Label: "about", Value: "open", Kind: settingsAbout},
	}
}

func (a *App) realtimeWindowText() string {
	return realtimeWindowText(a.realtimeWindowDuration())
}

func (a *App) profileSettingText() string {
	if len(a.profiles) <= 1 {
		return valueOr(a.profileName, "-")
	}
	return fmt.Sprintf("%s (%d/%d)", valueOr(a.profileName, "-"), a.profileIndex()+1, len(a.profiles))
}

func (a *App) profileIndex() int {
	for i, profile := range a.profiles {
		if profile.Name == a.profileName {
			return i
		}
	}
	return 0
}

func (a *App) adjustSelectedProfile(delta int) {
	if len(a.profiles) <= 1 {
		a.settingsStatus = "read only"
		return
	}
	if delta == 0 {
		delta = 1
	}
	current := a.profileIndex()
	next := (current + delta) % len(a.profiles)
	if next < 0 {
		next += len(a.profiles)
	}
	if next == current {
		return
	}
	if err := a.switchProfile(a.profiles[next]); err != nil {
		a.settingsStatus = "profile failed: " + err.Error()
		return
	}
	a.persistSettings()
}

func (a *App) switchProfile(profile ConnectionProfile) error {
	profile.Name = strings.TrimSpace(profile.Name)
	profile.URL = strings.TrimSpace(profile.URL)
	profile.APIKey = strings.TrimSpace(profile.APIKey)
	if profile.Name == "" {
		return fmt.Errorf("profile name is empty")
	}
	client, err := komari.NewClientWithOptions(profile.URL, komari.Options{APIKey: profile.APIKey, Timeout: a.fetchTimeout})
	if err != nil {
		return err
	}
	a.connectionVersion++
	a.client = client
	a.profileName = profile.Name
	a.settingsURL = client.BaseURL()
	a.settingsAPIKey = profile.APIKey
	if index := a.profileIndex(); index >= 0 && index < len(a.profiles) {
		a.profiles[index].URL = client.BaseURL()
		a.profiles[index].APIKey = profile.APIKey
	}
	a.selected = 0
	a.listScroll = 0
	a.detailScroll = 0
	a.aboutScroll = 0
	a.detail = false
	a.chartFocus = false
	a.searchEditing = false
	a.searchQuery = ""
	a.searchDraft = ""
	a.searchAnchorUUID = ""
	a.nodeFilter = nodeFilterAll
	a.nodeSort = nodeSortDefault
	a.snapshot = komari.Snapshot{}
	a.err = nil
	a.loading = true
	a.fetching = false
	a.refreshPending = false
	a.komariUpdate = komariUpdateState{}
	a.lastFullFetch = time.Time{}
	a.realtimeNow = time.Time{}
	a.nodeDetail = map[detailKey]nodeDetail{}
	a.realtimeStatus = map[string][]komari.Status{}
	a.invalidateViewNodesCache()
	a.requestFullRefresh()
	return nil
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

func (a *App) settingsChromeRows() int {
	rows := 1
	if a.settingsStatus != "" {
		rows++
	}
	return rows
}

func (a *App) clampSettingsSelection(count int) {
	if count <= 0 {
		a.settingsSelected = 0
		a.settingsScroll = 0
		return
	}
	if a.settingsSelected < 0 {
		a.settingsSelected = 0
	}
	if a.settingsSelected >= count {
		a.settingsSelected = count - 1
	}
	if a.settingsScroll < 0 {
		a.settingsScroll = 0
	}
	if a.settingsScroll >= count {
		a.settingsScroll = count - 1
	}
}

func (a *App) adjustSettingsScroll(visibleRows int, count int) {
	a.clampSettingsSelection(count)
	if visibleRows <= 0 || count <= 0 {
		a.settingsScroll = 0
		return
	}
	if a.settingsSelected < a.settingsScroll {
		a.settingsScroll = a.settingsSelected
	}
	if a.settingsSelected >= a.settingsScroll+visibleRows {
		a.settingsScroll = a.settingsSelected - visibleRows + 1
	}
	maxScroll := count - visibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	if a.settingsScroll > maxScroll {
		a.settingsScroll = maxScroll
	}
	if a.settingsScroll < 0 {
		a.settingsScroll = 0
	}
}

func (a *App) moveSettingsSelection(delta int) {
	count := a.settingsCount()
	if count == 0 {
		a.settingsSelected = 0
		a.settingsScroll = 0
		return
	}
	a.settingsSelected += delta
	a.clampSettingsSelection(count)
}

func (a *App) adjustSelectedSetting(delta int) {
	items := a.settingsItems()
	if a.settingsSelected < 0 || a.settingsSelected >= len(items) {
		return
	}
	switch items[a.settingsSelected].Kind {
	case settingsSite, settingsURL, settingsAPIKey:
		a.settingsStatus = "read only"
		return
	case settingsProfile:
		a.adjustSelectedProfile(delta)
	case settingsRenameProfile:
		a.beginRenameProfile()
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
	case settingsRealtimeWindow:
		a.realtimeWindow = adjustedRealtimeWindow(a.realtimeWindowDuration(), delta)
	case settingsChartYAxis:
		a.toggleChartYAxisMode()
	case settingsASCII:
		a.style.ASCII = !a.style.ASCII
	case settingsNoColor:
		a.style.NoColor = !a.style.NoColor
	case settingsWarnCPU:
		a.warnCPU = adjustedPercent(a.warnCPU, delta)
	case settingsWarnRAM:
		a.warnRAM = adjustedPercent(a.warnRAM, delta)
	case settingsWarnDisk:
		a.warnDisk = adjustedPercent(a.warnDisk, delta)
	case settingsWarnExpiryDays:
		a.warnExpiryDays = adjustedExpiryDays(a.warnExpiryDays, delta)
	case settingsAbout:
		a.openAbout()
		return
	}
	a.persistSettings()
}

func (a *App) beginRenameProfile() {
	if strings.TrimSpace(a.profileName) == "" {
		a.settingsStatus = "rename failed: profile name is empty"
		return
	}
	a.settingsRenamingProfile = true
	a.settingsProfileDraft = a.profileName
	a.updateRenameProfileStatus()
}

func (a *App) updateRenameProfileStatus() {
	a.settingsStatus = "rename profile: " + valueOr(a.settingsProfileDraft, "_") + "  Enter save  Esc cancel"
}

func (a *App) handleRenameProfileKey(key keyEvent) {
	switch key.name {
	case "force-quit":
		a.quit = true
	case "open":
		if key.text != "" {
			a.settingsProfileDraft += key.text
			a.updateRenameProfileStatus()
			return
		}
		a.finishRenameProfile()
	case "back":
		if key.text != "" {
			a.settingsProfileDraft += key.text
			a.updateRenameProfileStatus()
			return
		}
		a.settingsRenamingProfile = false
		a.settingsProfileDraft = ""
		a.settingsStatus = "rename canceled"
	case "backspace":
		a.settingsProfileDraft = dropLastRune(a.settingsProfileDraft)
		a.updateRenameProfileStatus()
	default:
		if key.text != "" {
			a.settingsProfileDraft += key.text
			a.updateRenameProfileStatus()
		}
	}
}

func (a *App) finishRenameProfile() {
	oldName := strings.TrimSpace(a.profileName)
	newName := strings.TrimSpace(a.settingsProfileDraft)
	if newName == "" {
		a.settingsStatus = "rename failed: profile name is required"
		return
	}
	if !validProfileNameText(newName) {
		a.settingsStatus = "rename failed: use a name without spaces or slashes"
		return
	}
	if newName == oldName {
		a.settingsRenamingProfile = false
		a.settingsProfileDraft = ""
		a.settingsStatus = "rename unchanged"
		return
	}
	for _, profile := range a.profiles {
		if profile.Name == newName {
			a.settingsStatus = "rename failed: profile already exists"
			return
		}
	}
	settings := a.currentPersistentSettings(oldName)
	settings.Profile = newName
	if err := a.savePersistentSettings(settings); err != nil {
		a.settingsStatus = "save failed: " + err.Error()
		return
	}
	for i := range a.profiles {
		if a.profiles[i].Name == oldName {
			a.profiles[i].Name = newName
			break
		}
	}
	a.profileName = newName
	a.settingsRenamingProfile = false
	a.settingsProfileDraft = ""
	a.settingsStatus = "renamed"
}

func validProfileNameText(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsSpace(r) || r == '/' || r == '\\' {
			return false
		}
	}
	return true
}

func (a *App) persistSettings() {
	if a.saveSettings == nil {
		a.settingsStatus = "runtime only"
		return
	}
	if err := a.savePersistentSettings(a.currentPersistentSettings("")); err != nil {
		a.settingsStatus = "save failed: " + err.Error()
		return
	}
	a.settingsStatus = "saved"
}

func (a *App) currentPersistentSettings(renameFrom string) PersistentSettings {
	return PersistentSettings{
		Profile:           a.profileName,
		RenameProfileFrom: renameFrom,
		Interval:          a.refreshInterval.String(),
		Timeout:           a.fetchTimeout.String(),
		Mode:              string(a.mode),
		RealtimeWindow:    realtimeWindowText(a.realtimeWindowDuration()),
		ChartYAxisMode:    string(a.chartYAxisMode),
		ASCII:             a.style.ASCII,
		NoColor:           a.style.NoColor,
		WarnCPU:           a.warnCPU,
		WarnRAM:           a.warnRAM,
		WarnDisk:          a.warnDisk,
		WarnExpiryDays:    a.warnExpiryDays,
	}
}

func (a *App) savePersistentSettings(settings PersistentSettings) error {
	if a.saveSettings == nil {
		return nil
	}
	return a.saveSettings(settings)
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

func adjustedRealtimeWindow(current time.Duration, delta int) time.Duration {
	return adjustedDuration(current, []time.Duration{
		time.Minute,
		5 * time.Minute,
		10 * time.Minute,
	}, delta)
}

func realtimeWindowText(value time.Duration) string {
	switch value {
	case time.Minute:
		return "1m"
	case 5 * time.Minute:
		return "5m"
	case 10 * time.Minute:
		return "10m"
	default:
		return "1m"
	}
}

func adjustedPercent(current float64, delta int) float64 {
	presets := []float64{50, 60, 70, 75, 80, 85, 90, 95, 100}
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

func adjustedExpiryDays(current int, delta int) int {
	presets := []int{1, 3, 7, 14, 30, 60, 90}
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

func percentSettingText(value float64) string {
	if value == float64(int(value)) {
		return fmt.Sprintf("%.0f%%", value)
	}
	return fmt.Sprintf("%.1f%%", value)
}
