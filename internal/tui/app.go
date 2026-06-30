package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"ktui/internal/komari"
)

type App struct {
	client          *komari.Client
	refreshInterval time.Duration
	style           Style
	mode            Mode

	selected int
	scroll   int
	tab      int
	window   int
	detail   bool

	snapshot komari.Snapshot
	err      error
	loading  bool
	fetching bool
	quit     bool

	marqueeFrame int

	nodeDetail map[detailKey]nodeDetail

	renderCh  chan struct{}
	refreshCh chan struct{}
	resultCh  chan fetchResult
	detailCh  chan detailResult
	keyCh     chan keyEvent
}

type Options struct {
	RefreshInterval time.Duration
	ASCII           bool
	NoColor         bool
	Mode            Mode
}

type Mode string

const (
	ModeSheet Mode = "sheet"
	ModeLine  Mode = "line"
)

type nodeDetail struct {
	UUID      string
	Window    int
	Loading   bool
	Err       error
	FetchedAt time.Time
	Recent    komari.RecentStatusResp
	Load      komari.LoadRecordsResp
	Ping      komari.PingRecordsResp
}

type fetchResult struct {
	snapshot komari.Snapshot
	err      error
}

type detailResult struct {
	key    detailKey
	detail nodeDetail
}

type detailKey struct {
	UUID   string
	Window int
}

type keyEvent struct {
	name string
}

var tabNames = []string{"overview", "node", "history", "ping", "meta"}

type detailWindow struct {
	Label string
	Hours int
}

type detailSection struct {
	Title string
	Lines []string
	Chart *axisChart
}

type axisChart struct {
	Values []float64
	From   string
	To     string
	Unit   string
}

const detailCardHeight = 7

var detailWindows = []detailWindow{
	{Label: "realtime", Hours: 0},
	{Label: "4h", Hours: 4},
	{Label: "1d", Hours: 24},
	{Label: "7d", Hours: 24 * 7},
	{Label: "30d", Hours: 24 * 30},
}

func New(client *komari.Client, refreshInterval time.Duration) *App {
	return NewWithOptions(client, Options{RefreshInterval: refreshInterval})
}

func NewWithOptions(client *komari.Client, opts Options) *App {
	if opts.RefreshInterval <= 0 {
		opts.RefreshInterval = 5 * time.Second
	}
	if opts.Mode == "" {
		opts.Mode = ModeSheet
	}
	return &App{
		client:          client,
		refreshInterval: opts.RefreshInterval,
		style:           Style{ASCII: opts.ASCII, NoColor: opts.NoColor},
		mode:            opts.Mode,
		renderCh:        make(chan struct{}, 1),
		refreshCh:       make(chan struct{}, 2),
		resultCh:        make(chan fetchResult, 2),
		detailCh:        make(chan detailResult, 4),
		keyCh:           make(chan keyEvent, 16),
		loading:         true,
		nodeDetail:      map[detailKey]nodeDetail{},
	}
}

func (a *App) Run(ctx context.Context) error {
	state, err := enterRawMode()
	if err != nil {
		return fmt.Errorf("enter raw mode: %w", err)
	}
	defer state.restore()
	stopSignals := installSignalRestore(state)
	defer stopSignals()
	stopResize := installResizeHandler(a.requestRender)
	defer stopResize()

	go a.readKeys(ctx)
	a.requestRefresh()

	ticker := time.NewTicker(a.refreshInterval)
	defer ticker.Stop()
	marqueeTicker := time.NewTicker(300 * time.Millisecond)
	defer marqueeTicker.Stop()

	a.render()
	for !a.quit {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			a.requestRefresh()
		case <-marqueeTicker.C:
			width, _ := terminalSize()
			if width < 100 {
				a.marqueeFrame++
				a.render()
			}
		case <-a.refreshCh:
			a.fetch(ctx)
		case result := <-a.resultCh:
			a.loading = false
			a.fetching = false
			a.err = result.err
			if result.err == nil {
				a.snapshot = result.snapshot
				a.clampSelection()
				if a.detail {
					a.ensureSelectedDetail(ctx)
				}
			}
			a.render()
		case detail := <-a.detailCh:
			a.nodeDetail[detail.key] = detail.detail
			a.render()
		case key := <-a.keyCh:
			a.handleKey(ctx, key)
			a.render()
		case <-a.renderCh:
			a.render()
		}
	}
	return nil
}

func (a *App) fetch(ctx context.Context) {
	if a.fetching {
		return
	}
	a.loading = true
	a.fetching = true
	a.render()
	go func() {
		fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		snapshot, err := a.client.Snapshot(fetchCtx)
		a.resultCh <- fetchResult{snapshot: snapshot, err: err}
	}()
}

func (a *App) fetchDetail(ctx context.Context, uuid string, force bool) {
	if uuid == "" {
		return
	}
	key := detailKey{UUID: uuid, Window: a.window}
	current, ok := a.nodeDetail[key]
	if ok && current.Loading {
		return
	}
	if ok && !force && time.Since(current.FetchedAt) < 45*time.Second {
		return
	}
	current.UUID = uuid
	current.Window = a.window
	current.Loading = true
	current.Err = nil
	a.nodeDetail[key] = current
	a.render()

	go func() {
		detailCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		window := detailWindows[key.Window]
		result := nodeDetail{UUID: uuid, Window: key.Window, FetchedAt: time.Now()}

		if recent, err := a.client.RecentStatus(detailCtx, uuid); err == nil {
			result.Recent = recent
		} else {
			result.Err = err
		}
		if window.Hours > 0 {
			if load, err := a.client.LoadRecords(detailCtx, uuid, window.Hours, "all", maxCountForWindow(window.Hours)); err == nil {
				result.Load = load
			} else if result.Err == nil {
				result.Err = err
			}
			if ping, err := a.client.PingRecords(detailCtx, uuid, window.Hours, -1, maxCountForWindow(window.Hours)); err == nil {
				result.Ping = ping
			} else if result.Err == nil {
				result.Err = err
			}
		}
		a.detailCh <- detailResult{key: key, detail: result}
	}()
}

func (a *App) ensureSelectedDetail(ctx context.Context) {
	if len(a.snapshot.Nodes) == 0 {
		return
	}
	a.fetchDetail(ctx, a.snapshot.Nodes[a.selected].UUID, false)
}

func (a *App) requestRefresh() {
	select {
	case a.refreshCh <- struct{}{}:
	default:
	}
}

func (a *App) requestRender() {
	select {
	case a.renderCh <- struct{}{}:
	default:
	}
}

func (a *App) readKeys(ctx context.Context) {
	buf := make([]byte, 8)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := os.Stdin.Read(buf)
		if err != nil {
			if err == io.EOF {
				continue
			}
			return
		}
		if n == 0 {
			continue
		}
		for _, key := range parseKeys(buf[:n]) {
			select {
			case a.keyCh <- key:
			default:
			}
		}
	}
}

func parseKeys(data []byte) []keyEvent {
	out := make([]keyEvent, 0, len(data))
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case 3:
			out = append(out, keyEvent{name: "force-quit"})
		case 'q', 'Q':
			out = append(out, keyEvent{name: "quit"})
		case 'r', 'R':
			out = append(out, keyEvent{name: "refresh"})
		case '\r', '\n':
			out = append(out, keyEvent{name: "open"})
		case 'd', 'D':
			out = append(out, keyEvent{name: "detail-refresh"})
		case 'a', 'A':
			out = append(out, keyEvent{name: "ascii"})
		case 'o', 'O':
			out = append(out, keyEvent{name: "open"})
		case 'b', 'B', 0x7f:
			out = append(out, keyEvent{name: "back"})
		case 'm', 'M':
			out = append(out, keyEvent{name: "mode"})
		case 'j':
			out = append(out, keyEvent{name: "down"})
		case 'k':
			out = append(out, keyEvent{name: "up"})
		case 'h':
			out = append(out, keyEvent{name: "tab-left"})
		case 'l':
			out = append(out, keyEvent{name: "tab-right"})
		case '1', '2', '3', '4', '5':
			out = append(out, keyEvent{name: "tab-" + string(data[i])})
		case '\t':
			out = append(out, keyEvent{name: "tab-right"})
		case 'g':
			out = append(out, keyEvent{name: "top"})
		case 'G':
			out = append(out, keyEvent{name: "bottom"})
		case '[':
			out = append(out, keyEvent{name: "window-left"})
		case ']':
			out = append(out, keyEvent{name: "window-right"})
		case 0x1b:
			if i+2 < len(data) && data[i+1] == '[' {
				switch data[i+2] {
				case 'A':
					out = append(out, keyEvent{name: "up"})
					i += 2
				case 'B':
					out = append(out, keyEvent{name: "down"})
					i += 2
				case 'C':
					out = append(out, keyEvent{name: "tab-right"})
					i += 2
				case 'D':
					out = append(out, keyEvent{name: "tab-left"})
					i += 2
				case '5':
					out = append(out, keyEvent{name: "pageup"})
					if i+3 < len(data) && data[i+3] == '~' {
						i += 3
					} else {
						i += 2
					}
				case '6':
					out = append(out, keyEvent{name: "pagedown"})
					if i+3 < len(data) && data[i+3] == '~' {
						i += 3
					} else {
						i += 2
					}
				}
			} else {
				out = append(out, keyEvent{name: "back"})
			}
		}
	}
	return out
}

func (a *App) handleKey(ctx context.Context, key keyEvent) {
	previous := a.selected
	previousTab := a.tab
	previousWindow := a.window
	switch key.name {
	case "force-quit":
		a.quit = true
	case "quit":
		if a.detail {
			a.detail = false
			a.scroll = 0
		} else {
			a.quit = true
		}
	case "back":
		if a.detail {
			a.detail = false
			a.scroll = 0
		}
	case "open":
		if len(a.snapshot.Nodes) > 0 {
			a.detail = true
			a.scroll = 0
			a.ensureSelectedDetail(ctx)
		}
	case "refresh":
		a.requestRefresh()
		if a.detail && len(a.snapshot.Nodes) > 0 {
			a.fetchDetail(ctx, a.snapshot.Nodes[a.selected].UUID, true)
		}
	case "detail-refresh":
		if len(a.snapshot.Nodes) > 0 {
			a.detail = true
			a.scroll = 0
			a.fetchDetail(ctx, a.snapshot.Nodes[a.selected].UUID, true)
		}
	case "ascii":
		a.style.ASCII = !a.style.ASCII
	case "mode":
		if !a.detail {
			if a.mode == ModeLine {
				a.mode = ModeSheet
			} else {
				a.mode = ModeLine
			}
		}
	case "up":
		if a.detail {
			a.scroll -= detailScrollStep()
		} else {
			a.selected--
		}
	case "down":
		if a.detail {
			a.scroll += detailScrollStep()
		} else {
			a.selected++
		}
	case "pageup":
		if a.detail {
			a.scroll -= detailScrollStep() * 3
		} else {
			a.selected -= 10
		}
	case "pagedown":
		if a.detail {
			a.scroll += detailScrollStep() * 3
		} else {
			a.selected += 10
		}
	case "top":
		if a.detail {
			a.scroll = 0
		} else {
			a.selected = 0
		}
	case "bottom":
		if a.detail {
			a.scroll = 1 << 30
		} else {
			a.selected = len(a.snapshot.Nodes) - 1
		}
	case "tab-left":
		if a.detail {
			a.tab = (a.tab + len(tabNames) - 1) % len(tabNames)
			a.scroll = 0
		}
	case "tab-right":
		if a.detail {
			a.tab = (a.tab + 1) % len(tabNames)
			a.scroll = 0
		}
	case "tab-1", "tab-2", "tab-3", "tab-4", "tab-5":
		if a.detail {
			a.tab = int(key.name[len(key.name)-1] - '1')
			a.scroll = 0
		}
	case "window-left":
		if a.detail {
			a.window = (a.window + len(detailWindows) - 1) % len(detailWindows)
			a.scroll = 0
		}
	case "window-right":
		if a.detail {
			a.window = (a.window + 1) % len(detailWindows)
			a.scroll = 0
		}
	}
	a.clampSelection()
	if a.scroll < 0 {
		a.scroll = 0
	}
	if a.detail && (previous != a.selected || previousTab != a.tab || previousWindow != a.window || a.tabNeedsDetail()) {
		a.ensureSelectedDetail(ctx)
	}
}

