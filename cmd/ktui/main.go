package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"ktui/internal/config"
	"ktui/internal/komari"
	"ktui/internal/tui"
	"ktui/internal/update"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	args, configPath := splitConfigArg(os.Args[1:])
	if configPath != "" {
		if err := os.Setenv("KTUI_CONFIG", configPath); err != nil {
			fatal(err)
		}
	}
	if len(args) > 0 && (args[0] == "version" || args[0] == "--version" || args[0] == "-v") {
		printVersion()
		return
	}
	if len(args) > 0 && args[0] == "update" {
		if err := handleUpdate(args[1:]); err != nil {
			fatal(err)
		}
		return
	}
	if len(args) > 0 && args[0] == "config" {
		if err := handleConfig(args[1:]); err != nil {
			fatal(err)
		}
		return
	}
	if len(args) > 0 && args[0] == "export" {
		if err := handleExport(args[1:]); err != nil {
			fatal(err)
		}
		return
	}
	if len(args) > 0 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		if err := handleHelp(args[1:]); err != nil {
			fatal(err)
		}
		return
	}
	if len(args) > 0 && looksLikeCommand(args[0]) {
		usageError(fmt.Errorf("unknown command %q", args[0]))
	}

	cfg, cfgPath, err := loadEffectiveConfig()
	if err != nil {
		fatal(err)
	}

	intervalDefault, err := cfg.IntervalDuration()
	if err != nil {
		fatal(err)
	}
	timeoutDefault, err := cfg.TimeoutDuration()
	if err != nil {
		fatal(err)
	}

	var (
		baseURL        string
		apiKey         string
		interval       time.Duration
		timeout        time.Duration
		realtimePoints int
		chartYAxis     string
		once           bool
		ascii          bool
		noColor        bool
		lineMode       bool
		sheetMode      bool
	)

	flags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flags.StringVar(&baseURL, "url", cfg.URL, "Komari base URL")
	flags.StringVar(&apiKey, "api-key", cfg.APIKey, "Komari API key (sent as Bearer token)")
	flags.DurationVar(&interval, "interval", intervalDefault, "refresh interval")
	flags.DurationVar(&timeout, "timeout", timeoutDefault, "HTTP timeout")
	flags.IntVar(&realtimePoints, "realtime-points", cfg.RealtimePoints, "realtime chart sample limit, 0 auto")
	flags.StringVar(&chartYAxis, "chart-y-axis", cfg.ChartYAxis, "percent chart Y axis mode: absolute or relative")
	flags.BoolVar(&once, "once", false, "fetch once and print a summary without entering the TUI")
	flags.BoolVar(&ascii, "ascii", cfg.ASCII, "use ASCII-only rendering for terminals/fonts with Unicode issues")
	flags.BoolVar(&noColor, "no-color", cfg.NoColor, "disable ANSI color and inverse video")
	flags.BoolVar(&lineMode, "line", false, "show servers as one-by-one line blocks")
	flags.BoolVar(&sheetMode, "sheet", false, "show servers in the sheet/table layout")
	flags.String("config", cfgPath, "config file path")
	flags.Usage = printHelp
	if err := flags.Parse(args); err != nil {
		fatal(err)
	}
	if flags.NArg() > 0 {
		usageError(fmt.Errorf("unexpected argument %q", flags.Arg(0)))
	}
	if realtimePoints < 0 {
		fatal(fmt.Errorf("--realtime-points must be 0 or a positive number"))
	}
	chartYAxis = strings.ToLower(strings.TrimSpace(chartYAxis))
	if chartYAxis != "absolute" && chartYAxis != "relative" {
		fatal(fmt.Errorf("--chart-y-axis must be absolute or relative"))
	}

	mode := tui.ModeSheet
	if cfg.Mode == "line" {
		mode = tui.ModeLine
	}
	if lineMode && sheetMode {
		fatal(fmt.Errorf("--line and --sheet cannot be used together"))
	}
	if lineMode {
		mode = tui.ModeLine
	}
	if sheetMode {
		mode = tui.ModeSheet
	}
	baseURL, apiKey, err = prepareConnectionConfig(cfg, baseURL, apiKey, os.Stdin, os.Stdout)
	if err != nil {
		fatal(err)
	}

	client, err := komari.NewClientWithOptions(baseURL, komari.Options{APIKey: apiKey, Timeout: timeout})
	if err != nil {
		fatal(err)
	}

	if once {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		snapshot, err := client.Snapshot(ctx)
		if err != nil {
			fatal(err)
		}
		printSummary(snapshot)
		return
	}

	app := tui.NewWithOptions(client, tui.Options{
		URL:             baseURL,
		APIKey:          apiKey,
		RefreshInterval: interval,
		FetchTimeout:    timeout,
		DetailTimeout:   timeout,
		RealtimePoints:  realtimePoints,
		ChartYAxisMode:  chartYAxis,
		WarnCPU:         cfg.WarnCPU,
		WarnRAM:         cfg.WarnRAM,
		WarnDisk:        cfg.WarnDisk,
		WarnExpiryDays:  cfg.WarnExpiryDays,
		SaveSettings:    saveTUISettings,
		CheckUpdate:     checkSoftwareUpdate,
		ASCII:           ascii,
		NoColor:         noColor,
		Mode:            mode,
	})
	if err := app.Run(context.Background()); err != nil && err != context.Canceled {
		fatal(err)
	}
}

