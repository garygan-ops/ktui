package tui

import (
	"context"
	"fmt"
	"time"

	"ktui/internal/komari"
)

type App struct {
	client          *komari.Client
	refreshInterval time.Duration
	fetchTimeout    time.Duration
	detailTimeout   time.Duration
	detailCacheTTL  time.Duration
	realtimePoints  int
	saveSettings    func(PersistentSettings) error
	checkUpdate     func(context.Context) (UpdateCheckResult, error)
	style           Style
	mode            Mode

	selected          int
	scroll            int
	tab               int
	window            int
	detail            bool
	chartFocus        bool
	chartFocusIndex   int
	settings          bool
	cardStep          int
	settingsWasDetail bool
	settingsSelected  int
	settingsStatus    string
	settingsURL       string
	settingsAPIKey    string
	chartYAxisMode    chartYAxisMode
	warnCPU           float64
	warnRAM           float64
	warnDisk          float64
	warnExpiryDays    int
	searchEditing     bool
	searchQuery       string
	searchDraft       string
	nodeFilter        nodeFilterMode
	nodeSort          nodeSortMode
	notice            string

	snapshot komari.Snapshot
	err      error
	loading  bool
	fetching bool
	update   updateState
	// At most one refresh is queued while an in-flight request is finishing.
	refreshPending  bool
	intervalChanged bool
	quit            bool

	marqueeFrame  int
	lastFullFetch time.Time
	realtimeNow   time.Time

	nodeDetail     map[detailKey]nodeDetail
	realtimeStatus map[string][]komari.Status

	renderCh  chan struct{}
	refreshCh chan struct{}
	resultCh  chan fetchResult
	detailCh  chan detailResult
	updateCh  chan updateResult
	keyCh     chan keyEvent
}

type Options struct {
	URL             string
	APIKey          string
	RefreshInterval time.Duration
	FetchTimeout    time.Duration
	DetailTimeout   time.Duration
	DetailCacheTTL  time.Duration
	RealtimePoints  int
	ChartYAxisMode  string
	WarnCPU         float64
	WarnRAM         float64
	WarnDisk        float64
	WarnExpiryDays  int
	SaveSettings    func(PersistentSettings) error
	CheckUpdate     func(context.Context) (UpdateCheckResult, error)
	ASCII           bool
	NoColor         bool
	Mode            Mode
}

type UpdateCheckResult struct {
	CurrentVersion string
	LatestVersion  string
	AssetName      string
	Available      bool
}

type PersistentSettings struct {
	Interval       string
	Timeout        string
	Mode           string
	RealtimePoints int
	ChartYAxisMode string
	ASCII          bool
	NoColor        bool
	WarnCPU        float64
	WarnRAM        float64
	WarnDisk       float64
	WarnExpiryDays int
}

type Mode string

const (
	ModeSheet Mode = "sheet"
	ModeLine  Mode = "line"
)

type chartYAxisMode string

const (
	chartYAxisAbsolute chartYAxisMode = "absolute"
	chartYAxisRelative chartYAxisMode = "relative"
)

type nodeFilterMode string

const (
	nodeFilterAll      nodeFilterMode = "all"
	nodeFilterOffline  nodeFilterMode = "offline"
	nodeFilterExpiring nodeFilterMode = "expiring"
	nodeFilterHighLoad nodeFilterMode = "high-load"
)

type nodeSortMode string

const (
	nodeSortDefault nodeSortMode = "default"
	nodeSortStatus  nodeSortMode = "status"
	nodeSortCPU     nodeSortMode = "cpu"
	nodeSortRAM     nodeSortMode = "ram"
	nodeSortTraffic nodeSortMode = "traffic"
	nodeSortExpiry  nodeSortMode = "expiry"
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
	full     bool
}

type detailResult struct {
	key    detailKey
	detail nodeDetail
}

type updateResult struct {
	result UpdateCheckResult
	err    error
}

type updateState struct {
	Checking  bool
	Checked   bool
	Available bool
	Latest    string
	AssetName string
	Err       error
}

type detailKey struct {
	UUID   string
	Window int
}