func (a *App) tabNeedsDetail() bool {
	return a.tab == 2 || a.tab == 3
}

func detailScrollStep() int {
	return detailCardHeight
}

func (a *App) clampSelection() {
	count := len(a.snapshot.Nodes)
	if count == 0 {
		a.selected = 0
		a.scroll = 0
		return
	}
	if a.selected < 0 {
		a.selected = 0
	}
	if a.selected >= count {
		a.selected = count - 1
	}
}

func (a *App) render() {
	width, height := terminalSize()
	drawWidth := width
	if drawWidth > 1 {
		drawWidth--
	}
	setMarquee(a.marqueeFrame, drawWidth < 100)
	defer setMarquee(0, false)

	var b strings.Builder
	b.WriteString("\x1b[?25l")
	if drawWidth < 30 || height < 8 {
		lines := []string{
			"ktui",
			"Window too small.",
			"Resize to at least 30x8.",
			"q quits.",
		}
		a.writeFrame(&b, width, height, lines)
		fmt.Print(b.String())
		return
	}

	lines := make([]string, 0, height)
	lines = append(lines, a.headerLines(drawWidth)...)

	bodyHeight := height - len(lines) - 1
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	if a.detail {
		lines = append(lines, a.renderDetailBody(drawWidth, bodyHeight)...)
	} else if a.mode == ModeLine {
		lines = append(lines, a.renderLineBody(drawWidth, bodyHeight)...)
	} else {
		lines = append(lines, a.renderSheetBody(drawWidth, bodyHeight)...)
	}

	lines = append(lines, a.footerLine(drawWidth))
	a.writeFrame(&b, width, height, lines)
	fmt.Print(b.String())
}

func (a *App) renderSheetBody(width int, bodyHeight int) []string {
	if bodyHeight <= 0 {
		return nil
	}
	if len(a.snapshot.Nodes) == 0 {
		if a.loading {
			return fillBody([]string{"Loading nodes..."}, width, bodyHeight)
		}
		return fillBody([]string{"No nodes returned by Komari."}, width, bodyHeight)
	}

	cardHeight := 9
	gap := 2
	minCardWidth := 40
	columns := max(1, (width+gap)/(minCardWidth+gap))
	for columns > 1 {
		cardWidth := (width - gap*(columns-1)) / columns
		if cardWidth >= minCardWidth {
			break
		}
		columns--
	}
	cardWidth := width
	if columns > 1 {
		cardWidth = (width - gap*(columns-1)) / columns
	}

	rowsVisible := max(1, bodyHeight/cardHeight)
	selectedRow := a.selected / columns
	if selectedRow < a.scroll {
		a.scroll = selectedRow
	}
	if selectedRow >= a.scroll+rowsVisible {
		a.scroll = selectedRow - rowsVisible + 1
	}
	maxScroll := max(0, (len(a.snapshot.Nodes)+columns-1)/columns-rowsVisible)
	if a.scroll > maxScroll {
		a.scroll = maxScroll
	}
	if a.scroll < 0 {
		a.scroll = 0
	}

	lines := make([]string, 0, bodyHeight)
	startRow := a.scroll
	endRow := min((len(a.snapshot.Nodes)+columns-1)/columns, startRow+rowsVisible)
	for row := startRow; row < endRow; row++ {
		rowCards := make([][]string, 0, columns)
		for col := 0; col < columns; col++ {
			index := row*columns + col
			if index >= len(a.snapshot.Nodes) {
				rowCards = append(rowCards, emptyCard(cardWidth, cardHeight))
				continue
			}
			rowCards = append(rowCards, a.nodeCard(index, a.snapshot.Nodes[index], cardWidth, cardHeight))
		}
		for lineIndex := 0; lineIndex < cardHeight; lineIndex++ {
			var line strings.Builder
			for col, card := range rowCards {
				if col > 0 {
					line.WriteString(strings.Repeat(" ", gap))
				}
				line.WriteString(card[lineIndex])
			}
			lines = append(lines, fitLine(line.String(), width))
		}
	}
	return fillBody(lines, width, bodyHeight)
}

func (a *App) nodeCard(index int, node komari.Node, width int, height int) []string {
	if height < 5 {
		height = 5
	}
	st := a.snapshot.Status[node.UUID]
	contentHeight := height - 2
	body := a.overviewCard(index, node, st, max(0, width-2), contentHeight)

	lines := make([]string, 0, height)
	lines = append(lines, a.cardTop(width, index == a.selected))
	for i := 0; i < contentHeight; i++ {
		content := ""
		if i < len(body) {
			content = body[i]
		}
		line := a.style.boxLine(content, width)
		if index == a.selected && i == 0 && !a.style.NoColor {
			line = a.style.bold(line)
		} else if !st.Online {
			line = a.style.dim(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, a.cardBottom(width, index == a.selected))
	return lines
}

func (a *App) overviewCard(index int, node komari.Node, st komari.Status, width int, maxLines int) []string {
	ramPct := percent(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal))
	diskPct := percent(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal))
	nameWidth := max(8, width-16)
	status := a.statusPill(st.Online)
	marker := " "
	if index == a.selected {
		marker = ">"
	}
	title := fmt.Sprintf(" %s %-*s %s", marker, nameWidth, cleanLine(a.nodeLabel(node), nameWidth), status)
	lines := []string{
		title,
		a.metricLine("CPU", st.CPU, "load "+fmt.Sprintf("%.2f %.2f %.2f", st.Load, st.Load5, st.Load15), width),
		a.metricLine("RAM", ramPct, usageCompact(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal)), width),
		a.metricLine("DSK", diskPct, usageCompact(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal)), width),
		fmt.Sprintf(" NET  %s %s  %s %s", a.style.up(), speedIEC(st.NetOut), a.style.down(), speedIEC(st.NetIn)),
		fmt.Sprintf(" FLOW %s %s  %s %s", a.style.up(), bytesIEC(st.NetTotalUp), a.style.down(), bytesIEC(st.NetTotalDown)),
		fmt.Sprintf(" UP   %-10s EXP %s", durationCompact(st.Uptime), expiryText(node, time.Now())),
	}
	if node.Tags != "" || node.Group != "" {
		lines = append(lines, fmt.Sprintf(" OS   %s  TAG %s %s", valueOr(a.text(node.OS), "-"), valueOr(a.text(node.Group), "-"), valueOr(a.text(node.Tags), "-")))
	}
	return limitRawLines(lines, maxLines)
}

func (a *App) metricLine(label string, pct float64, value string, width int) string {
	barWidth := min(12, max(6, width-30))
	return fmt.Sprintf(" %-4s %5.1f%% %s  %s", label, pct, a.usageBar(pct, barWidth), value)
}

func (a *App) usageBar(pct float64, width int) string {
	bar := a.style.miniBar(pct, width)
	switch {
	case pct >= 90:
		return a.style.red(bar)
	case pct >= 75:
		return a.style.yellow(bar)
	default:
		return a.style.cyan(bar)
	}
}

func (a *App) statusPill(online bool) string {
	if a.style.ASCII {
		if online {
			return a.style.green("ONLINE")
		}
		return a.style.red("OFFLINE")
	}
	if online {
		return a.style.green("● online")
	}
	return a.style.red("● offline")
}

func (a *App) cardTop(width int, selected bool) string {
	line := a.style.boxTop(width)
	if selected {
		return a.style.cyan(line)
	}
	return a.style.dim(line)
}

func (a *App) cardBottom(width int, selected bool) string {
	line := a.style.boxBottom(width)
	if selected {
		return a.style.cyan(line)
	}
	return a.style.dim(line)
}

func (a *App) renderDetailBody(width int, bodyHeight int) []string {
	if bodyHeight <= 0 {
		return nil
	}
	if len(a.snapshot.Nodes) == 0 {
		if a.loading {
			return fillBody([]string{"Loading nodes..."}, width, bodyHeight)
		}
		return fillBody([]string{"No nodes returned by Komari."}, width, bodyHeight)
	}

	a.clampSelection()
	node := a.snapshot.Nodes[a.selected]
	st := a.snapshot.Status[node.UUID]
	chrome := a.detailChromeLines(node, st, width)
	contentHeight := bodyHeight - len(chrome)
	if contentHeight < 1 {
		contentHeight = 1
		if len(chrome) > bodyHeight-1 {
			chrome = chrome[:max(0, bodyHeight-1)]
		}
	}
	if contentHeight >= detailCardHeight {
		contentHeight = contentHeight / detailCardHeight * detailCardHeight
	}

	content := a.detailContentLines(node, st, width)
	if contentHeight >= detailCardHeight {
		a.scroll = a.scroll / detailCardHeight * detailCardHeight
	}
	maxScroll := max(0, len(content)-contentHeight)
	if contentHeight >= detailCardHeight {
		maxScroll = maxScroll / detailCardHeight * detailCardHeight
	}
	if a.scroll > maxScroll {
		a.scroll = maxScroll
	}
	if a.scroll < 0 {
		a.scroll = 0
	}
	end := min(len(content), a.scroll+contentHeight)
	lines := make([]string, 0, bodyHeight)
	lines = append(lines, chrome...)
	lines = append(lines, content[a.scroll:end]...)
	return fillBody(lines, width, bodyHeight)
}

func (a *App) detailChromeLines(node komari.Node, st komari.Status, width int) []string {
	lines := make([]string, 0, 6)
	title := fmt.Sprintf(" %s  %s  %s  region %s  seen %s",
		a.text(node.Name),
		a.statusPill(st.Online),
		durationCompact(st.Uptime),
		valueOr(a.text(node.Region), "-"),
		shortTimeFromNull(st.Time),
	)
	lines = append(lines, a.style.bold(fitLine(title, width)))
	lines = append(lines, a.detailMetricStrip(node, st, width))
	lines = append(lines, a.detailTabLine(width))
	lines = append(lines, a.detailWindowLine(width))
	return lines
}

func (a *App) detailMetricStrip(node komari.Node, st komari.Status, width int) string {
	ramPct := percent(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal))
	diskPct := percent(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal))
	barWidth := 6
	if width >= 100 {
		barWidth = 12
	} else if width >= 80 {
		barWidth = 8
	}
	parts := []string{
		fmt.Sprintf(" CPU %5.1f%% %s", st.CPU, a.usageBar(st.CPU, barWidth)),
		fmt.Sprintf(" RAM %5.1f%% %s", ramPct, a.usageBar(ramPct, barWidth)),
		fmt.Sprintf(" DSK %5.1f%% %s", diskPct, a.usageBar(diskPct, barWidth)),
	}
	if width >= 104 {
		parts = append(parts, fmt.Sprintf(" NET %s %s %s %s", a.style.up(), speedIEC(st.NetOut), a.style.down(), speedIEC(st.NetIn)))
	}
	return fitLine(strings.Join(parts, "  "), width)
}