func loadEffectiveConfig() (config.Config, string, error) {
	cfg, path, err := config.Load()
	if err != nil {
		return cfg, path, err
	}
	cfg = applyEnv(cfg)
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return cfg, path, err
	}
	return cfg, path, nil
}

func applyEnv(cfg config.Config) config.Config {
	if value := os.Getenv("KTUI_URL"); value != "" {
		cfg.URL = value
	}
	if value := os.Getenv("KTUI_API_KEY"); value != "" {
		cfg.APIKey = value
	}
	if value := os.Getenv("KTUI_INTERVAL"); value != "" {
		cfg.Interval = value
	}
	if value := os.Getenv("KTUI_TIMEOUT"); value != "" {
		cfg.Timeout = value
	}
	if value := os.Getenv("KTUI_REALTIME_POINTS"); value != "" {
		points, err := strconv.Atoi(value)
		if err != nil {
			cfg.RealtimePoints = -1
		} else {
			cfg.RealtimePoints = points
		}
	}
	if value := os.Getenv("KTUI_CHART_Y_AXIS"); value != "" {
		cfg.ChartYAxis = strings.ToLower(strings.TrimSpace(value))
	}
	if value := os.Getenv("KTUI_WARN_CPU"); value != "" {
		if parsed, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(value), "%"), 64); err == nil {
			cfg.WarnCPU = parsed
		} else {
			cfg.WarnCPU = -1
		}
	}
	if value := os.Getenv("KTUI_WARN_RAM"); value != "" {
		if parsed, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(value), "%"), 64); err == nil {
			cfg.WarnRAM = parsed
		} else {
			cfg.WarnRAM = -1
		}
	}
	if value := os.Getenv("KTUI_WARN_DISK"); value != "" {
		if parsed, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(value), "%"), 64); err == nil {
			cfg.WarnDisk = parsed
		} else {
			cfg.WarnDisk = -1
		}
	}
	if value := os.Getenv("KTUI_WARN_EXPIRY_DAYS"); value != "" {
		if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			cfg.WarnExpiryDays = parsed
		} else {
			cfg.WarnExpiryDays = -1
		}
	}
	if value := os.Getenv("KTUI_MODE"); value != "" {
		cfg.Mode = value
	}
	if envBool("KTUI_ASCII") {
		cfg.ASCII = true
	}
	if envBool("NO_COLOR") || envBool("KTUI_NO_COLOR") {
		cfg.NoColor = true
	}
	return cfg
}

func saveTUISettings(settings tui.PersistentSettings) error {
	cfg, _, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Interval = settings.Interval
	cfg.Timeout = settings.Timeout
	cfg.Mode = settings.Mode
	cfg.RealtimePoints = settings.RealtimePoints
	cfg.ChartYAxis = settings.ChartYAxisMode
	cfg.ASCII = settings.ASCII
	cfg.NoColor = settings.NoColor
	cfg.WarnCPU = settings.WarnCPU
	cfg.WarnRAM = settings.WarnRAM
	cfg.WarnDisk = settings.WarnDisk
	cfg.WarnExpiryDays = settings.WarnExpiryDays
	_, err = config.Save(cfg)
	return err
}

