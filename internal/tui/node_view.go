package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"ktui/internal/komari"
)

func (a *App) viewNodes() []komari.Node {
	return a.viewNodesForFilter(a.nodeFilter)
}

func (a *App) viewNodesForFilter(filter nodeFilterMode) []komari.Node {
	nodes := make([]komari.Node, 0, len(a.snapshot.Nodes))
	query := a.currentSearchQuery()
	for _, node := range a.snapshot.Nodes {
		st := a.snapshot.Status[node.UUID]
		if query != "" && !a.nodeMatchesSearch(node, query) {
			continue
		}
		if !a.nodeMatchesFilterMode(filter, node, st) {
			continue
		}
		nodes = append(nodes, node)
	}
	a.sortNodes(nodes)
	return nodes
}

func (a *App) selectedNode() (komari.Node, bool) {
	nodes := a.viewNodes()
	if len(nodes) == 0 {
		return komari.Node{}, false
	}
	if a.selected < 0 {
		a.selected = 0
	}
	if a.selected >= len(nodes) {
		a.selected = len(nodes) - 1
	}
	return nodes[a.selected], true
}

func (a *App) selectedNodeUUID() string {
	node, ok := a.selectedNode()
	if !ok {
		return ""
	}
	return node.UUID
}

func (a *App) selectNodeUUID(uuid string) bool {
	if uuid == "" {
		return false
	}
	for i, node := range a.viewNodes() {
		if node.UUID == uuid {
			a.selected = i
			return true
		}
	}
	return false
}

func (a *App) restoreSelection(uuid string) {
	if !a.selectNodeUUID(uuid) {
		a.selected = 0
		a.scroll = 0
	}
	a.clampSelection()
}

func (a *App) currentSearchQuery() string {
	if a.searchEditing {
		return strings.TrimSpace(a.searchDraft)
	}
	return strings.TrimSpace(a.searchQuery)
}

func (a *App) nodeMatchesSearch(node komari.Node, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	fields := []string{
		node.Name,
		node.Region,
		node.Group,
		node.Tags,
		node.IPv4,
		node.IPv6,
		node.UUID,
		node.OS,
		node.Arch,
		node.PublicRemark,
		node.Remark,
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

func (a *App) nodeMatchesFilter(node komari.Node, st komari.Status) bool {
	return a.nodeMatchesFilterMode(a.nodeFilter, node, st)
}

func (a *App) nodeMatchesFilterMode(filter nodeFilterMode, node komari.Node, st komari.Status) bool {
	switch filter {
	case nodeFilterOffline:
		return !st.Online
	case nodeFilterExpiring:
		return a.nodeExpiring(node, time.Now())
	case nodeFilterHighLoad:
		return a.nodeHighLoad(node, st)
	default:
		return true
	}
}

func (a *App) sortNodes(nodes []komari.Node) {
	switch a.nodeSort {
	case nodeSortStatus:
		sort.SliceStable(nodes, func(i, j int) bool {
			left := a.snapshot.Status[nodes[i].UUID]
			right := a.snapshot.Status[nodes[j].UUID]
			if left.Online != right.Online {
				return !left.Online
			}
			return nodes[i].Name < nodes[j].Name
		})
	case nodeSortCPU:
		sort.SliceStable(nodes, func(i, j int) bool {
			left := a.snapshot.Status[nodes[i].UUID].CPU
			right := a.snapshot.Status[nodes[j].UUID].CPU
			if left == right {
				return nodes[i].Name < nodes[j].Name
			}
			return left > right
		})
	case nodeSortRAM:
		sort.SliceStable(nodes, func(i, j int) bool {
			left := a.nodeRAMPercent(nodes[i])
			right := a.nodeRAMPercent(nodes[j])
			if left == right {
				return nodes[i].Name < nodes[j].Name
			}
			return left > right
		})
	case nodeSortTraffic:
		sort.SliceStable(nodes, func(i, j int) bool {
			left := a.nodeTrafficTotal(nodes[i])
			right := a.nodeTrafficTotal(nodes[j])
			if left == right {
				return nodes[i].Name < nodes[j].Name
			}
			return left > right
		})
	case nodeSortExpiry:
		sort.SliceStable(nodes, func(i, j int) bool {
			left := nodes[i].ExpiredAt
			right := nodes[j].ExpiredAt
			if left.Valid != right.Valid {
				return left.Valid
			}
			if left.Valid && !left.Time.Equal(right.Time) {
				return left.Time.Before(right.Time)
			}
			return nodes[i].Name < nodes[j].Name
		})
	}
}

func (a *App) nodeRAMPercent(node komari.Node) float64 {
	st := a.snapshot.Status[node.UUID]
	return percent(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal))
}

func (a *App) nodeDiskPercent(node komari.Node) float64 {
	st := a.snapshot.Status[node.UUID]
	return percent(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal))
}