func (a *App) detailTabLine(width int) string {
	parts := make([]string, 0, len(tabNames))
	for i, name := range tabNames {
		label := fmt.Sprintf(" %d %s ", i+1, name)
		if i == a.tab {
			if a.style.NoColor {
				label = "[" + strings.TrimSpace(label) + "]"
			} else {
				label = a.style.inverse(label)
			}
		} else {
			label = a.style.dim(label)
		}
		parts = append(parts, label)
	}
	return fitLine(strings.Join(parts, " "), width)
}

func (a *App) detailWindowLine(width int) string {
	parts := make([]string, 0, len(detailWindows)+1)
	parts = append(parts, a.style.dim(" window "))
	for i, window := range detailWindows {
		label := " " + window.Label + " "
		if i == a.window {
			if a.style.NoColor {
				label = "[" + window.Label + "]"
			} else {
				label = a.style.cyan(label)
			}
		} else {
			label = a.style.dim(label)
		}
		parts = append(parts, label)
	}
	return fitLine(strings.Join(parts, " "), width)
}

func (a *App) detailContentLines(node komari.Node, st komari.Status, width int) []string {
	sections := a.detailSections(node, st)
	if len(sections) == 0 {
		return []string{fitLine("", width)}
	}
	if width >= 96 {
		return a.detailSectionGrid(sections, width, 2)
	}
	return a.detailSectionGrid(sections, width, 1)
}

func (a *App) detailSectionGrid(sections []detailSection, width int, columns int) []string {
	gap := 2
	cardHeight := detailCardHeight
	if columns < 1 {
		columns = 1
	}
	for columns > 1 {
		cardWidth := (width - gap*(columns-1)) / columns
		if cardWidth >= 42 {
			break
		}
		columns--
	}
	cardWidth := width
	if columns > 1 {
		cardWidth = (width - gap*(columns-1)) / columns
	}

	lines := make([]string, 0, len(sections)*(cardHeight+1))
	for row := 0; row < len(sections); row += columns {
		rowCards := make([][]string, 0, columns)
		for col := 0; col < columns; col++ {
			index := row + col
			if index >= len(sections) {
				rowCards = append(rowCards, emptyCard(cardWidth, cardHeight))
				continue
			}
			rowCards = append(rowCards, a.detailSectionCard(sections[index], cardWidth, cardHeight))
		}
		for lineIndex := 0; lineIndex < cardHeight; lineIndex++ {
			var line strings.Builder
			for col, card := range rowCards {
				if col > 0 {
					line.WriteString(strings.Repeat(" ", gap))
				}
				line.WriteString(card[lineIndex])
			}
			lines = append(lines, fitLine(line.String(), width))
		}
	}
	return lines
}

func (a *App) detailSectionCard(section detailSection, width int, height int) []string {
	if height < 4 {
		height = 4
	}
	lines := make([]string, 0, height)
	lines = append(lines, a.cardTop(width, false))
	title := " " + a.style.bold(section.Title)
	lines = append(lines, a.style.boxLine(title, width))
	contentHeight := height - 3
	contentLines := section.Lines
	if section.Chart != nil {
		contentLines = a.axisChartLines(*section.Chart, width-2, contentHeight)
	}
	for i := 0; i < contentHeight; i++ {
		content := ""
		if i < len(contentLines) {
			content = contentLines[i]
		}
		lines = append(lines, a.style.boxLine(content, width))
	}
	lines = append(lines, a.cardBottom(width, false))
	return lines
}

func (a *App) detailSections(node komari.Node, st komari.Status) []detailSection {
	switch a.tab {
	case 1:
		return a.nodeInfoSections(node, st)
	case 2:
		return a.historySections(node, st)
	case 3:
		return a.pingSections(node, st)
	case 4:
		return a.metaSections(node, st)
	default:
		return a.overviewSections(node, st)
	}
}

func (a *App) overviewSections(node komari.Node, st komari.Status) []detailSection {
	ramTotal := firstNonZero(st.RAMTotal, node.MemTotal)
	diskTotal := firstNonZero(st.DiskTotal, node.DiskTotal)
	swapTotal := firstNonZero(st.SwapTotal, node.SwapTotal)
	ramPct := percent(st.RAM, ramTotal)
	diskPct := percent(st.Disk, diskTotal)
	swapPct := percent(st.Swap, swapTotal)
	sections := []detailSection{
		{
			Title: "Health",
			Lines: []string{
				fmt.Sprintf(" Status  %s", a.style.coloredStatus(st.Online)),
				fmt.Sprintf(" Seen    %s", timeText(st.Time)),
				fmt.Sprintf(" Uptime  %s", durationCompact(st.Uptime)),
				fmt.Sprintf(" Load    %.2f %.2f %.2f", st.Load, st.Load5, st.Load15),
				fmt.Sprintf(" Proc    %d", st.Process),
			},
		},
		{
			Title: "Resources",
			Lines: []string{
				a.detailUsageLine(" CPU", st.CPU, fmt.Sprintf("%.1f%%", st.CPU)),
				a.detailUsageLine(" RAM", ramPct, usageCompact(st.RAM, ramTotal)),
				a.detailUsageLine(" Disk", diskPct, usageCompact(st.Disk, diskTotal)),
				a.detailUsageLine(" Swap", swapPct, usageCompact(st.Swap, swapTotal)),
			},
		},
		{
			Title: "Network",
			Lines: []string{
				fmt.Sprintf(" Now     %s %s", a.style.up(), speedIEC(st.NetOut)),
				fmt.Sprintf("         %s %s", a.style.down(), speedIEC(st.NetIn)),
				fmt.Sprintf(" Total   %s %s", a.style.up(), bytesIEC(st.NetTotalUp)),
				fmt.Sprintf("         %s %s", a.style.down(), bytesIEC(st.NetTotalDown)),
				fmt.Sprintf(" Conn    TCP %d  UDP %d", st.Connections, st.ConnectionsUDP),
			},
		},
		{
			Title: "Placement",
			Lines: []string{
				fmt.Sprintf(" Region  %s", valueOr(a.text(node.Region), "-")),
				fmt.Sprintf(" Group   %s", valueOr(a.text(node.Group), "-")),
				fmt.Sprintf(" Tags    %s", valueOr(a.text(node.Tags), "-")),
				fmt.Sprintf(" OS      %s", valueOr(a.text(node.OS), "-")),
			},
		},
	}
	if node.TrafficLimit > 0 {
		pct := trafficPercent(st.NetTotalUp, st.NetTotalDown, node.TrafficLimit, node.TrafficLimitType)
		sections = append(sections, detailSection{
			Title: "Traffic Limit",
			Lines: []string{
				a.detailUsageLine(" Used", pct, fmt.Sprintf("%.1f%%", pct)),
				fmt.Sprintf(" Limit   %s", bytesIEC(node.TrafficLimit)),
				fmt.Sprintf(" Type    %s", valueOr(node.TrafficLimitType, "-")),
			},
		})
	}
	return sections
}

func (a *App) nodeInfoSections(node komari.Node, st komari.Status) []detailSection {
	sections := []detailSection{
		{
			Title: "System",
			Lines: []string{
				fmt.Sprintf(" OS      %s", valueOr(a.text(node.OS), "-")),
				fmt.Sprintf(" Arch    %s", valueOr(a.text(node.Arch), "-")),
				fmt.Sprintf(" Kernel  %s", valueOr(a.text(node.KernelVersion), "-")),
				fmt.Sprintf(" Agent   %s", valueOr(a.text(node.Version), "-")),
				fmt.Sprintf(" Virt    %s", valueOr(a.text(node.Virtualization), "-")),
			},
		},
		{
			Title: "Hardware",
			Lines: []string{
				fmt.Sprintf(" CPU     %s", valueOr(a.text(node.CPUName), "-")),
				fmt.Sprintf(" Cores   %s", coreText(node)),
				fmt.Sprintf(" GPU     %s", valueOr(a.text(node.GPUName), "-")),
				fmt.Sprintf(" Memory  %s", bytesIEC(node.MemTotal)),
				fmt.Sprintf(" Disk    %s", bytesIEC(node.DiskTotal)),
			},
		},
		{
			Title: "Live Load",
			Lines: []string{
				a.detailUsageLine(" CPU", st.CPU, fmt.Sprintf("%.1f%%", st.CPU)),
				a.detailUsageLine(" RAM", percent(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal)), usageCompact(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal))),
				a.detailUsageLine(" Disk", percent(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal)), usageCompact(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal))),
				fmt.Sprintf(" Temp    %.1f", st.Temp),
			},
		},
	}
	if node.Price != 0 || node.ExpiredAt.Valid || node.PublicRemark != "" || node.Remark != "" {
		price := "-"
		if node.Price > 0 {
			price = fmt.Sprintf("%s%.2f / %dd", valueOr(node.Currency, ""), node.Price, node.BillingCycle)
		} else if node.Price < 0 {
			price = "free"
		}
		sections = append(sections, detailSection{
			Title: "Billing",
			Lines: []string{
				fmt.Sprintf(" Price   %s", price),
				fmt.Sprintf(" Renew   %t", node.AutoRenewal),
				fmt.Sprintf(" Expire  %s", nullableDate(node.ExpiredAt)),
				fmt.Sprintf(" Public  %s", valueOr(a.text(node.PublicRemark), "-")),
				fmt.Sprintf(" Remark  %s", valueOr(a.text(node.Remark), "-")),
			},
		})
	}
	return sections
}

func (a *App) historySections(node komari.Node, st komari.Status) []detailSection {
	window := detailWindows[a.window]
	detail := a.currentDetail(node.UUID)
	infoSections := []detailSection{
		{
			Title: "Window",
			Lines: []string{
				fmt.Sprintf(" Range   %s", window.Label),
				fmt.Sprintf(" Records load %d recent %d", len(detail.Load.Records), len(detail.Recent.Records)),
				fmt.Sprintf(" From    %s", nullableDateTime(detail.Load.From)),
				fmt.Sprintf(" To      %s", nullableDateTime(detail.Load.To)),
			},
		},
	}
	if detail.Loading {
		infoSections = append(infoSections, detailSection{
			Title: "Status",
			Lines: []string{" Loading records...", " Press d to force reload."},
		})
	}
	if detail.Err != nil {
		infoSections = append(infoSections, detailSection{
			Title: "Error",
			Lines: []string{" " + detail.Err.Error(), " Press d to retry."},
		})
	}
	if window.Hours == 0 {
		records := realtimeStatusRecords(detail.Recent.Records, st)
		sections := a.historyMetricSections(node, st, records, "Realtime")
		return append(sections, infoSections...)
	}
	if len(detail.Load.Records) == 0 {
		return append(infoSections, detailSection{
			Title: "History",
			Lines: []string{" No load history yet.", " Press d to load details."},
		})
	}

	sections := a.historyMetricSections(node, st, detail.Load.Records, "History")
	return append(sections, infoSections...)
}