func prepareConnectionConfig(cfg config.Config, baseURL string, apiKey string, input *os.File, output io.Writer) (string, string, error) {
	if strings.TrimSpace(baseURL) != "" {
		return baseURL, apiKey, nil
	}
	if !isInteractiveTerminal(input) {
		return "", "", fmt.Errorf("Komari URL is not set. Run `ktui config set url https://your-komari.example.com` or pass `--url https://your-komari.example.com`")
	}
	cfg.APIKey = apiKey
	next, err := firstRunSetup(cfg, input, output)
	if err != nil {
		return "", "", err
	}
	return next.URL, next.APIKey, nil
}

func firstRunSetup(cfg config.Config, input io.Reader, output io.Writer) (config.Config, error) {
	reader := bufio.NewReader(input)
	fmt.Fprintln(output, "ktui first run setup")
	fmt.Fprintln(output, "Komari URL is required before opening the TUI.")
	baseURL, err := promptLine(reader, output, "Komari URL: ")
	if err != nil {
		return cfg, err
	}
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return cfg, fmt.Errorf("Komari URL is required")
	}
	client, err := komari.NewClientWithOptions(baseURL, komari.Options{})
	if err != nil {
		return cfg, err
	}
	apiKey, err := promptSecretLine(reader, input, output, "API key (optional): ")
	if err != nil && err != io.EOF {
		return cfg, err
	}
	cfg.URL = client.BaseURL()
	if strings.TrimSpace(apiKey) != "" {
		cfg.APIKey = strings.TrimSpace(apiKey)
	}
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	path, err := config.Save(cfg)
	if err != nil {
		return cfg, err
	}
	fmt.Fprintf(output, "Saved config: %s\n", path)
	return cfg, nil
}

func promptLine(reader *bufio.Reader, output io.Writer, prompt string) (string, error) {
	fmt.Fprint(output, prompt)
	value, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	if err == io.EOF && value == "" {
		return "", io.EOF
	}
	return strings.TrimRight(value, "\r\n"), nil
}

func promptSecretLine(reader *bufio.Reader, input io.Reader, output io.Writer, prompt string) (string, error) {
	file, ok := input.(*os.File)
	if !ok || reader.Buffered() > 0 || !isInteractiveTerminal(file) {
		return promptLine(reader, output, prompt)
	}
	fmt.Fprint(output, prompt)
	value, err := readSecretFromTerminal(file)
	fmt.Fprintln(output)
	return value, err
}

func isInteractiveTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func checkSoftwareUpdate(ctx context.Context) (tui.UpdateCheckResult, error) {
	result, err := update.Check(ctx, update.Options{
		CurrentVersion: version,
		Timeout:        8 * time.Second,
	})
	if err != nil {
		return tui.UpdateCheckResult{}, err
	}
	return tui.UpdateCheckResult{
		CurrentVersion: result.CurrentVersion,
		LatestVersion:  result.LatestVersion,
		AssetName:      result.AssetName,
		Available:      result.Available,
	}, nil
}

func handleConfig(args []string) error {
	if len(args) == 0 {
		printConfigHelp()
		return nil
	}
	switch args[0] {
	case "help":
		printConfigHelp()
		return nil
	case "path":
		path, err := config.Path()
		if err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	case "init":
		fs := flag.NewFlagSet("ktui config init", flag.ExitOnError)
		force := fs.Bool("force", false, "overwrite existing config")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		path, err := config.Path()
		if err != nil {
			return err
		}
		if _, err := os.Stat(path); err == nil && !*force {
			return fmt.Errorf("config already exists: %s (use --force to overwrite)", path)
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		saved, err := config.Save(config.Default())
		if err != nil {
			return err
		}
		fmt.Println(saved)
		return nil
	case "show":
		cfg, path, err := config.Load()
		if err != nil {
			return err
		}
		if cfg.APIKey != "" {
			cfg.APIKey = "********"
		}
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(path)
		fmt.Println(string(data))
		return nil
	case "set":
		if len(args) != 3 {
			return fmt.Errorf("usage: ktui config set <key> <value>")
		}
		cfg, _, err := config.Load()
		if err != nil {
			return err
		}
		cfg, err = config.Set(cfg, args[1], args[2])
		if err != nil {
			return err
		}
		path, err := config.Save(cfg)
		if err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	default:
		return fmt.Errorf("unknown config command %q", args[0])
	}
}

func handleHelp(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: ktui help [config|keys|update|export]")
	}
	if len(args) == 0 {
		printHelp()
		return nil
	}
	switch args[0] {
	case "config":
		printConfigHelp()
	case "keys":
		printKeysHelp()
	case "update":
		printUpdateHelp()
	case "export":
		printExportHelp()
	default:
		return fmt.Errorf("unknown help topic %q", args[0])
	}
	return nil
}