type keyEvent struct {
	name string
	text string
	x    int
	y    int
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
	Values     []float64
	Series     []axisSeries
	Times      []time.Time
	From       string
	To         string
	Unit       string
	Window     time.Duration
	Until      time.Time
	FixedRange bool
	Min        float64
	Max        float64
}

type axisSeries struct {
	Name   string
	Values []float64
}

const detailCardHeight = 7

const (
	defaultRefreshInterval = 5 * time.Second
	defaultFetchTimeout    = 15 * time.Second
	defaultDetailTimeout   = 20 * time.Second
	defaultDetailCacheTTL  = 45 * time.Second
	fullRefreshInterval    = 60 * time.Second
	realtimeWindowDuration = time.Minute
	maxRealtimeSamplesCap  = 1200
)

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
		opts.RefreshInterval = defaultRefreshInterval
	}
	if opts.FetchTimeout <= 0 {
		opts.FetchTimeout = defaultFetchTimeout
	}
	if opts.DetailTimeout <= 0 {
		opts.DetailTimeout = defaultDetailTimeout
	}
	if opts.DetailCacheTTL <= 0 {
		opts.DetailCacheTTL = defaultDetailCacheTTL
	}
	chartYAxisMode := chartYAxisAbsolute
	if opts.ChartYAxisMode == string(chartYAxisRelative) {
		chartYAxisMode = chartYAxisRelative
	}
	if opts.Mode == "" {
		opts.Mode = ModeSheet
	}
	if opts.WarnCPU <= 0 {
		opts.WarnCPU = 90
	}
	if opts.WarnRAM <= 0 {
		opts.WarnRAM = 85
	}
	if opts.WarnDisk <= 0 {
		opts.WarnDisk = 90
	}
	if opts.WarnExpiryDays <= 0 {
		opts.WarnExpiryDays = 7
	}
	return &App{
		client:          client,
		refreshInterval: opts.RefreshInterval,
		fetchTimeout:    opts.FetchTimeout,
		detailTimeout:   opts.DetailTimeout,
		detailCacheTTL:  opts.DetailCacheTTL,
		realtimePoints:  opts.RealtimePoints,
		checkUpdate:     opts.CheckUpdate,
		settingsURL:     opts.URL,
		settingsAPIKey:  opts.APIKey,
		style:           Style{ASCII: opts.ASCII, NoColor: opts.NoColor},
		mode:            opts.Mode,
		warnCPU:         opts.WarnCPU,
		warnRAM:         opts.WarnRAM,
		warnDisk:        opts.WarnDisk,
		warnExpiryDays:  opts.WarnExpiryDays,
		nodeFilter:      nodeFilterAll,
		nodeSort:        nodeSortDefault,
		renderCh:        make(chan struct{}, 1),
		refreshCh:       make(chan struct{}, 2),
		resultCh:        make(chan fetchResult, 2),
		detailCh:        make(chan detailResult, 4),
		updateCh:        make(chan updateResult, 1),
		keyCh:           make(chan keyEvent, 16),
		loading:         true,
		nodeDetail:      map[detailKey]nodeDetail{},
		realtimeStatus:  map[string][]komari.Status{},
		chartYAxisMode:  chartYAxisMode,
		saveSettings:    opts.SaveSettings,
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
	a.startUpdateCheck(ctx)
	a.requestRefresh()

	ticker := time.NewTicker(a.refreshInterval)
	defer ticker.Stop()
	marqueeTicker := time.NewTicker(300 * time.Millisecond)
	defer marqueeTicker.Stop()

	a.render()
	for !a.quit {
		a.resetRefreshTickerIfNeeded(ticker)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			a.advanceRealtimeNow(time.Now())
			a.render()
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
			pending := a.refreshPending
			selectedUUID := a.selectedNodeUUID()
			a.refreshPending = false
			a.loading = false
			a.fetching = false
			a.err = result.err
			if result.err == nil {
				a.snapshot = result.snapshot
				if result.full {
					a.lastFullFetch = result.snapshot.FetchedAt
				}
				a.recordRealtimeSnapshot(result.snapshot, result.snapshot.FetchedAt)
				a.restoreSelection(selectedUUID)
				if a.detail {
					a.ensureSelectedDetail(ctx)
				}
			} else {
				a.recordRealtimeSample(a.realtimeNowOrTime(time.Now()))
			}
			a.render()
			if pending {
				a.requestRefresh()
			}
		case detail := <-a.detailCh:
			a.nodeDetail[detail.key] = detail.detail
			a.render()
		case update := <-a.updateCh:
			a.update.Checking = false
			a.update.Checked = true
			a.update.Err = update.err
			if update.err == nil {
				a.update.Available = update.result.Available
				a.update.Latest = update.result.LatestVersion
				a.update.AssetName = update.result.AssetName
				if update.result.Available {
					a.notice = ""
				}
			}
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

func (a *App) startUpdateCheck(ctx context.Context) {
	if a.checkUpdate == nil {
		return
	}
	a.update.Checking = true
	go func() {
		result, err := a.checkUpdate(ctx)
		select {
		case a.updateCh <- updateResult{result: result, err: err}:
		case <-ctx.Done():
		}
	}()
}

type refreshTicker interface {
	Reset(time.Duration)
}

func (a *App) resetRefreshTickerIfNeeded(ticker refreshTicker) {
	if !a.intervalChanged {
		return
	}
	ticker.Reset(a.refreshInterval)
	a.intervalChanged = false
}

func (a *App) fetch(ctx context.Context) {
	if a.fetching {
		a.refreshPending = true
		return
	}
	a.loading = true
	a.fetching = true
	a.render()
	previous := a.snapshot
	fullFetch := a.needsFullFetch(time.Now())
	go func() {
		fetchCtx, cancel := context.WithTimeout(ctx, a.fetchTimeout)
		defer cancel()
		snapshot, err := a.fetchSnapshot(fetchCtx, previous, fullFetch)
		a.resultCh <- fetchResult{snapshot: snapshot, err: err, full: fullFetch}
	}()
}

func (a *App) needsFullFetch(now time.Time) bool {
	if len(a.snapshot.Nodes) == 0 || a.lastFullFetch.IsZero() {
		return true
	}
	return now.Sub(a.lastFullFetch) >= fullRefreshInterval
}

func (a *App) fetchSnapshot(ctx context.Context, previous komari.Snapshot, fullFetch bool) (komari.Snapshot, error) {
	if fullFetch {
		return a.client.Snapshot(ctx)
	}

	status, err := a.client.LatestStatus(ctx)
	if err != nil {
		return komari.Snapshot{}, err
	}
	nodes := make(map[string]komari.Node, len(previous.Nodes))
	for _, node := range previous.Nodes {
		nodes[node.UUID] = node
	}
	snapshot := komari.NewSnapshot(a.client.BaseURL(), previous.Public, nodes, status)
	snapshot.Version = previous.Version
	snapshot.RPCVersion = previous.RPCVersion
	snapshot.Me = previous.Me
	snapshot.Methods = previous.Methods
	snapshot.LastErr = previous.LastErr
	snapshot.NodeDetailErr = previous.NodeDetailErr
	return snapshot, nil
}

func (a *App) fetchDetail(ctx context.Context, uuid string, force bool) {
	if uuid == "" || a.client == nil {
		return
	}
	key := detailKey{UUID: uuid, Window: a.window}
	current, ok := a.nodeDetail[key]
	if ok && current.Loading {
		return
	}
	if ok && !force && time.Since(current.FetchedAt) < a.detailCacheTTL {
		return
	}
	current.UUID = uuid
	current.Window = a.window
	current.Loading = true
	current.Err = nil
	a.nodeDetail[key] = current
	a.render()

	go func() {
		detailCtx, cancel := context.WithTimeout(ctx, a.detailTimeout)
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
	node, ok := a.selectedNode()
	if !ok {
		return
	}
	a.fetchDetail(ctx, node.UUID, false)
}

func (a *App) requestRefresh() {
	select {
	case a.refreshCh <- struct{}{}:
	default:
	}
}

func (a *App) requestFullRefresh() {
	a.lastFullFetch = time.Time{}
	a.requestRefresh()
}

func (a *App) requestRender() {
	select {
	case a.renderCh <- struct{}{}:
	default:
	}
}