func (a *App) pingSections(node komari.Node, st komari.Status) []detailSection {
	window := detailWindows[a.window]
	detail := a.currentDetail(node.UUID)
	sections := []detailSection{
		{
			Title: "Window",
			Lines: []string{
				fmt.Sprintf(" Range   %s", window.Label),
				fmt.Sprintf(" Tasks   %d", len(detail.Ping.Tasks)),
				fmt.Sprintf(" Records %d", len(detail.Ping.Records)),
				fmt.Sprintf(" From    %s", nullableDateTime(detail.Ping.From)),
				fmt.Sprintf(" To      %s", nullableDateTime(detail.Ping.To)),
			},
		},
	}
	realtime := a.realtimePingLines(st, 5)
	if len(realtime) == 0 {
		realtime = []string{" No realtime ping data."}
	}
	sections = append(sections, detailSection{Title: "Realtime Ping", Lines: realtime})
	if window.Hours == 0 {
		records := realtimeStatusRecords(detail.Recent.Records, st)
		if values := realtimePingStatusValues(records, func(ping komari.Ping) float64 { return ping.Latest }); len(values) > 0 {
			sections = append(sections, a.sparkSection("Latency Spark", values))
		}
		if values := realtimePingStatusValues(records, func(ping komari.Ping) float64 { return ping.Loss }); len(values) > 0 {
			sections = append(sections, a.sparkSection("Loss Spark", values))
		}
		return sections
	}
	if detail.Loading {
		sections = append(sections, detailSection{
			Title: "Status",
			Lines: []string{" Loading ping records...", " Press d to force reload."},
		})
	}
	if detail.Err != nil {
		sections = append(sections, detailSection{
			Title: "Error",
			Lines: []string{" " + detail.Err.Error(), " Press d to retry."},
		})
	}
	if len(detail.Ping.Tasks) == 0 && len(detail.Ping.Records) == 0 {
		return append(sections, detailSection{
			Title: "History Ping",
			Lines: []string{" No ping history yet.", " Press d to load details."},
		})
	}
	if len(detail.Ping.Tasks) > 0 {
		taskLines := make([]string, 0, len(detail.Ping.Tasks))
		for _, task := range detail.Ping.Tasks {
			taskLines = append(taskLines, fmt.Sprintf(" %-10s avg %4.0f min %4.0f max %4.0f",
				cleanLine(a.text(task.Name), 10), task.Avg, task.Min, task.Max))
			if len(taskLines) >= 5 {
				break
			}
		}
		sections = append(sections, detailSection{Title: "History Ping", Lines: taskLines})
		lossLines := make([]string, 0, len(detail.Ping.Tasks))
		for _, task := range detail.Ping.Tasks {
			lossLines = append(lossLines, fmt.Sprintf(" %-10s loss %.1f%% total %d", cleanLine(a.text(task.Name), 10), task.Loss, task.Total))
			if len(lossLines) >= 5 {
				break
			}
		}
		sections = append(sections, detailSection{Title: "Loss", Lines: lossLines})
	} else {
		sections = append(sections, detailSection{
			Title: "History Ping",
			Lines: []string{
				fmt.Sprintf(" Records %d", len(detail.Ping.Records)),
				" Tasks   unavailable",
			},
		})
	}
	if values := pingRecordValues(detail.Ping.Records); len(values) > 0 {
		sections = append(sections, a.sparkSection("Latency Spark", values))
	}
	return sections
}

func (a *App) metaSections(node komari.Node, st komari.Status) []detailSection {
	window := detailWindows[a.window]
	detail := a.currentDetail(node.UUID)
	sections := []detailSection{
		{
			Title: "Identity",
			Lines: []string{
				fmt.Sprintf(" UUID    %s", node.UUID),
				fmt.Sprintf(" Name    %s", a.text(node.Name)),
				fmt.Sprintf(" IPv4    %s", a.nodeMetaValue(node.IPv4)),
				fmt.Sprintf(" IPv6    %s", a.nodeMetaValue(node.IPv6)),
				fmt.Sprintf(" Agent   %s", valueOr(a.text(node.Version), "-")),
			},
		},
		{
			Title: "Komari",
			Lines: []string{
				fmt.Sprintf(" Version %s", valueOr(a.snapshot.Version.Version, "-")),
				fmt.Sprintf(" Hash    %s", valueOr(a.snapshot.Version.Hash, "-")),
				fmt.Sprintf(" RPC     %s", valueOr(a.snapshot.RPCVersion, "-")),
				fmt.Sprintf(" Auth    %s", a.authText()),
				fmt.Sprintf(" Methods %d", len(a.snapshot.Methods)),
			},
		},
		{
			Title: "Site",
			Lines: []string{
				fmt.Sprintf(" Private %t", a.snapshot.Public.PrivateSite),
				fmt.Sprintf(" Records %t", a.snapshot.Public.RecordEnabled),
				fmt.Sprintf(" CORS    %t", a.snapshot.Public.CORSOriginCheckEnabled),
				fmt.Sprintf(" OAuth   %t %s", a.snapshot.Public.OAuthEnable, valueOr(a.snapshot.Public.OAuthProvider, "-")),
			},
		},
		{
			Title: "Detail Cache",
			Lines: []string{
				fmt.Sprintf(" Window  %s", window.Label),
				fmt.Sprintf(" Load    %d", len(detail.Load.Records)),
				fmt.Sprintf(" Ping    %d", len(detail.Ping.Records)),
				fmt.Sprintf(" Online  %t", st.Online),
				fmt.Sprintf(" Seen    %s", timeText(st.Time)),
			},
		},
	}
	if len(a.snapshot.Methods) > 0 {
		methods := make([]string, 0, min(len(a.snapshot.Methods), 5))
		for _, method := range a.snapshot.Methods {
			methods = append(methods, " "+method)
			if len(methods) >= 5 {
				break
			}
		}
		sections = append(sections, detailSection{Title: "RPC Methods", Lines: methods})
	}
	return sections
}

func (a *App) detailUsageLine(label string, pct float64, value string) string {
	return fmt.Sprintf(" %-5s %5.1f%% %s  %s", strings.TrimSpace(label), pct, a.usageBar(pct, 10), value)
}

func (a *App) sparkSection(title string, values []float64) detailSection {
	return detailSection{
		Title: title,
		Lines: []string{
			" " + sparklineLimit(values, a.style.ASCII, 36),
			fmt.Sprintf(" Samples %d", len(values)),
		},
	}
}

func (a *App) historyChartSection(title string, records []komari.Status, unit string, values []float64) detailSection {
	return detailSection{
		Title: title,
		Chart: &axisChart{
			Values: values,
			From:   chartTimeLabel(firstRecordTime(records)),
			To:     chartTimeLabel(lastRecordTime(records)),
			Unit:   unit,
		},
	}
}

func (a *App) axisChartLines(chart axisChart, width int, height int) []string {
	if height <= 0 {
		return nil
	}
	if len(chart.Values) == 0 {
		return limitRawLines([]string{" no samples"}, height)
	}
	if height < 4 || width < 18 {
		return limitRawLines([]string{
			" " + sparklineLimit(chart.Values, a.style.ASCII, max(1, width-1)),
			fmt.Sprintf(" %s -> %s", chart.From, chart.To),
		}, height)
	}

	minVal, maxVal := minMaxFloat(chart.Values)
	midVal := (minVal + maxVal) / 2
	labelWidth := chartLabelWidth(minVal, midVal, maxVal, chart.Unit)
	plotWidth := max(1, width-labelWidth-2)
	points := downsampleValues(chart.Values, plotWidth)
	rows := []struct {
		value float64
		label string
	}{
		{maxVal, chartValueLabel(maxVal, chart.Unit, labelWidth)},
		{midVal, chartValueLabel(midVal, chart.Unit, labelWidth)},
		{minVal, chartValueLabel(minVal, chart.Unit, labelWidth)},
	}

	out := make([]string, 0, height)
	for rowIndex, row := range rows {
		var b strings.Builder
		b.WriteString(row.label)
		if a.style.ASCII {
			b.WriteString("|")
		} else {
			b.WriteString("│")
		}
		for _, value := range points {
			if chartPointRow(value, minVal, maxVal) == rowIndex {
				if a.style.ASCII {
					b.WriteByte('*')
				} else {
					b.WriteRune('•')
				}
			} else if rowIndex == 2 {
				if a.style.ASCII {
					b.WriteByte('-')
				} else {
					b.WriteRune('─')
				}
			} else {
				b.WriteByte(' ')
			}
		}
		out = append(out, b.String())
	}

	axis := chartXAxisLine(chart.From, chart.To, width, labelWidth, a.style.ASCII)
	out = append(out, axis)
	return limitRawLines(out, height)
}

func (a *App) historyMetricSections(node komari.Node, st komari.Status, records []komari.Status, label string) []detailSection {
	sum := summarizeStatusWithTotals(records, node.MemTotal, node.DiskTotal)
	return []detailSection{
		a.historyChartSection("CPU Chart", records, "%", statusValues(records, func(st komari.Status) float64 { return st.CPU })),
		a.historyChartSection("RAM Chart", records, "%", statusRAMPercentValues(records, node.MemTotal)),
		a.historyChartSection("Disk Chart", records, "%", statusDiskPercentValues(records, node.DiskTotal)),
		a.historyChartSection("Net Out Chart", records, "B/s", statusValues(records, func(st komari.Status) float64 { return float64(st.NetOut) })),
		a.historyChartSection("Net In Chart", records, "B/s", statusValues(records, func(st komari.Status) float64 { return float64(st.NetIn) })),
		a.historyChartSection("Connections Chart", records, "", statusValues(records, func(st komari.Status) float64 { return float64(st.Connections) })),
		a.historyChartSection("Process Chart", records, "", statusValues(records, func(st komari.Status) float64 { return float64(st.Process) })),
		{
			Title: label + " Load",
			Lines: []string{
				fmt.Sprintf(" CPU  avg %.1f  max %.1f", sum.CPUAvg, sum.CPUMax),
				fmt.Sprintf(" RAM  avg %.1f  max %.1f", sum.RAMAvg, sum.RAMMax),
				fmt.Sprintf(" Disk avg %.1f  max %.1f", sum.DiskAvg, sum.DiskMax),
				fmt.Sprintf(" Load avg %.2f max %.2f", sum.LoadAvg, sum.LoadMax),
			},
		},
		{
			Title: "Network",
			Lines: []string{
				fmt.Sprintf(" Out avg %s", speedIEC(int64(sum.NetOutAvg))),
				fmt.Sprintf(" Out max %s", speedIEC(sum.NetOutMax)),
				fmt.Sprintf(" In  avg %s", speedIEC(int64(sum.NetInAvg))),
				fmt.Sprintf(" In  max %s", speedIEC(sum.NetInMax)),
			},
		},
		{
			Title: "Runtime",
			Lines: []string{
				fmt.Sprintf(" Conn avg %.0f max %d", sum.ConnectionsAvg, sum.ConnectionsMax),
				fmt.Sprintf(" Conn now TCP %d UDP %d", st.Connections, st.ConnectionsUDP),
				fmt.Sprintf(" Proc avg %.0f max %d", sum.ProcessAvg, sum.ProcessMax),
				fmt.Sprintf(" Proc now %d", st.Process),
			},
		},
		{
			Title: "Current",
			Lines: []string{
				fmt.Sprintf(" Samples %d", len(records)),
				fmt.Sprintf(" Seen    %s", timeText(st.Time)),
				fmt.Sprintf(" Net now %s %s", a.style.up(), speedIEC(st.NetOut)),
				fmt.Sprintf("         %s %s", a.style.down(), speedIEC(st.NetIn)),
			},
		},
	}
}

func (a *App) realtimePingLines(st komari.Status, maxLines int) []string {
	if len(st.Ping) == 0 || maxLines <= 0 {
		return nil
	}
	keys := make([]string, 0, len(st.Ping))
	for key := range st.Ping {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, min(len(keys), maxLines))
	for _, key := range keys {
		ping := st.Ping[key]
		name := valueOr(a.text(ping.Name), key)
		lines = append(lines, fmt.Sprintf(" %-10s now %4.0f avg %4.0f loss %.1f%%",
			cleanLine(name, 10), ping.Latest, ping.Avg, ping.Loss))
		if len(lines) >= maxLines {
			break
		}
	}
	return lines
}