func handleUpdate(args []string) error {
	fs := flag.NewFlagSet("ktui update", flag.ExitOnError)
	checkOnly := fs.Bool("check", false, "check whether an update is available without installing it")
	targetTag := fs.String("tag", "", "install a specific release tag instead of the latest release")
	apiURL := fs.String("api-url", update.DefaultAPIBaseURL, "Gitea repository API URL")
	timeout := fs.Duration("timeout", 60*time.Second, "update HTTP timeout")
	fs.Usage = printUpdateHelp
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected update argument %q", fs.Arg(0))
	}
	return update.Run(context.Background(), update.Options{
		APIBaseURL:     *apiURL,
		CurrentVersion: version,
		TargetVersion:  *targetTag,
		CheckOnly:      *checkOnly,
		Timeout:        *timeout,
		Stdout:         os.Stdout,
	})
}

func printHelp() {
	fmt.Print(helpText)
}

const helpText = `ktui - Komari terminal UI

Usage:
  ktui [flags]
  ktui version
  ktui update [--check] [--tag v0.1.0]
  ktui export <json|csv|markdown> [--output PATH]
  ktui config <path|init|show|set|help>
  ktui help [config|keys|update|export]

Flags:
  --url URL          Komari base URL
  --api-key KEY     Komari API key, sent as a Bearer token
  --interval 5s     refresh interval
  --timeout 10s     HTTP timeout
  --realtime-points N
                   realtime chart sample limit, 0 auto
  --chart-y-axis MODE
                   percent chart Y axis mode: absolute or relative
  --sheet           show the boxed sheet layout
  --line            show one server block after another
  --ascii           use ASCII-only rendering
  --no-color        disable ANSI color
  --once            fetch once and print a summary
  --config PATH     config file path

Examples:
  ktui
  ktui --sheet
  ktui --line --ascii --no-color
  ktui version
  ktui update --check
  ktui export markdown
  ktui export csv --output nodes.csv
  ktui config init
  ktui config set api-key your_api_key
  ktui help keys
`

func printVersion() {
	fmt.Printf("ktui %s\n", version)
	fmt.Printf("commit: %s\n", commit)
	fmt.Printf("built:  %s\n", date)
}

func printUpdateHelp() {
	fmt.Printf(`ktui update - update ktui from Gitea Releases

Usage:
  ktui update [flags]

Flags:
  --check          check for an update without installing it
  --tag TAG        install a specific release tag, for example v0.1.0
  --api-url URL    Gitea repository API URL
  --timeout 60s    HTTP timeout

Default API URL:
  %s

Examples:
  ktui update --check
  ktui update
  ktui update --tag v0.1.0

Private repositories:
  KTUI_UPDATE_TOKEN=your_token ktui update
`, update.DefaultAPIBaseURL)
}

func looksLikeCommand(arg string) bool {
	return arg != "" && !strings.HasPrefix(arg, "-")
}

func printConfigHelp() {
	path, err := config.Path()
	if err != nil {
		path = "<user-config-dir>/ktui/config.json"
	}
	fmt.Printf(`ktui config - persistent settings

Default path:
  %s

Commands:
  ktui config path
  ktui config init [--force]
  ktui config show
  ktui config set <key> <value>
  ktui config help

Keys:
  url       Komari base URL
  api-key   Komari API key
  interval  refresh interval, for example 5s
  timeout   HTTP timeout, for example 10s
  realtime-points
            realtime chart sample limit, 0 auto
	  chart-y-axis
	            percent chart Y axis mode: absolute or relative
	  warn-cpu  CPU warning threshold percent, for example 90
	  warn-ram  RAM warning threshold percent, for example 85
	  warn-disk Disk warning threshold percent, for example 90
	  warn-expiry-days
	            expiry warning window in days
	  mode      sheet or line
  ascii     true or false
  no-color  true or false

Precedence:
  defaults < config file < environment variables < command-line flags
`, path)
}