func (a *App) nodeTrafficTotal(node komari.Node) int64 {
	st := a.snapshot.Status[node.UUID]
	return st.NetTotalUp + st.NetTotalDown
}

func (a *App) nodeHighLoad(node komari.Node, st komari.Status) bool {
	return st.CPU >= a.warnCPU ||
		percent(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal)) >= a.warnRAM ||
		percent(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal)) >= a.warnDisk
}

func (a *App) nodeExpiring(node komari.Node, now time.Time) bool {
	if !node.ExpiredAt.Valid || a.warnExpiryDays <= 0 || node.Price < 0 {
		return false
	}
	return node.ExpiredAt.Time.Sub(now) <= time.Duration(a.warnExpiryDays)*24*time.Hour
}

func (a *App) cycleNodeSort() {
	selected := a.selectedNodeUUID()
	switch a.nodeSort {
	case nodeSortDefault, "":
		a.nodeSort = nodeSortStatus
	case nodeSortStatus:
		a.nodeSort = nodeSortCPU
	case nodeSortCPU:
		a.nodeSort = nodeSortRAM
	case nodeSortRAM:
		a.nodeSort = nodeSortTraffic
	case nodeSortTraffic:
		a.nodeSort = nodeSortExpiry
	default:
		a.nodeSort = nodeSortDefault
	}
	a.restoreSelection(selected)
}

func (a *App) cycleNodeFilter() {
	selected := a.selectedNodeUUID()
	order := []nodeFilterMode{nodeFilterAll, nodeFilterOffline, nodeFilterExpiring, nodeFilterHighLoad}
	current := a.normalizedNodeFilter()
	start := 0
	for i, filter := range order {
		if filter == current {
			start = i
			break
		}
	}

	for step := 1; step <= len(order); step++ {
		next := order[(start+step)%len(order)]
		if next != nodeFilterAll && len(a.viewNodesForFilter(next)) == 0 {
			continue
		}
		if next == current {
			a.notice = "no nodes match offline, expiring, or high-load filters"
			a.nodeFilter = nodeFilterAll
		} else {
			a.notice = ""
			a.nodeFilter = next
		}
		a.restoreSelection(selected)
		return
	}
	a.notice = "no nodes match offline, expiring, or high-load filters"
	a.nodeFilter = nodeFilterAll
	a.restoreSelection(selected)
}

func (a *App) normalizedNodeFilter() nodeFilterMode {
	if a.nodeFilter == "" {
		return nodeFilterAll
	}
	return a.nodeFilter
}

func (a *App) listScopeActive() bool {
	return strings.TrimSpace(a.searchQuery) != "" ||
		(a.nodeFilter != "" && a.nodeFilter != nodeFilterAll)
}

func (a *App) clearListScope() bool {
	if !a.listScopeActive() {
		return false
	}
	selected := a.selectedNodeUUID()
	a.searchEditing = false
	a.searchQuery = ""
	a.searchDraft = ""
	a.nodeFilter = nodeFilterAll
	a.restoreSelection(selected)
	return true
}

func (a *App) filterText() string {
	if a.nodeFilter == "" {
		return string(nodeFilterAll)
	}
	return string(a.nodeFilter)
}

func (a *App) sortText() string {
	if a.nodeSort == "" {
		return string(nodeSortDefault)
	}
	return string(a.nodeSort)
}

func (a *App) listTitle() string {
	visible := len(a.viewNodes())
	total := len(a.snapshot.Nodes)
	parts := []string{fmt.Sprintf("Servers (%d/%d nodes)", visible, total)}
	if query := a.currentSearchQuery(); query != "" {
		parts = append(parts, "search:"+query)
	}
	if a.nodeFilter != "" && a.nodeFilter != nodeFilterAll {
		parts = append(parts, "filter:"+a.filterText())
	}
	if a.nodeSort != "" && a.nodeSort != nodeSortDefault {
		parts = append(parts, "sort:"+a.sortText())
	}
	return strings.Join(parts, "  ")
}