func (a *App) nodeTabDetails(node komari.Node, st komari.Status, width int, maxLines int) []string {
	var lines []string
	switch a.tab {
	case 1:
		lines = a.nodeInfoCardDetails(node)
	case 2:
		lines = a.historyCardDetails(node, width)
	case 3:
		lines = a.pingCardDetails(node, st)
	case 4:
		lines = a.metaCardDetails(node)
	default:
		lines = a.overviewCardDetails(node, st, width)
	}
	return limitRawLines(lines, maxLines)
}

func (a *App) overviewCardDetails(node komari.Node, st komari.Status, width int) []string {
	ramPct := percent(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal))
	diskPct := percent(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal))
	barWidth := max(6, width-14)
	return []string{
		fmt.Sprintf(" CPU  %5.1f%% %s", st.CPU, a.style.bar(st.CPU, barWidth)),
		fmt.Sprintf(" RAM  %5.1f%% %s", ramPct, a.style.bar(ramPct, barWidth)),
		fmt.Sprintf(" DISK %5.1f%% %s", diskPct, a.style.bar(diskPct, barWidth)),
		fmt.Sprintf(" NET  %s %s  %s %s", a.style.up(), speedIEC(st.NetOut), a.style.down(), speedIEC(st.NetIn)),
		fmt.Sprintf(" FLOW %s %s  %s %s", a.style.up(), bytesIEC(st.NetTotalUp), a.style.down(), bytesIEC(st.NetTotalDown)),
		fmt.Sprintf(" LOAD %.2f %.2f %.2f | UP %s", st.Load, st.Load5, st.Load15, durationCompact(st.Uptime)),
	}
}

func (a *App) nodeInfoCardDetails(node komari.Node) []string {
	return []string{
		fmt.Sprintf(" OS   %s / %s", valueOr(a.text(node.OS), "-"), valueOr(a.text(node.Arch), "-")),
		fmt.Sprintf(" KERN %s", valueOr(a.text(node.KernelVersion), "-")),
		fmt.Sprintf(" CPU  %s", valueOr(a.text(node.CPUName), "-")),
		fmt.Sprintf(" CORE %s | VIRT %s", coreText(node), valueOr(a.text(node.Virtualization), "-")),
		fmt.Sprintf(" MEM  %s | DISK %s", bytesIEC(node.MemTotal), bytesIEC(node.DiskTotal)),
		fmt.Sprintf(" GPU  %s", valueOr(a.text(node.GPUName), "-")),
	}
}

func (a *App) historyCardDetails(node komari.Node, width int) []string {
	detail := a.currentDetail(node.UUID)
	st := a.snapshot.Status[node.UUID]
	var records []komari.Status
	if detailWindows[a.window].Hours == 0 {
		records = realtimeStatusRecords(detail.Recent.Records, st)
	} else if len(detail.Load.Records) > 0 {
		records = detail.Load.Records
	} else {
		if detail.Loading {
			return []string{
				" Loading load history...",
				" Press d to force refresh.",
			}
		}
		if detail.Err != nil {
			return []string{
				" ERR " + detail.Err.Error(),
				" Press d to retry.",
			}
		}
		return []string{
			" No history loaded.",
			" Select this node or press d.",
		}
	}

	sum := summarizeStatusWithTotals(records, node.MemTotal, node.DiskTotal)
	sparkWidth := max(6, width-9)
	return []string{
		fmt.Sprintf(" SAMPLES %-4d CPU %.1f RAM %.1f", len(records), sum.CPUAvg, sum.RAMAvg),
		fmt.Sprintf(" DISK %.1f NET %s/%s", sum.DiskAvg, speedIEC(int64(sum.NetOutAvg)), speedIEC(int64(sum.NetInAvg))),
		fmt.Sprintf(" CONN avg %.0f max %d", sum.ConnectionsAvg, sum.ConnectionsMax),
		fmt.Sprintf(" PROC avg %.0f max %d", sum.ProcessAvg, sum.ProcessMax),
		" CPU  " + sparklineLimit(statusValues(records, func(st komari.Status) float64 { return st.CPU }), a.style.ASCII, sparkWidth),
		" NET  " + sparklineLimit(statusValues(records, func(st komari.Status) float64 { return float64(st.NetOut + st.NetIn) }), a.style.ASCII, sparkWidth),
	}
}

func (a *App) pingCardDetails(node komari.Node, st komari.Status) []string {
	lines := make([]string, 0, 8)
	if len(st.Ping) > 0 {
		keys := make([]string, 0, len(st.Ping))
		for key := range st.Ping {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			ping := st.Ping[key]
			name := valueOr(a.text(ping.Name), key)
			lines = append(lines, fmt.Sprintf(" NOW %-8s %4.0fms avg %4.0f loss %.1f%%",
				cleanLine(name, 8), ping.Latest, ping.Avg, ping.Loss))
			if len(lines) >= 3 {
				break
			}
		}
	}

	detail := a.currentDetail(node.UUID)
	if detailWindows[a.window].Hours == 0 {
		records := realtimeStatusRecords(detail.Recent.Records, st)
		sparkWidth := 18
		if values := realtimePingStatusValues(records, func(ping komari.Ping) float64 { return ping.Latest }); len(values) > 0 {
			lines = append(lines, " LAT "+sparklineLimit(values, a.style.ASCII, sparkWidth))
		}
		if values := realtimePingStatusValues(records, func(ping komari.Ping) float64 { return ping.Loss }); len(values) > 0 {
			lines = append(lines, " LOS "+sparklineLimit(values, a.style.ASCII, sparkWidth))
		}
		if len(lines) == 0 {
			lines = append(lines, " No ping data. Press d.")
		}
		return lines
	}
	if detail.Loading {
		lines = append(lines, " Loading 6h ping history...")
	}
	if detail.Err != nil {
		lines = append(lines, " ERR "+detail.Err.Error())
	}
	if len(detail.Ping.Tasks) > 0 {
		for _, task := range detail.Ping.Tasks {
			lines = append(lines, fmt.Sprintf(" 6H  %-8s avg %4.0f loss %.1f%%",
				cleanLine(a.text(task.Name), 8), task.Avg, task.Loss))
			if len(lines) >= 6 {
				break
			}
		}
	} else if len(lines) == 0 {
		lines = append(lines, " No ping data. Press d.")
	}
	return lines
}

func (a *App) metaCardDetails(node komari.Node) []string {
	return []string{
		" UUID " + shortID(node.UUID),
		" IPv4 " + a.nodeMetaValue(node.IPv4),
		" IPv6 " + a.nodeMetaValue(node.IPv6),
		" AGENT " + valueOr(a.text(node.Version), "-"),
		fmt.Sprintf(" KOMARI %s RPC %s", valueOr(a.snapshot.Version.Version, "-"), valueOr(a.snapshot.RPCVersion, "-")),
		fmt.Sprintf(" AUTH %s | METHODS %d", a.authText(), len(a.snapshot.Methods)),
	}
}

func (a *App) nodeMetaValue(value string) string {
	if strings.TrimSpace(value) != "" {
		return a.text(value)
	}
	if !a.client.HasAPIKey() {
		return "api-key required"
	}
	if a.snapshot.NodeDetailErr != nil {
		return "detail unavailable"
	}
	return "-"
}

func (a *App) currentDetail(uuid string) nodeDetail {
	return a.nodeDetail[detailKey{UUID: uuid, Window: a.window}]
}

func emptyCard(width int, height int) []string {
	lines := make([]string, 0, height)
	for i := 0; i < height; i++ {
		lines = append(lines, fitLine("", width))
	}
	return lines
}

func (a *App) renderLineBody(width int, bodyHeight int) []string {
	if bodyHeight <= 0 {
		return nil
	}
	if len(a.snapshot.Nodes) == 0 {
		if a.loading {
			return fillBody([]string{"Loading nodes..."}, width, bodyHeight)
		}
		return fillBody([]string{"No nodes returned by Komari."}, width, bodyHeight)
	}

	a.clampSelection()
	header := a.lineTableHeader(width)
	visibleRows := max(1, bodyHeight-len(header))
	a.adjustScroll(visibleRows)

	lines := make([]string, 0, bodyHeight)
	lines = append(lines, header...)
	end := min(len(a.snapshot.Nodes), a.scroll+visibleRows)
	for i := a.scroll; i < end; i++ {
		lines = append(lines, a.lineTableRow(i, a.snapshot.Nodes[i], width))
	}
	return fillBody(lines, width, bodyHeight)
}

func (a *App) lineTableHeader(width int) []string {
	title := a.lineTableColumns(width, false, komari.Node{}, komari.Status{}, false)
	return []string{
		a.style.bold(fitLine("Servers "+a.style.dim(fmt.Sprintf("(%d nodes)", len(a.snapshot.Nodes))), width)),
		a.style.dim(fitLine(title, width)),
		a.style.dim(fitLine(a.style.separator(width), width)),
	}
}

func (a *App) lineTableRow(index int, node komari.Node, width int) string {
	st := a.snapshot.Status[node.UUID]
	selected := index == a.selected
	line := a.lineTableColumns(width, true, node, st, selected)
	if index == a.selected {
		return a.style.inverse(fitLine(line, width))
	}
	if !st.Online {
		return a.style.dim(fitLine(line, width))
	}
	return fitLine(line, width)
}