func printKeysHelp() {
	fmt.Print(`ktui keys

List layer:
	  Up/k, Down/j       select server
	  Mouse wheel        select previous/next server
	  Mouse click        open server detail
	  Footer click       search/sort/filter/settings/mode/refresh/ascii/quit
	  PgUp, PgDn         jump faster
	  /                  edit node search
	  c                  cycle sort: default/status/cpu/ram/traffic/expiry
	  v                  cycle filter: all/offline/expiring/high-load
	  Enter/o            open selected server detail
	  s                  open settings
  m                  switch line/sheet mode
  r                  refresh now
  d                  open or reload selected server detail data
  a                  toggle ASCII compatibility mode
  u                  show update command when an update is available
  q, Ctrl-C          quit

Detail layer:
  Esc, b, q          return to list layer
  Mouse click Back   return to list layer
  f, Enter           focus first chart on chart tabs
  Mouse click chart  focus clicked chart
  h/l, 1-5, Tab      switch detail tabs
  [, ]               switch time window
  Up/k, Down/j       scroll one card
  Mouse wheel        scroll detail cards
  Mouse click        switch tabs or time window
  Footer click       Back/settings/refresh
  s                  open settings
  u                  show update command when an update is available
  PgUp, PgDn         scroll faster

Chart focus:
  Esc, b, q, Enter   return to detail layer
  h/l, PgUp/PgDn     switch focused chart
  [, ]               switch time window

Settings layer:
	  Esc, q, s          return to previous layer
	  Up/k, Down/j       select setting
	  Mouse wheel/click  select setting
	  Footer click       back/adjust/toggle
	  Left/h, Right/l    adjust value
	  Enter              toggle or advance value

Search:
	  Type text          match node name, region, tags, group, IP, OS, UUID
	  Backspace          delete one character
	  Enter              apply search
	  Esc                cancel editing
	`)
}

func envBool(key string) bool {
	switch os.Getenv(key) {
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	default:
		return false
	}
}

func splitConfigArg(args []string) ([]string, string) {
	out := make([]string, 0, len(args))
	configPath := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--config" && i+1 < len(args) {
			configPath = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "--config=") {
			configPath = strings.TrimPrefix(arg, "--config=")
			continue
		}
		out = append(out, arg)
	}
	return out, configPath
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ktui:", err)
	os.Exit(1)
}

func usageError(err error) {
	fmt.Fprintln(os.Stderr, "ktui:", err)
	fmt.Fprintln(os.Stderr)
	fmt.Fprint(os.Stderr, helpText)
	os.Exit(1)
}

func printSummary(snapshot komari.Snapshot) {
	title := snapshot.Public.SiteName
	if title == "" {
		title = "Komari"
	}
	fmt.Printf("%s (%s)\n", title, snapshot.SourceURL)
	if snapshot.Version.Version != "" {
		fmt.Printf("Version: %s  RPC: %s\n", snapshot.Version.Version, snapshot.RPCVersion)
	}
	if snapshot.Me.LoggedIn {
		fmt.Printf("Auth: logged in as %s\n", snapshot.Me.Username)
	} else {
		fmt.Println("Auth: guest")
	}
	if snapshot.NodeDetailErr != nil {
		fmt.Printf("Node detail: unavailable (%v)\n", snapshot.NodeDetailErr)
	}
	fmt.Printf("Online: %d/%d\n", snapshot.Online, snapshot.Total)
	for _, node := range snapshot.Nodes {
		st := snapshot.Status[node.UUID]
		state := "offline"
		if st.Online {
			state = "online"
		}
		fmt.Printf("- %-36s %-7s CPU %5.1f%% RAM %5.1f%% Disk %5.1f%%\n",
			node.Name,
			state,
			st.CPU,
			percentage(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal)),
			percentage(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal)),
		)
		if node.IPv4 != "" || node.IPv6 != "" {
			fmt.Printf("  ip4 %-39s ip6 %s\n", valueOr(node.IPv4, "-"), valueOr(node.IPv6, "-"))
		}
	}
}

func firstNonZero(a, b int64) int64 {
	if a != 0 {
		return a
	}
	return b
}

func percentage(used, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(used) / float64(total) * 100
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
