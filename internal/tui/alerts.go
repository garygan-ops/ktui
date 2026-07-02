package tui

import (
	"strings"
	"time"

	"ktui/internal/komari"
)

type nodeAlert struct {
	Critical bool
	Warning  bool
	Reasons  []string
}

func (a *App) alertForNode(node komari.Node, st komari.Status, now time.Time) nodeAlert {
	var alert nodeAlert
	if !st.Online {
		alert.Critical = true
		alert.Reasons = append(alert.Reasons, "offline")
	}
	if st.CPU >= a.warnCPU {
		alert.Critical = true
		alert.Reasons = append(alert.Reasons, "cpu")
	}
	if percent(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal)) >= a.warnRAM {
		alert.Critical = true
		alert.Reasons = append(alert.Reasons, "ram")
	}
	if percent(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal)) >= a.warnDisk {
		alert.Critical = true
		alert.Reasons = append(alert.Reasons, "disk")
	}
	if node.ExpiredAt.Valid && node.Price >= 0 {
		until := node.ExpiredAt.Time.Sub(now)
		if until < 0 {
			alert.Critical = true
			alert.Reasons = append(alert.Reasons, "expired")
		} else if until <= time.Duration(a.warnExpiryDays)*24*time.Hour {
			alert.Warning = true
			alert.Reasons = append(alert.Reasons, "expires")
		}
	}
	return alert
}

func (a *App) styleAlertLine(line string, alert nodeAlert) string {
	switch {
	case alert.Critical:
		return a.style.red(line)
	case alert.Warning:
		return a.style.yellow(line)
	default:
		return line
	}
}

func alertText(alert nodeAlert) string {
	if len(alert.Reasons) == 0 {
		return "ok"
	}
	return strings.Join(alert.Reasons, " ")
}