func (a *App) lineTableColumns(width int, row bool, node komari.Node, st komari.Status, selected bool) string {
	nameWidth := 28
	regionWidth := 10
	netWidth := 19
	trafficWidth := 19
	uptimeWidth := 10
	loadWidth := 16
	osWidth := 12
	expWidth := 8
	tagWidth := 16
	showDisk := width >= 72
	showRegion := width >= 86
	showNet := width >= 104
	showLoad := width >= 122
	showUptime := width >= 136
	showTraffic := width >= 156
	showRuntime := width >= 174
	showExp := width >= 186
	showOS := width >= 202
	showTags := width >= 220
	if width < 64 {
		nameWidth = max(12, width-34)
	} else if width < 86 {
		nameWidth = 24
	}

	if !row {
		parts := []string{
			fmt.Sprintf("   %-3s %-*s %7s %7s", "ST", nameWidth, "NODE", "CPU", "RAM"),
		}
		if showDisk {
			parts = append(parts, fmt.Sprintf(" %7s", "DISK"))
		}
		if showRegion {
			parts = append(parts, fmt.Sprintf(" %-*s", regionWidth, "REGION"))
		}
		if showNet {
			parts = append(parts, fmt.Sprintf(" %-*s", netWidth, "NET"))
		}
		if showLoad {
			parts = append(parts, fmt.Sprintf(" %-*s", loadWidth, "LOAD"))
		}
		if showUptime {
			parts = append(parts, fmt.Sprintf(" %-*s", uptimeWidth, "UPTIME"))
		}
		if showTraffic {
			parts = append(parts, fmt.Sprintf(" %-*s", trafficWidth, "TRAFFIC"))
		}
		if showRuntime {
			parts = append(parts, fmt.Sprintf(" %5s %4s", "CONN", "PROC"))
		}
		if showExp {
			parts = append(parts, fmt.Sprintf(" %-*s", expWidth, "EXP"))
		}
		if showOS {
			parts = append(parts, fmt.Sprintf(" %-*s", osWidth, "OS"))
		}
		if showTags {
			parts = append(parts, fmt.Sprintf(" %-*s", tagWidth, "TAG"))
		}
		return strings.Join(parts, "")
	}

	marker := " "
	if selected {
		marker = ">"
	}
	state := "on"
	if !st.Online {
		state = "off"
	}
	if !a.style.ASCII {
		if st.Online {
			state = "●"
		} else {
			state = "●"
		}
		if !selected {
			if st.Online {
				state = a.style.green(state)
			} else {
				state = a.style.red(state)
			}
		}
	}
	stateCell := padRight(state, 3)
	parts := []string{
		fmt.Sprintf(" %s %s %-*s %6.1f%% %6.1f%%",
			marker,
			stateCell,
			nameWidth,
			cleanLine(a.nodeLabel(node), nameWidth),
			st.CPU,
			percent(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal)),
		),
	}
	if showDisk {
		parts = append(parts, fmt.Sprintf(" %6.1f%%", percent(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal))))
	}
	if showRegion {
		parts = append(parts, fmt.Sprintf(" %-*s", regionWidth, cleanLine(valueOr(a.text(node.Region), "-"), regionWidth)))
	}
	if showNet {
		net := fmt.Sprintf("%s %s %s %s", a.style.up(), speedIEC(st.NetOut), a.style.down(), speedIEC(st.NetIn))
		parts = append(parts, fmt.Sprintf(" %-*s", netWidth, cleanLine(net, netWidth)))
	}
	if showLoad {
		load := fmt.Sprintf("%.2f %.2f %.2f", st.Load, st.Load5, st.Load15)
		parts = append(parts, fmt.Sprintf(" %-*s", loadWidth, cleanLine(load, loadWidth)))
	}
	if showUptime {
		parts = append(parts, fmt.Sprintf(" %-*s", uptimeWidth, durationCompact(st.Uptime)))
	}
	if showTraffic {
		traffic := fmt.Sprintf("%s %s %s %s", a.style.up(), bytesIEC(st.NetTotalUp), a.style.down(), bytesIEC(st.NetTotalDown))
		parts = append(parts, fmt.Sprintf(" %-*s", trafficWidth, cleanLine(traffic, trafficWidth)))
	}
	if showRuntime {
		parts = append(parts, fmt.Sprintf(" %5d %4d", st.Connections, st.Process))
	}
	if showExp {
		parts = append(parts, fmt.Sprintf(" %-*s", expWidth, cleanLine(expiryText(node, time.Now()), expWidth)))
	}
	if showOS {
		parts = append(parts, fmt.Sprintf(" %-*s", osWidth, cleanLine(valueOr(a.text(node.OS), "-"), osWidth)))
	}
	if showTags {
		tag := strings.TrimSpace(valueOr(a.text(node.Group), "") + " " + valueOr(a.text(node.Tags), ""))
		parts = append(parts, fmt.Sprintf(" %-*s", tagWidth, cleanLine(valueOr(tag, "-"), tagWidth)))
	}
	return strings.Join(parts, "")
}

func fillBody(lines []string, width int, height int) []string {
	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		out = append(out, fitLine(line, width))
	}
	return out
}

func (a *App) writeFrame(b *strings.Builder, width int, height int, lines []string) {
	if width <= 0 || height <= 0 {
		return
	}
	drawWidth := width
	if drawWidth > 1 {
		drawWidth--
	}
	b.WriteString("\x1b[2J")
	for row := 0; row < height; row++ {
		line := ""
		if row < len(lines) {
			line = lines[row]
		}
		b.WriteString(fmt.Sprintf("\x1b[%d;1H", row+1))
		b.WriteString(fitLine(line, drawWidth))
		b.WriteString("\x1b[K")
	}
}

func (a *App) headerLines(width int) []string {
	title := a.text(a.snapshot.Public.SiteName)
	if title == "" {
		title = "Komari TUI"
	}
	status := "READY"
	if a.loading {
		status = "LOADING"
	}
	if a.err != nil {
		status = "ERROR"
	}
	mode := "utf8"
	if a.style.ASCII {
		mode = "ascii"
	}

	head := a.style.inverse(headerAlign(" ktui  "+title, a.authText()+"  "+status+" ", width))
	onlineValue := fmt.Sprintf("%d/%d", a.snapshot.Online, a.snapshot.Total)
	switch {
	case a.snapshot.Total == 0:
		onlineValue = a.style.dim(onlineValue)
	case a.snapshot.Online == a.snapshot.Total:
		onlineValue = a.style.green(onlineValue)
	case a.snapshot.Online == 0:
		onlineValue = a.style.red(onlineValue)
	default:
		onlineValue = a.style.yellow(onlineValue)
	}
	summary := strings.Join([]string{
		a.headerChip("online", onlineValue),
		a.headerChip("regions", fmt.Sprintf("%d", len(a.snapshot.RegionList))),
		a.headerChip("updated", shortTime(a.snapshot.FetchedAt)),
	}, "  ")

	transfer := strings.Join([]string{
		a.headerChip("traffic", fmt.Sprintf("%s %s  %s %s",
			a.style.up(),
			headerCompactUnit(bytesIEC(a.snapshot.TotalUp)),
			a.style.down(),
			headerCompactUnit(bytesIEC(a.snapshot.TotalDown)),
		)),
		a.headerChip("speed", fmt.Sprintf("%s %s  %s %s",
			a.style.up(),
			headerCompactUnit(speedIEC(a.snapshot.SpeedUp)),
			a.style.down(),
			headerCompactUnit(speedIEC(a.snapshot.SpeedDown)),
		)),
	}, "  ")

	contextParts := []string{
		a.headerChip("view", "list"),
		a.headerChip("mode", string(a.mode)+"/"+mode),
		a.headerChip("komari", valueOr(a.snapshot.Version.Version, "-")),
		a.headerChip("rpc", valueOr(a.snapshot.RPCVersion, "-")),
	}
	if a.detail {
		nodeName := "-"
		if len(a.snapshot.Nodes) > 0 {
			nodeName = a.nodeLabel(a.snapshot.Nodes[a.selected])
		}
		contextParts = []string{
			a.headerChip("view", "detail"),
			a.headerChip("node", nodeName),
			a.headerChip("komari", valueOr(a.snapshot.Version.Version, "-")),
			a.headerChip("rpc", valueOr(a.snapshot.RPCVersion, "-")),
		}
	}
	context := strings.Join(contextParts, "  ")
	if a.err != nil {
		context = a.style.red("ERROR " + a.err.Error())
	}

	lines := []string{
		head,
		fitLine(summary, width),
		fitLine(transfer, width),
		fitLine(context, width),
	}
	return lines
}

func (a *App) headerChip(label string, value string) string {
	if strings.TrimSpace(value) == "" {
		value = "-"
	}
	return a.style.dim(strings.ToUpper(label)) + " " + value
}

