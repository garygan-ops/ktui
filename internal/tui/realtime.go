package tui

import (
	"sort"
	"time"

	"ktui/internal/komari"
)

func (a *App) recordRealtimeSample(sampleTime time.Time) bool {
	if len(a.snapshot.Nodes) == 0 {
		return false
	}
	if sampleTime.IsZero() {
		sampleTime = time.Now()
	}
	a.advanceRealtimeNow(sampleTime)
	a.recordRealtimeSnapshot(a.snapshot, sampleTime)
	return true
}

func (a *App) recordRealtimeSnapshot(snapshot komari.Snapshot, sampleTime time.Time) {
	if a.realtimeStatus == nil {
		a.realtimeStatus = map[string][]komari.Status{}
	}
	if !sampleTime.IsZero() {
		a.advanceRealtimeNow(sampleTime)
	}
	seen := make(map[string]struct{}, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		st, ok := snapshot.Status[node.UUID]
		if !ok {
			continue
		}
		if !st.Time.Valid && !sampleTime.IsZero() {
			st.Time = komari.NullTime{Time: sampleTime, Valid: true}
		}
		seen[node.UUID] = struct{}{}
		a.appendRealtimeStatus(node.UUID, st)
	}
	for uuid := range a.realtimeStatus {
		if _, ok := seen[uuid]; !ok {
			delete(a.realtimeStatus, uuid)
		}
	}
}

func (a *App) appendRealtimeStatus(uuid string, st komari.Status) {
	records := appendStatusSample(a.realtimeStatus[uuid], st)
	a.realtimeStatus[uuid] = a.limitRealtimeRecords(records)
}

func (a *App) realtimeRecords(uuid string, recent []komari.Status, current komari.Status) []komari.Status {
	local := a.realtimeStatus[uuid]
	records := make([]komari.Status, 0, len(recent)+len(local)+1)
	records = append(records, recent...)
	records = append(records, local...)
	if hasStatusSample(current) {
		if !current.Time.Valid {
			current.Time = komari.NullTime{Time: a.realtimeNowOrTime(time.Now()), Valid: true}
		}
		records = append(records, current)
	}
	return a.limitRealtimeRecords(records)
}

func (a *App) limitRealtimeRecords(records []komari.Status) []komari.Status {
	sortStatusSamples(records)
	records = dedupeStatusSamples(records)
	limit := a.maxRealtimeSamples()
	if len(records) > limit {
		records = records[len(records)-limit:]
	}
	return records
}

func hasStatusSample(st komari.Status) bool {
	return st.Time.Valid ||
		st.Online ||
		st.CPU != 0 ||
		st.RAM != 0 ||
		st.Disk != 0 ||
		st.NetIn != 0 ||
		st.NetOut != 0 ||
		st.Load != 0 ||
		st.Process != 0 ||
		st.Connections != 0 ||
		len(st.Ping) > 0
}

func (a *App) advanceRealtimeNow(now time.Time) {
	if now.IsZero() {
		return
	}
	if a.realtimeNow.IsZero() || now.After(a.realtimeNow) {
		a.realtimeNow = now
	}
}

func (a *App) realtimeNowOrTime(fallback time.Time) time.Time {
	if !a.realtimeNow.IsZero() {
		return a.realtimeNow
	}
	return fallback
}

func (a *App) maxRealtimeSamples() int {
	if a.realtimePoints > 0 {
		if a.realtimePoints < 2 {
			return 2
		}
		if a.realtimePoints > maxRealtimeSamplesCap {
			return maxRealtimeSamplesCap
		}
		return a.realtimePoints
	}
	interval := a.refreshInterval
	if interval <= 0 {
		interval = defaultRefreshInterval
	}
	limit := int(realtimeWindowDuration / interval)
	if limit < 2 {
		return 2
	}
	if limit > maxRealtimeSamplesCap {
		return maxRealtimeSamplesCap
	}
	return limit
}

func sortStatusSamples(records []komari.Status) {
	sort.SliceStable(records, func(i, j int) bool {
		left := records[i].Time
		right := records[j].Time
		switch {
		case left.Valid && right.Valid:
			return left.Time.Before(right.Time)
		case left.Valid:
			return true
		case right.Valid:
			return false
		default:
			return false
		}
	})
}

func dedupeStatusSamples(records []komari.Status) []komari.Status {
	out := records[:0]
	for _, record := range records {
		out = appendStatusSample(out, record)
	}
	return out
}
