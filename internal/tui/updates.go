package tui

import (
	"fmt"
	"strings"
)

func (a *App) hasAvailableUpdate() bool {
	return a.update.Available || a.komariUpdate.Available
}

func (a *App) isCheckingUpdates() bool {
	return a.update.Checking || a.komariUpdate.Checking
}

func (a *App) updateHeaderText() string {
	switch {
	case a.update.Available && a.komariUpdate.Available:
		return fmt.Sprintf("UPDATES ktui %s, Komari %s available  press u", valueOr(a.update.Latest, "latest"), valueOr(a.komariUpdate.Latest, "latest"))
	case a.update.Available:
		return fmt.Sprintf("KTUI UPDATE %s available  run `ktui update install`", valueOr(a.update.Latest, "latest"))
	case a.komariUpdate.Available:
		return fmt.Sprintf("KOMARI UPDATE %s available  press u", valueOr(a.komariUpdate.Latest, "latest"))
	default:
		return ""
	}
}

func (a *App) updateHintText() string {
	parts := []string{}
	if a.update.Available {
		parts = append(parts, "ktui: "+valueOr(a.update.Latest, "latest")+" available  run `ktui update install`")
	}
	if a.komariUpdate.Available {
		current := valueOr(a.komariUpdate.Current, valueOr(a.snapshot.Version.Version, "current"))
		latest := valueOr(a.komariUpdate.Latest, "latest")
		message := "Komari server: " + current + " -> " + latest + "  update from GitHub releases"
		if a.komariUpdate.ReleaseURL != "" {
			message += " " + a.komariUpdate.ReleaseURL
		}
		parts = append(parts, message)
	}
	return strings.Join(parts, "  |  ")
}