func headerAlign(left string, right string, width int) string {
	left = cleanLine(left, width)
	right = cleanLine(right, width)
	gap := width - displayWidth(left) - displayWidth(right)
	if gap < 1 {
		return fitLine(left+" "+right, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

func headerCompactUnit(value string) string {
	return strings.ReplaceAll(value, " ", "")
}

func (a *App) footerLine(width int) string {
	footer := " ↑↓/jk select   Enter detail   m mode   r refresh   a ascii   q quit "
	if a.detail {
		footer = " Esc/q back   1-5/h/l tabs   [ ] window   j/k scroll   r refresh   d reload "
	}
	return a.style.inverse(cleanLine(footer, width))
}

func (a *App) adjustScroll(visibleRows int) {
	if a.selected < a.scroll {
		a.scroll = a.selected
	}
	if a.selected >= a.scroll+visibleRows {
		a.scroll = a.selected - visibleRows + 1
	}
	if a.scroll < 0 {
		a.scroll = 0
	}
	maxScroll := len(a.snapshot.Nodes) - visibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	if a.scroll > maxScroll {
		a.scroll = maxScroll
	}
}

func (a *App) panelLines(width int, maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	switch a.tab {
	case 0:
		return a.overviewLines(width, maxLines)
	case 1:
		return a.detailLines(width, maxLines)
	case 2:
		return a.historyLines(width, maxLines)
	case 3:
		return a.pingLines(width, maxLines)
	default:
		return a.metaLines(width, maxLines)
	}
}

func (a *App) overviewLines(width int, maxLines int) []string {
	if len(a.snapshot.Nodes) == 0 {
		lines := []string{a.style.bold("Overview")}
		return limitLines(append(lines, "No data yet."), width, maxLines)
	}
	node := a.snapshot.Nodes[a.selected]
	st := a.snapshot.Status[node.UUID]
	ramTotal := firstNonZero(st.RAMTotal, node.MemTotal)
	diskTotal := firstNonZero(st.DiskTotal, node.DiskTotal)
	lines := []string{a.style.bold("Overview: " + a.text(node.Name))}
	lines = append(lines,
		fmt.Sprintf("Status      %s", a.style.coloredStatus(st.Online)),
		fmt.Sprintf("Seen        %s", timeText(st.Time)),
		fmt.Sprintf("Uptime      %s", durationCompact(st.Uptime)),
		fmt.Sprintf("Region      %s", valueOr(a.text(node.Region), "-")),
		fmt.Sprintf("Group       %s", valueOr(a.text(node.Group), "-")),
		"",
		fmt.Sprintf("CPU         %5.1f%%  %s", st.CPU, a.style.bar(st.CPU, fitBar(width, 28))),
		fmt.Sprintf("RAM         %s", usage(st.RAM, ramTotal)),
		fmt.Sprintf("Disk        %s", usage(st.Disk, diskTotal)),
		fmt.Sprintf("Swap        %s", usage(st.Swap, firstNonZero(st.SwapTotal, node.SwapTotal))),
		fmt.Sprintf("Load        %.2f %.2f %.2f", st.Load, st.Load5, st.Load15),
		fmt.Sprintf("Net         %s %s  %s %s", a.style.up(), speedIEC(st.NetOut), a.style.down(), speedIEC(st.NetIn)),
		fmt.Sprintf("Traffic     %s %s  %s %s", a.style.up(), bytesIEC(st.NetTotalUp), a.style.down(), bytesIEC(st.NetTotalDown)),
		fmt.Sprintf("Processes   %d", st.Process),
		fmt.Sprintf("Conn        TCP %d UDP %d", st.Connections, st.ConnectionsUDP),
	)
	if node.TrafficLimit > 0 {
		pct := trafficPercent(st.NetTotalUp, st.NetTotalDown, node.TrafficLimit, node.TrafficLimitType)
		lines = append(lines, fmt.Sprintf("Limit       %.1f%% of %s (%s)", pct, bytesIEC(node.TrafficLimit), node.TrafficLimitType))
	}
	return limitLines(lines, width, maxLines)
}

func (a *App) detailLines(width int, maxLines int) []string {
	if len(a.snapshot.Nodes) == 0 {
		return []string{}
	}
	node := a.snapshot.Nodes[a.selected]
	st := a.snapshot.Status[node.UUID]
	lines := []string{a.style.bold("Node")}

	lines = append(lines,
		fmt.Sprintf("%s  %s", a.text(node.Name), a.style.coloredStatus(st.Online)),
		fmt.Sprintf("UUID    %s", node.UUID),
		fmt.Sprintf("Region  %s", valueOr(a.text(node.Region), "-")),
		fmt.Sprintf("Group   %s", valueOr(a.text(node.Group), "-")),
		fmt.Sprintf("OS      %s / %s", valueOr(a.text(node.OS), "-"), valueOr(a.text(node.Arch), "-")),
		fmt.Sprintf("Kernel  %s", valueOr(a.text(node.KernelVersion), "-")),
		fmt.Sprintf("CPU     %s (%d cores, %d physical)", valueOr(a.text(node.CPUName), "-"), node.CPUCores, node.CPUPhysicalCores),
		fmt.Sprintf("GPU     %s", valueOr(a.text(node.GPUName), "-")),
		fmt.Sprintf("Virt    %s", valueOr(a.text(node.Virtualization), "-")),
		fmt.Sprintf("Agent   %s", valueOr(a.text(node.Version), "-")),
		"",
		fmt.Sprintf("CPU     %5.1f%%  %s", st.CPU, a.style.bar(st.CPU, fitBar(width, 28))),
		fmt.Sprintf("RAM     %s", usage(firstNonZero(st.RAM, 0), firstNonZero(st.RAMTotal, node.MemTotal))),
		fmt.Sprintf("Disk    %s", usage(firstNonZero(st.Disk, 0), firstNonZero(st.DiskTotal, node.DiskTotal))),
		fmt.Sprintf("Swap    %s", usage(st.Swap, firstNonZero(st.SwapTotal, node.SwapTotal))),
		fmt.Sprintf("Load    %.2f %.2f %.2f", st.Load, st.Load5, st.Load15),
		fmt.Sprintf("Net     %s %s  %s %s", a.style.up(), speedIEC(st.NetOut), a.style.down(), speedIEC(st.NetIn)),
		fmt.Sprintf("Traffic %s %s  %s %s", a.style.up(), bytesIEC(st.NetTotalUp), a.style.down(), bytesIEC(st.NetTotalDown)),
		fmt.Sprintf("Uptime  %s", durationCompact(st.Uptime)),
		fmt.Sprintf("Proc    %d   Conn TCP %d UDP %d", st.Process, st.Connections, st.ConnectionsUDP),
		fmt.Sprintf("Seen    %s", timeText(st.Time)),
	)

	if node.IPv4 != "" || node.IPv6 != "" {
		lines = append(lines,
			"",
			fmt.Sprintf("IPv4    %s", valueOr(node.IPv4, "-")),
			fmt.Sprintf("IPv6    %s", valueOr(node.IPv6, "-")),
		)
	}
	if node.TrafficLimit > 0 {
		pct := trafficPercent(st.NetTotalUp, st.NetTotalDown, node.TrafficLimit, node.TrafficLimitType)
		lines = append(lines, fmt.Sprintf("Limit   %.1f%% of %s (%s)", pct, bytesIEC(node.TrafficLimit), node.TrafficLimitType))
	}
	if node.Price != 0 || node.ExpiredAt.Valid || node.Tags != "" || node.PublicRemark != "" || node.Remark != "" {
		price := "-"
		if node.Price > 0 {
			price = fmt.Sprintf("%s%.2f / %dd", valueOr(node.Currency, ""), node.Price, node.BillingCycle)
		} else if node.Price < 0 {
			price = "free"
		}
		lines = append(lines,
			"",
			fmt.Sprintf("Billing %s auto=%t", price, node.AutoRenewal),
			fmt.Sprintf("Expire  %s", nullableDate(node.ExpiredAt)),
			fmt.Sprintf("Tags    %s", valueOr(a.text(node.Tags), "-")),
			fmt.Sprintf("Public  %s", valueOr(a.text(node.PublicRemark), "-")),
			fmt.Sprintf("Remark  %s", valueOr(a.text(node.Remark), "-")),
		)
	}

	return limitLines(lines, width, maxLines)
}

func (a *App) historyLines(width int, maxLines int) []string {
	if len(a.snapshot.Nodes) == 0 {
		return nil
	}
	node := a.snapshot.Nodes[a.selected]
	window := detailWindows[a.window]
	detail := a.currentDetail(node.UUID)
	lines := []string{a.style.bold(fmt.Sprintf("History: %s  window %s", a.text(node.Name), window.Label))}
	if detail.Loading {
		lines = append(lines, "Loading records...")
	}
	if detail.Err != nil {
		lines = append(lines, a.style.red("detail error: "+detail.Err.Error()))
	}
	var records []komari.Status
	if window.Hours == 0 {
		st := a.snapshot.Status[node.UUID]
		records = realtimeStatusRecords(detail.Recent.Records, st)
	} else {
		records = detail.Load.Records
	}
	if len(records) == 0 {
		lines = append(lines, "No load history loaded yet. Press d to refresh details.")
		return limitLines(lines, width, maxLines)
	}

	sum := summarizeStatusWithTotals(records, node.MemTotal, node.DiskTotal)
	sparkWidth := fitBar(width, 10)
	lines = append(lines,
		fmt.Sprintf("Window  %s - %s   samples %d", nullableDateTime(detail.Load.From), nullableDateTime(detail.Load.To), len(records)),
		fmt.Sprintf("CPU     avg %5.1f%%  max %5.1f%%", sum.CPUAvg, sum.CPUMax),
		fmt.Sprintf("RAM     avg %5.1f%%  max %5.1f%%", sum.RAMAvg, sum.RAMMax),
		fmt.Sprintf("Disk    avg %5.1f%%  max %5.1f%%", sum.DiskAvg, sum.DiskMax),
		fmt.Sprintf("Load    avg %.2f  max %.2f", sum.LoadAvg, sum.LoadMax),
		fmt.Sprintf("Net     avg %s %s  %s %s", a.style.up(), speedIEC(int64(sum.NetOutAvg)), a.style.down(), speedIEC(int64(sum.NetInAvg))),
		fmt.Sprintf("Conn    avg %.0f  max %d", sum.ConnectionsAvg, sum.ConnectionsMax),
		fmt.Sprintf("Proc    avg %.0f  max %d", sum.ProcessAvg, sum.ProcessMax),
		"",
		"CPU spark",
		"  "+sparklineLimit(statusValues(records, func(st komari.Status) float64 { return st.CPU }), a.style.ASCII, sparkWidth),
		"RAM spark",
		"  "+sparklineLimit(statusRAMPercentValues(records, node.MemTotal), a.style.ASCII, sparkWidth),
		"Disk spark",
		"  "+sparklineLimit(statusDiskPercentValues(records, node.DiskTotal), a.style.ASCII, sparkWidth),
		"Net out spark",
		"  "+sparklineLimit(statusValues(records, func(st komari.Status) float64 { return float64(st.NetOut) }), a.style.ASCII, sparkWidth),
		"Net in spark",
		"  "+sparklineLimit(statusValues(records, func(st komari.Status) float64 { return float64(st.NetIn) }), a.style.ASCII, sparkWidth),
		"Connections spark",
		"  "+sparklineLimit(statusValues(records, func(st komari.Status) float64 { return float64(st.Connections) }), a.style.ASCII, sparkWidth),
		"Process spark",
		"  "+sparklineLimit(statusValues(records, func(st komari.Status) float64 { return float64(st.Process) }), a.style.ASCII, sparkWidth),
	)
	return limitLines(lines, width, maxLines)
}

func (a *App) pingLines(width int, maxLines int) []string {
	if len(a.snapshot.Nodes) == 0 {
		return nil
	}
	node := a.snapshot.Nodes[a.selected]
	st := a.snapshot.Status[node.UUID]
	window := detailWindows[a.window]
	detail := a.currentDetail(node.UUID)
	lines := []string{a.style.bold(fmt.Sprintf("Ping: %s  window %s", a.text(node.Name), window.Label))}

	if len(st.Ping) > 0 {
		lines = append(lines, "Realtime")
		keys := make([]string, 0, len(st.Ping))
		for key := range st.Ping {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			ping := st.Ping[key]
			lines = append(lines, fmt.Sprintf("  %-14s latest %.0fms avg %.0fms min %.0f max %.0f loss %.1f%%",
				cleanLine(a.text(ping.Name), 14), ping.Latest, ping.Avg, ping.Min, ping.Max, ping.Loss))
		}
	}
	if window.Hours == 0 {
		if len(st.Ping) == 0 {
			lines = append(lines, "No realtime ping data.")
		} else {
			records := realtimeStatusRecords(detail.Recent.Records, st)
			sparkWidth := fitBar(width, 10)
			if values := realtimePingStatusValues(records, func(ping komari.Ping) float64 { return ping.Latest }); len(values) > 0 {
				lines = append(lines, "", "Latency spark", "  "+sparklineLimit(values, a.style.ASCII, sparkWidth))
			}
			if values := realtimePingStatusValues(records, func(ping komari.Ping) float64 { return ping.Loss }); len(values) > 0 {
				lines = append(lines, "Loss spark", "  "+sparklineLimit(values, a.style.ASCII, sparkWidth))
			}
		}
		return limitLines(lines, width, maxLines)
	}

	if detail.Loading {
		lines = append(lines, "", "Loading ping records...")
	}
	if detail.Err != nil {
		lines = append(lines, a.style.red("detail error: "+detail.Err.Error()))
	}
	if len(detail.Ping.Tasks) == 0 && len(detail.Ping.Records) == 0 {
		lines = append(lines, "", "No ping history loaded yet. Press d to refresh details.")
		return limitLines(lines, width, maxLines)
	}
	lines = append(lines, "", fmt.Sprintf("History samples %d", len(detail.Ping.Records)))
	for _, task := range detail.Ping.Tasks {
		lines = append(lines, fmt.Sprintf("  %-16s avg %.0fms min %.0f max %.0f loss %.1f%% total %d",
			cleanLine(a.text(task.Name), 16), task.Avg, task.Min, task.Max, task.Loss, task.Total))
	}
	if values := pingRecordValues(detail.Ping.Records); len(values) > 0 {
		lines = append(lines, "", "Latency spark", "  "+sparklineLimit(values, a.style.ASCII, fitBar(width, 10)))
	}
	return limitLines(lines, width, maxLines)
}

func (a *App) metaLines(width int, maxLines int) []string {
	if len(a.snapshot.Nodes) == 0 {
		return nil
	}
	node := a.snapshot.Nodes[a.selected]
	st := a.snapshot.Status[node.UUID]
	window := detailWindows[a.window]
	detail := a.currentDetail(node.UUID)
	lines := []string{a.style.bold(fmt.Sprintf("Meta: %s  window %s", a.text(node.Name), window.Label))}
	lines = append(lines,
		fmt.Sprintf("UUID       %s", node.UUID),
		fmt.Sprintf("IPv4       %s", a.nodeMetaValue(node.IPv4)),
		fmt.Sprintf("IPv6       %s", a.nodeMetaValue(node.IPv6)),
		fmt.Sprintf("Agent      %s", valueOr(a.text(node.Version), "-")),
		fmt.Sprintf("Seen       %s", timeText(st.Time)),
		fmt.Sprintf("Online     %t", st.Online),
		"",
		fmt.Sprintf("Komari     %s", valueOr(a.snapshot.Version.Version, "-")),
		fmt.Sprintf("Hash       %s", valueOr(a.snapshot.Version.Hash, "-")),
		fmt.Sprintf("RPC        %s", valueOr(a.snapshot.RPCVersion, "-")),
		fmt.Sprintf("Auth       logged_in=%t user=%s uuid=%s", a.snapshot.Me.LoggedIn, valueOr(a.snapshot.Me.Username, "-"), valueOr(a.snapshot.Me.UUID, "-")),
		fmt.Sprintf("Private    %t", a.snapshot.Public.PrivateSite),
		fmt.Sprintf("CORS       %t", a.snapshot.Public.CORSOriginCheckEnabled),
		fmt.Sprintf("OAuth      %t %s", a.snapshot.Public.OAuthEnable, valueOr(a.snapshot.Public.OAuthProvider, "-")),
		fmt.Sprintf("Methods    %d", len(a.snapshot.Methods)),
	)
	if window.Hours > 0 {
		lines = append(lines,
			"",
			fmt.Sprintf("Window     %s", window.Label),
			fmt.Sprintf("Load recs  %d", len(detail.Load.Records)),
			fmt.Sprintf("Ping recs  %d", len(detail.Ping.Records)),
			fmt.Sprintf("From       %s", nullableDateTime(detail.Load.From)),
			fmt.Sprintf("To         %s", nullableDateTime(detail.Load.To)),
		)
		if detail.Loading {
			lines = append(lines, "Detail     loading")
		}
		if detail.Err != nil {
			lines = append(lines, "Detail     "+detail.Err.Error())
		}
	} else {
		lines = append(lines,
			"",
			"Realtime status",
			fmt.Sprintf("CPU        %.1f%%", st.CPU),
			fmt.Sprintf("RAM        %s", usage(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal))),
			fmt.Sprintf("Disk       %s", usage(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal))),
			fmt.Sprintf("Net        %s %s  %s %s", a.style.up(), speedIEC(st.NetOut), a.style.down(), speedIEC(st.NetIn)),
		)
	}
	if len(a.snapshot.Methods) > 0 {
		lines = append(lines, "")
		for _, method := range a.snapshot.Methods {
			lines = append(lines, "  "+method)
		}
	}
	return limitLines(lines, width, maxLines)
}

func (a *App) nodeLabel(node komari.Node) string {
	name := a.text(node.Name)
	region := a.text(node.Region)
	if a.style.ASCII || region == "" {
		return name
	}
	return region + " " + name
}

func (a *App) text(value string) string {
	return a.style.sanitizeText(value)
}

func limitLines(lines []string, width int, maxLines int) []string {
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	for i := range lines {
		lines[i] = fitLine(lines[i], width)
	}
	return lines
}

type statusSummary struct {
	CPUAvg         float64
	CPUMax         float64
	RAMAvg         float64
	RAMMax         float64
	DiskAvg        float64
	DiskMax        float64
	LoadAvg        float64
	LoadMax        float64
	NetOutAvg      float64
	NetInAvg       float64
	NetOutMax      int64
	NetInMax       int64
	ConnectionsAvg float64
	ConnectionsMax int
	ProcessAvg     float64
	ProcessMax     int
}

func summarizeStatus(records []komari.Status) statusSummary {
	return summarizeStatusWithTotals(records, 0, 0)
}

func summarizeStatusWithTotals(records []komari.Status, ramTotalFallback int64, diskTotalFallback int64) statusSummary {
	var sum statusSummary
	if len(records) == 0 {
		return sum
	}
	var cpuTotal, ramTotal, diskTotal, loadTotal, netInTotal, netOutTotal, connectionsTotal, processTotal float64
	for _, st := range records {
		cpuTotal += st.CPU
		sum.CPUMax = maxFloat(sum.CPUMax, st.CPU)
		ramPct := percent(st.RAM, firstNonZero(st.RAMTotal, ramTotalFallback))
		ramTotal += ramPct
		sum.RAMMax = maxFloat(sum.RAMMax, ramPct)
		diskPct := percent(st.Disk, firstNonZero(st.DiskTotal, diskTotalFallback))
		diskTotal += diskPct
		sum.DiskMax = maxFloat(sum.DiskMax, diskPct)
		loadTotal += st.Load
		sum.LoadMax = maxFloat(sum.LoadMax, st.Load)
		netInTotal += float64(st.NetIn)
		netOutTotal += float64(st.NetOut)
		connectionsTotal += float64(st.Connections)
		if st.Connections > sum.ConnectionsMax {
			sum.ConnectionsMax = st.Connections
		}
		processTotal += float64(st.Process)
		if st.Process > sum.ProcessMax {
			sum.ProcessMax = st.Process
		}
		if st.NetIn > sum.NetInMax {
			sum.NetInMax = st.NetIn
		}
		if st.NetOut > sum.NetOutMax {
			sum.NetOutMax = st.NetOut
		}
	}
	count := float64(len(records))
	sum.CPUAvg = cpuTotal / count
	sum.RAMAvg = ramTotal / count
	sum.DiskAvg = diskTotal / count
	sum.LoadAvg = loadTotal / count
	sum.NetInAvg = netInTotal / count
	sum.NetOutAvg = netOutTotal / count
	sum.ConnectionsAvg = connectionsTotal / count
	sum.ProcessAvg = processTotal / count
	return sum
}

func statusValues(records []komari.Status, pick func(komari.Status) float64) []float64 {
	values := make([]float64, 0, len(records))
	for _, record := range records {
		values = append(values, pick(record))
	}
	return values
}

func statusRAMPercentValues(records []komari.Status, fallbackTotal int64) []float64 {
	values := make([]float64, 0, len(records))
	for _, record := range records {
		values = append(values, percent(record.RAM, firstNonZero(record.RAMTotal, fallbackTotal)))
	}
	return values
}

func statusDiskPercentValues(records []komari.Status, fallbackTotal int64) []float64 {
	values := make([]float64, 0, len(records))
	for _, record := range records {
		values = append(values, percent(record.Disk, firstNonZero(record.DiskTotal, fallbackTotal)))
	}
	return values
}

func firstRecordTime(records []komari.Status) komari.NullTime {
	for _, record := range records {
		if record.Time.Valid {
			return record.Time
		}
	}
	return komari.NullTime{}
}

func lastRecordTime(records []komari.Status) komari.NullTime {
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Time.Valid {
			return records[i].Time
		}
	}
	return komari.NullTime{}
}

