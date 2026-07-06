package tui

import (
	"fmt"
	"runtime"
	"strings"
)

func (a *App) renderAboutBody(width int, bodyHeight int) []string {
	if bodyHeight <= 0 {
		return nil
	}
	content := a.aboutContentLines(width)
	contentWidth := scrollContentWidth(width, scrollIndicator{
		Start:   0,
		Height:  bodyHeight,
		Offset:  a.aboutScroll,
		Visible: bodyHeight,
		Total:   len(content),
	})
	if contentWidth != width {
		content = a.aboutContentLines(contentWidth)
	}

	maxScroll := len(content) - bodyHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if a.aboutScroll > maxScroll {
		a.aboutScroll = maxScroll
	}
	if a.aboutScroll < 0 {
		a.aboutScroll = 0
	}

	end := min(len(content), a.aboutScroll+bodyHeight)
	lines := make([]string, 0, bodyHeight)
	lines = append(lines, content[a.aboutScroll:end]...)
	lines = fillBody(lines, contentWidth, bodyHeight)
	return a.withScrollIndicator(lines, width, scrollIndicator{
		Start:   0,
		Height:  bodyHeight,
		Offset:  a.aboutScroll,
		Visible: bodyHeight,
		Total:   len(content),
	})
}

func (a *App) aboutContentLines(width int) []string {
	lines := make([]string, 0, 36)
	addSection := func(title string) {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, a.style.bold(fitLine(" "+title, width)))
	}
	addKV := func(label string, value string) {
		lines = append(lines, a.aboutKVLine(label, value, width))
	}

	addSection("ktui")
	addKV("version", valueOr(a.appVersion, "dev"))
	addKV("commit", valueOr(a.appCommit, "none"))
	addKV("built", valueOr(a.appBuildDate, "unknown"))
	addKV("runtime", runtime.Version())
	addKV("platform", runtime.GOOS+"/"+runtime.GOARCH)

	addSection("Komari")
	addKV("site", valueOr(a.text(a.snapshot.Public.SiteName), "-"))
	addKV("url", valueOr(a.settingsURL, valueOr(a.snapshot.SourceURL, "-")))
	addKV("auth", a.authText())
	addKV("version", valueOr(a.snapshot.Version.Version, "-"))
	addKV("hash", valueOr(a.snapshot.Version.Hash, "-"))
	addKV("rpc", valueOr(a.snapshot.RPCVersion, "-"))
	addKV("nodes", fmt.Sprintf("%d/%d online", a.snapshot.Online, a.snapshot.Total))

	addSection("Connection")
	addKV("api_key", configuredText(a.settingsAPIKey))
	addKV("interval", a.refreshInterval.String())
	addKV("timeout", a.fetchTimeout.String())
	addKV("last_refresh", shortTime(a.snapshot.FetchedAt))
	addKV("status", a.aboutStatusText())

	addSection("Display")
	addKV("view_mode", string(a.mode))
	addKV("ascii", boolText(a.style.ASCII))
	addKV("no_color", boolText(a.style.NoColor))
	addKV("chart_y_axis", a.chartYAxisModeText())
	addKV("realtime_points", a.realtimePointsText())
	addKV("warnings", fmt.Sprintf("cpu %.0f%%  ram %.0f%%  disk %.0f%%  expiry %dd", a.warnCPU, a.warnRAM, a.warnDisk, a.warnExpiryDays))

	addSection("ktui Update")
	addKV("status", a.aboutUpdateText())
	if a.update.Latest != "" {
		addKV("latest", a.update.Latest)
	}
	if a.update.AssetName != "" {
		addKV("asset", a.update.AssetName)
	}

	addSection("Komari Update")
	addKV("status", a.aboutKomariUpdateText())
	if a.komariUpdate.Current != "" {
		addKV("current", a.komariUpdate.Current)
	}
	if a.komariUpdate.Latest != "" {
		addKV("latest", a.komariUpdate.Latest)
	}
	if a.komariUpdate.ReleaseCount > 0 {
		addKV("releases", fmt.Sprintf("%d newer", a.komariUpdate.ReleaseCount))
	}
	if a.komariUpdate.ReleaseURL != "" {
		addKV("github", a.komariUpdate.ReleaseURL)
	}

	addSection("Commands")
	for _, command := range []string{
		"ktui version",
		"ktui status",
		"ktui update check",
		"ktui help keys",
	} {
		lines = append(lines, fitLine(" "+command, width))
	}
	return lines
}

func (a *App) aboutKVLine(label string, value string, width int) string {
	label = strings.TrimSpace(label)
	value = strings.TrimSpace(value)
	if value == "" {
		value = "-"
	}
	prefix := fmt.Sprintf(" %-15s", label)
	return fitLine(a.style.dim(prefix)+value, width)
}

func (a *App) aboutStatusText() string {
	switch {
	case a.loading:
		return "loading"
	case a.fetching:
		return "refreshing"
	case a.err != nil:
		return "error: " + a.err.Error()
	default:
		return "ready"
	}
}

func (a *App) aboutUpdateText() string {
	switch {
	case a.update.Checking:
		return "checking"
	case a.update.Err != nil:
		return "check failed: " + a.update.Err.Error()
	case a.update.Available:
		return "available"
	case a.update.Checked:
		return "up to date"
	default:
		return "not checked"
	}
}

func (a *App) aboutKomariUpdateText() string {
	switch {
	case a.komariUpdate.Checking:
		return "checking"
	case a.komariUpdate.Err != nil:
		return "check failed: " + a.komariUpdate.Err.Error()
	case a.komariUpdate.Available:
		return "available"
	case a.komariUpdate.Checked:
		return "up to date"
	case strings.TrimSpace(a.snapshot.Version.Version) == "":
		return "waiting for server version"
	default:
		return "not checked"
	}
}

func configuredText(value string) string {
	if strings.TrimSpace(value) == "" {
		return "not configured"
	}
	return "configured"
}