func chartTimeLabel(t komari.NullTime) string {
	if !t.Valid {
		return "-"
	}
	return t.Time.Local().Format("01-02 15:04")
}

func minMaxFloat(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	minVal, maxVal := values[0], values[0]
	for _, value := range values[1:] {
		if value < minVal {
			minVal = value
		}
		if value > maxVal {
			maxVal = value
		}
	}
	return minVal, maxVal
}

func chartPointRow(value, minVal, maxVal float64) int {
	if maxVal == minVal {
		return 1
	}
	ratio := (value - minVal) / (maxVal - minVal)
	switch {
	case ratio >= 2.0/3.0:
		return 0
	case ratio >= 1.0/3.0:
		return 1
	default:
		return 2
	}
}

func downsampleValues(values []float64, width int) []float64 {
	if width <= 0 {
		return nil
	}
	if len(values) <= width {
		out := make([]float64, len(values))
		copy(out, values)
		return out
	}
	step := float64(len(values)) / float64(width)
	out := make([]float64, 0, width)
	for i := 0; i < width; i++ {
		start := int(float64(i) * step)
		end := int(float64(i+1) * step)
		if end <= start {
			end = start + 1
		}
		if end > len(values) {
			end = len(values)
		}
		var total float64
		for _, value := range values[start:end] {
			total += value
		}
		out = append(out, total/float64(end-start))
	}
	return out
}

func chartLabelWidth(minVal, midVal, maxVal float64, unit string) int {
	width := max(displayWidth(chartValueLabel(minVal, unit, 0)), displayWidth(chartValueLabel(midVal, unit, 0)))
	width = max(width, displayWidth(chartValueLabel(maxVal, unit, 0)))
	if width < 4 {
		width = 4
	}
	if width > 10 {
		width = 10
	}
	return width
}

func chartValueLabel(value float64, unit string, width int) string {
	label := compactFloat(value)
	if unit == "B/s" {
		label = compactByteFloat(value)
	} else if unit != "" {
		label += unit
	}
	if width <= 0 {
		return label
	}
	return padRight(cleanLine(label, width), width)
}

func compactFloat(value float64) string {
	switch {
	case value >= 1000:
		return fmt.Sprintf("%.0f", value)
	case value >= 100:
		return fmt.Sprintf("%.0f", value)
	case value >= 10:
		return fmt.Sprintf("%.1f", value)
	default:
		return fmt.Sprintf("%.2f", value)
	}
}

func compactByteFloat(value float64) string {
	units := []string{"B/s", "K/s", "M/s", "G/s", "T/s"}
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	switch {
	case value >= 100:
		return fmt.Sprintf("%.0f%s", value, units[unit])
	case value >= 10:
		return fmt.Sprintf("%.1f%s", value, units[unit])
	default:
		return fmt.Sprintf("%.2f%s", value, units[unit])
	}
}

func chartXAxisLine(from, to string, width int, labelWidth int, ascii bool) string {
	prefix := strings.Repeat(" ", labelWidth)
	if ascii {
		prefix += "+"
	} else {
		prefix += "└"
	}
	plotWidth := max(1, width-displayWidth(prefix))
	axis := strings.Repeat("-", plotWidth)
	if !ascii {
		axis = strings.Repeat("─", plotWidth)
	}
	line := prefix + axis
	label := from + " -> " + to
	if displayWidth(label) <= plotWidth {
		line = prefix + padRight(label, plotWidth)
	}
	return cleanLine(line, width)
}

func realtimeStatusRecords(recent []komari.Status, current komari.Status) []komari.Status {
	records := make([]komari.Status, 0, len(recent)+1)
	records = append(records, recent...)
	if len(records) == 0 || !sameStatusSample(records[len(records)-1], current) {
		records = append(records, current)
	}
	return records
}

func sameStatusSample(a, b komari.Status) bool {
	if a.Time.Valid && b.Time.Valid {
		return a.Time.Time.Equal(b.Time.Time)
	}
	return a.CPU == b.CPU &&
		a.RAM == b.RAM &&
		a.Disk == b.Disk &&
		a.NetIn == b.NetIn &&
		a.NetOut == b.NetOut
}

func realtimePingStatusValues(records []komari.Status, pick func(komari.Ping) float64) []float64 {
	values := make([]float64, 0, len(records))
	for _, record := range records {
		if len(record.Ping) == 0 {
			continue
		}
		var total float64
		var count int
		for _, ping := range record.Ping {
			total += pick(ping)
			count++
		}
		if count > 0 {
			values = append(values, total/float64(count))
		}
	}
	return values
}

func pingRecordValues(records []komari.PingRecord) []float64 {
	values := make([]float64, 0, len(records))
	for _, record := range records {
		values = append(values, record.Value)
	}
	return values
}

func sparkline(values []float64, ascii bool) string {
	if len(values) == 0 {
		return "-"
	}
	if len(values) > 48 {
		step := float64(len(values)) / 48
		downsampled := make([]float64, 0, 48)
		for i := 0; i < 48; i++ {
			downsampled = append(downsampled, values[int(float64(i)*step)])
		}
		values = downsampled
	}
	minVal, maxVal := values[0], values[0]
	for _, value := range values {
		if value < minVal {
			minVal = value
		}
		if value > maxVal {
			maxVal = value
		}
	}
	if maxVal == minVal {
		if ascii {
			return strings.Repeat("-", len(values))
		}
		return strings.Repeat("▁", len(values))
	}
	blocks := []rune("▁▂▃▄▅▆▇█")
	if ascii {
		blocks = []rune("._-=+*#@")
	}
	var b strings.Builder
	for _, value := range values {
		idx := int((value - minVal) / (maxVal - minVal) * float64(len(blocks)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		b.WriteRune(blocks[idx])
	}
	return b.String()
}

func sparklineLimit(values []float64, ascii bool, width int) string {
	if width <= 0 {
		return ""
	}
	if len(values) > width {
		step := float64(len(values)) / float64(width)
		downsampled := make([]float64, 0, width)
		for i := 0; i < width; i++ {
			downsampled = append(downsampled, values[int(float64(i)*step)])
		}
		values = downsampled
	}
	return cleanLine(sparkline(values, ascii), width)
}

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
