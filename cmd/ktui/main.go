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

const (
	maxSystemClockSkew = 30 * time.Second
	clockCheckTimeout  = 5 * time.Second
)

func main() {
	if err := execute(); err != nil {
		fatal(err)
	}
}

func handleCommand(args []string) error {
	if len(args) == 0 {
		return handleTUI(args)
	}
	switch args[0] {
	case "status":
		return handleStatus(args[1:])
	case "export":
		return handleExport(args[1:])
	case "config":
		return handleConfig(args[1:])
	case "profile":
		return handleProfile(args[1:])
	case "update":
		if err := handleUpdate(args[1:]); err != nil {
			return err
		}
		return nil
	case "version":
		printVersion()
		return nil
	case "help":
		return handleHelp(args[1:])
	default:
		if !looksLikeCommand(args[0]) {
			return handleTUI(args)
		}
		usageError(fmt.Errorf("unknown command %q", args[0]))
		return nil
	}
}

func handleTUI(args []string) error {
	cfg, cfgPath, err := loadEffectiveConfig()
	if err != nil {
		return err
	}

	intervalDefault, err := cfg.IntervalDuration()
	if err != nil {
		return err
	}
	timeoutDefault, err := cfg.TimeoutDuration()
	if err != nil {
		return err
	}
	realtimeWindowDefault, err := cfg.RealtimeWindowDuration()
	if err != nil {
		return err
	}

	var (
		baseURL        string
		apiKey         string
		interval       time.Duration
		timeout        time.Duration
		realtimeWindow time.Duration
		chartYAxis     string
		modeValue      string
		profileName    string
		ascii          bool
		noColor        bool
	)

	flags := flag.NewFlagSet("ktui", flag.ExitOnError)
	flags.StringVar(&profileName, "profile", cfg.Profile, "profile name")
	flags.StringVar(&baseURL, "url", cfg.URL, "Komari base URL")
	flags.StringVar(&apiKey, "api-key", cfg.APIKey, "Komari API key (sent as Bearer token)")
	flags.DurationVar(&interval, "interval", intervalDefault, "refresh interval")
	flags.DurationVar(&timeout, "timeout", timeoutDefault, "HTTP timeout")
	flags.DurationVar(&realtimeWindow, "realtime-window", realtimeWindowDefault, "realtime chart time window: 1m, 5m, or 10m")
	flags.StringVar(&chartYAxis, "chart-y-axis", cfg.ChartYAxis, "percent chart Y axis mode: absolute or relative")
	flags.StringVar(&modeValue, "mode", cfg.Mode, "view mode: sheet or line")
	flags.BoolVar(&ascii, "ascii", cfg.ASCII, "use ASCII-only rendering for terminals/fonts with Unicode issues")
	flags.BoolVar(&noColor, "no-color", cfg.NoColor, "disable ANSI color and inverse video")
	flags.String("config", cfgPath, "config file path")
	flags.Usage = printHelp
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		usageError(fmt.Errorf("unexpected argument %q", flags.Arg(0)))
	}
	cfg, err = applyProfileFlag(cfg, profileName, flags, &baseURL, &apiKey)
	if err != nil {
		return err
	}
	if !validRealtimeWindow(realtimeWindow) {
		return fmt.Errorf("--realtime-window must be 1m, 5m, or 10m")
	}
	chartYAxis = strings.ToLower(strings.TrimSpace(chartYAxis))
	if chartYAxis != "absolute" && chartYAxis != "relative" {
		return fmt.Errorf("--chart-y-axis must be absolute or relative")
	}

	mode, err := parseModeFlag(modeValue)
	if err != nil {
		return err
	}
	baseURL, apiKey, err = prepareConnectionConfig(cfg, baseURL, apiKey, os.Stdin, os.Stdout)
	if err != nil {
		return err
	}

	client, err := komari.NewClientWithOptions(baseURL, komari.Options{APIKey: apiKey, Timeout: timeout})
	if err != nil {
		return err
	}

	if err := checkSystemClockBeforeTUI(client, timeout); err != nil {
		return err
	}

	app := tui.NewWithOptions(client, tui.Options{
		Profile:           cfg.Profile,
		Profiles:          tuiProfiles(cfg, baseURL, apiKey),
		URL:               baseURL,
		APIKey:            apiKey,
		RefreshInterval:   interval,
		FetchTimeout:      timeout,
		DetailTimeout:     timeout,
		RealtimeWindow:    realtimeWindow,
		ChartYAxisMode:    chartYAxis,
		WarnCPU:           cfg.WarnCPU,
		WarnRAM:           cfg.WarnRAM,
		WarnDisk:          cfg.WarnDisk,
		WarnExpiryDays:    cfg.WarnExpiryDays,
		SaveSettings:      saveTUISettings,
		CheckUpdate:       checkSoftwareUpdate,
		CheckKomariUpdate: checkKomariServerUpdate,
		Version:           version,
		Commit:            commit,
		BuildDate:         date,
		ASCII:             ascii,
		NoColor:           noColor,
		Mode:              mode,
	})
	if err := app.Run(context.Background()); err != nil && err != context.Canceled {
		return err
	}
	return nil
}

func handleStatus(args []string) error {
	cfg, cfgPath, err := loadEffectiveConfig()
	if err != nil {
		return err
	}
	timeoutDefault, err := cfg.TimeoutDuration()
	if err != nil {
		return err
	}

	var (
		baseURL     string
		apiKey      string
		timeout     time.Duration
		profileName string
	)
	fs := flag.NewFlagSet("ktui status", flag.ExitOnError)
	fs.StringVar(&profileName, "profile", cfg.Profile, "profile name")
	fs.StringVar(&baseURL, "url", cfg.URL, "Komari base URL")
	fs.StringVar(&apiKey, "api-key", cfg.APIKey, "Komari API key (sent as Bearer token)")
	fs.DurationVar(&timeout, "timeout", timeoutDefault, "HTTP timeout")
	fs.String("config", cfgPath, "config file path")
	fs.Usage = printStatusHelp
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected status argument %q", fs.Arg(0))
	}
	cfg, err = applyProfileFlag(cfg, profileName, fs, &baseURL, &apiKey)
	if err != nil {
		return err
	}

	baseURL, apiKey, err = prepareConnectionConfig(cfg, baseURL, apiKey, os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	client, err := komari.NewClientWithOptions(baseURL, komari.Options{APIKey: apiKey, Timeout: timeout})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	snapshot, err := client.Snapshot(ctx)
	if err != nil {
		return err
	}
	printSummary(snapshot)
	return nil
}

func parseModeFlag(value string) (tui.Mode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sheet":
		return tui.ModeSheet, nil
	case "line":
		return tui.ModeLine, nil
	default:
		return "", fmt.Errorf("--mode must be sheet or line")
	}
}

func validRealtimeWindow(value time.Duration) bool {
	switch value {
	case time.Minute, 5 * time.Minute, 10 * time.Minute:
		return true
	default:
		return false
	}
}

func applyProfileFlag(cfg config.Config, profileName string, flags *flag.FlagSet, baseURL *string, apiKey *string) (config.Config, error) {
	profileName = strings.TrimSpace(profileName)
	if profileName == "" || profileName == cfg.Profile {
		return cfg, nil
	}
	next, err := config.UseProfile(cfg, profileName)
	if err != nil {
		return cfg, err
	}
	if !flagWasSet(flags, "url") {
		*baseURL = next.URL
	}
	if !flagWasSet(flags, "api-key") {
		*apiKey = next.APIKey
	}
	return next, nil
}

func flagWasSet(flags *flag.FlagSet, name string) bool {
	found := false
	flags.Visit(func(item *flag.Flag) {
		if item.Name == name {
			found = true
		}
	})
	return found
}

type serverTimeSource interface {
	ServerTime(context.Context) (time.Time, error)
}

func checkSystemClockBeforeTUI(source serverTimeSource, timeout time.Duration) error {
	if timeout <= 0 || timeout > clockCheckTimeout {
		timeout = clockCheckTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return checkSystemClock(ctx, source, time.Now)
}

func checkSystemClock(ctx context.Context, source serverTimeSource, now func() time.Time) error {
	serverTime, err := source.ServerTime(ctx)
	if err != nil {
		return nil
	}
	if now == nil {
		now = time.Now
	}
	return validateSystemClock(now(), serverTime, maxSystemClockSkew)
}

func validateSystemClock(localTime, serverTime time.Time, maxSkew time.Duration) error {
	skew := localTime.Sub(serverTime)
	if absDuration(skew) <= maxSkew {
		return nil
	}
	direction := "ahead of"
	if skew < 0 {
		direction = "behind"
	}
	return fmt.Errorf(
		"system clock appears to be out of sync: local time is %s %s Komari server time (local %s, server %s). Please correct your system time and run ktui again",
		absDuration(skew).Round(time.Second),
		direction,
		localTime.Format(time.RFC3339),
		serverTime.Format(time.RFC3339),
	)
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func loadEffectiveConfig() (config.Config, string, error) {
	cfg, path, err := config.Load()
	if err != nil {
		return cfg, path, err
	}
	cfg, err = applyEnv(cfg)
	if err != nil {
		return cfg, path, err
	}
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return cfg, path, err
	}
	return cfg, path, nil
}

func applyEnv(cfg config.Config) (config.Config, error) {
	if value := os.Getenv("KTUI_PROFILE"); value != "" {
		var err error
		cfg, err = config.UseProfile(cfg, value)
		if err != nil {
			return cfg, err
		}
	}
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
	if value := os.Getenv("KTUI_REALTIME_WINDOW"); value != "" {
		cfg.RealtimeWindow = strings.TrimSpace(value)
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
	return cfg, nil
}

func saveTUISettings(settings tui.PersistentSettings) error {
	cfg, _, err := config.Load()
	if err != nil {
		return err
	}
	if strings.TrimSpace(settings.RenameProfileFrom) != "" {
		cfg, err = config.RenameProfile(cfg, settings.RenameProfileFrom, settings.Profile)
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(settings.Profile) != "" {
		cfg, err = config.UseProfile(cfg, settings.Profile)
		if err != nil {
			return err
		}
	}
	cfg.Interval = settings.Interval
	cfg.Timeout = settings.Timeout
	cfg.Mode = settings.Mode
	cfg.RealtimeWindow = settings.RealtimeWindow
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

func tuiProfiles(cfg config.Config, baseURL string, apiKey string) []tui.ConnectionProfile {
	cfg = cfg.WithDefaults()
	names := cfg.ProfileNames()
	out := make([]tui.ConnectionProfile, 0, len(names))
	for _, name := range names {
		profile := cfg.Profiles[name]
		if name == cfg.Profile {
			if strings.TrimSpace(baseURL) != "" {
				profile.URL = baseURL
			}
			profile.APIKey = apiKey
		}
		out = append(out, tui.ConnectionProfile{
			Name:   name,
			URL:    profile.URL,
			APIKey: profile.APIKey,
		})
	}
	return out
}

func prepareConnectionConfig(cfg config.Config, baseURL string, apiKey string, input *os.File, output io.Writer) (string, string, error) {
	if strings.TrimSpace(baseURL) != "" {
		return baseURL, apiKey, nil
	}
	if !isInteractiveTerminal(input) {
		return "", "", fmt.Errorf("Komari URL is not set. Run `ktui profile add %s --url https://your-komari.example.com --use` or pass `--url https://your-komari.example.com`", cfg.Profile)
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
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		apiKey = cfg.APIKey
	}
	cfg, err = config.SetActiveProfileConnection(cfg, client.BaseURL(), apiKey)
	if err != nil {
		return cfg, err
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

func checkKomariServerUpdate(ctx context.Context, currentVersion string) (tui.KomariUpdateCheckResult, error) {
	result, err := komari.CheckServerUpdate(ctx, komari.ServerUpdateOptions{
		CurrentVersion: currentVersion,
		Timeout:        8 * time.Second,
	})
	if err != nil {
		return tui.KomariUpdateCheckResult{}, err
	}
	return tui.KomariUpdateCheckResult{
		CurrentVersion: result.CurrentVersion,
		LatestVersion:  result.LatestVersion,
		ReleaseURL:     result.ReleaseURL,
		ReleaseCount:   result.ReleaseCount,
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
		data, err := json.MarshalIndent(cfg.Redacted(), "", "  ")
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

func handleProfile(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printProfileHelp()
		return nil
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return fmt.Errorf("usage: ktui profile list")
		}
		cfg, _, err := config.Load()
		if err != nil {
			return err
		}
		fmt.Printf("%-6s %-20s %-8s %s\n", "ACTIVE", "NAME", "AUTH", "URL")
		for _, name := range cfg.ProfileNames() {
			profile := cfg.Profiles[name]
			marker := ""
			if name == cfg.Profile {
				marker = "*"
			}
			auth := "none"
			if strings.TrimSpace(profile.APIKey) != "" {
				auth = "api-key"
			}
			fmt.Printf("%-6s %-20s %-8s %s\n", marker, name, auth, valueOr(profile.URL, "-"))
		}
		return nil
	case "current":
		if len(args) != 1 {
			return fmt.Errorf("usage: ktui profile current")
		}
		cfg, _, err := config.Load()
		if err != nil {
			return err
		}
		fmt.Println(cfg.Profile)
		return nil
	case "use":
		if len(args) != 2 {
			return fmt.Errorf("usage: ktui profile use <name>")
		}
		cfg, _, err := config.Load()
		if err != nil {
			return err
		}
		cfg, err = config.UseProfile(cfg, args[1])
		if err != nil {
			return err
		}
		path, err := config.Save(cfg)
		if err != nil {
			return err
		}
		fmt.Printf("active profile: %s\n%s\n", cfg.Profile, path)
		return nil
	case "add":
		if len(args) < 2 {
			return fmt.Errorf("usage: ktui profile add <name> --url URL [--api-key KEY] [--use]")
		}
		name := args[1]
		fs := flag.NewFlagSet("ktui profile add", flag.ExitOnError)
		url := fs.String("url", "", "Komari base URL")
		apiKey := fs.String("api-key", "", "Komari API key")
		useProfile := fs.Bool("use", false, "make this profile active")
		fs.Usage = printProfileHelp
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		if fs.NArg() > 0 {
			return fmt.Errorf("unexpected profile add argument %q", fs.Arg(0))
		}
		cfg, _, err := config.Load()
		if err != nil {
			return err
		}
		client, err := komari.NewClientWithOptions(*url, komari.Options{})
		if err != nil {
			return err
		}
		cfg, err = config.AddProfile(cfg, name, client.BaseURL(), *apiKey)
		if err != nil {
			return err
		}
		if *useProfile {
			cfg, err = config.UseProfile(cfg, name)
			if err != nil {
				return err
			}
		}
		path, err := config.Save(cfg)
		if err != nil {
			return err
		}
		fmt.Printf("saved profile: %s\n%s\n", name, path)
		return nil
	case "remove":
		if len(args) != 2 {
			return fmt.Errorf("usage: ktui profile remove <name>")
		}
		cfg, _, err := config.Load()
		if err != nil {
			return err
		}
		cfg, err = config.RemoveProfile(cfg, args[1])
		if err != nil {
			return err
		}
		path, err := config.Save(cfg)
		if err != nil {
			return err
		}
		fmt.Printf("removed profile: %s\nactive profile: %s\n%s\n", args[1], cfg.Profile, path)
		return nil
	case "rename":
		if len(args) != 3 {
			return fmt.Errorf("usage: ktui profile rename <old> <new>")
		}
		newName, err := config.NormalizeProfileName(args[2])
		if err != nil {
			return err
		}
		cfg, _, err := config.Load()
		if err != nil {
			return err
		}
		cfg, err = config.RenameProfile(cfg, args[1], args[2])
		if err != nil {
			return err
		}
		path, err := config.Save(cfg)
		if err != nil {
			return err
		}
		fmt.Printf("renamed profile: %s -> %s\nactive profile: %s\n%s\n", args[1], newName, cfg.Profile, path)
		return nil
	default:
		return fmt.Errorf("unknown profile command %q", args[0])
	}
}

func handleHelp(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: ktui help [status|config|profile|keys|update|export|completion]")
	}
	if len(args) == 0 {
		printHelp()
		return nil
	}
	switch args[0] {
	case "status":
		printStatusHelp()
	case "config":
		printConfigHelp()
	case "profile":
		printProfileHelp()
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
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printUpdateHelp()
		return nil
	}
	switch args[0] {
	case "check":
		return runUpdateCommand("check", args[1:], true)
	case "install":
		return runUpdateCommand("install", args[1:], false)
	default:
		return fmt.Errorf("unknown update command %q", args[0])
	}
}

func runUpdateCommand(name string, args []string, checkOnly bool) error {
	fs := flag.NewFlagSet("ktui update "+name, flag.ExitOnError)
	targetTag := ""
	apiURL := fs.String("api-url", update.DefaultAPIBaseURL, "Gitea repository API URL")
	timeout := fs.Duration("timeout", 60*time.Second, "update HTTP timeout")
	if !checkOnly {
		fs.StringVar(&targetTag, "tag", "", "install a specific release tag instead of the latest release")
	}
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
		TargetVersion:  targetTag,
		CheckOnly:      checkOnly,
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
  ktui status [flags]
  ktui export <markdown|csv|json> [flags]
  ktui config <init|path|show|set|help>
  ktui profile <list|current|use|add|rename|remove>
  ktui update <check|install>
  ktui completion <bash|zsh|fish|powershell>
  ktui version
  ktui help [status|config|profile|keys|update|export|completion]

Connection flags:
  --profile NAME    profile name
  --url URL          Komari base URL
  --api-key KEY     Komari API key, sent as a Bearer token
  --timeout 10s     HTTP timeout
  --config PATH     config file path

TUI flags:
  --interval 5s     refresh interval
  --mode MODE       view mode: sheet or line
  --realtime-window DURATION
                   realtime chart time window: 1m, 5m, or 10m
  --chart-y-axis MODE
                   percent chart Y axis mode: absolute or relative
  --ascii           use ASCII-only rendering
  --no-color        disable ANSI color

Examples:
  ktui
  ktui --mode sheet
  ktui --mode line --ascii --no-color
  ktui status
  ktui version
  ktui update check
  ktui update install
  ktui completion bash
  ktui export markdown
  ktui export csv --output nodes.csv
  ktui config init
  ktui config set api-key your_api_key
  ktui profile add prod --url https://komari.example.com --use
  ktui --profile prod
  ktui help keys
`

func printVersion() {
	fmt.Printf("ktui %s\n", version)
	fmt.Printf("commit: %s\n", commit)
	fmt.Printf("built:  %s\n", date)
}

func printStatusHelp() {
	fmt.Print(`ktui status - fetch once and print a node summary

Usage:
  ktui status [flags]

Flags:
  --profile NAME    profile name
  --url URL          Komari base URL
  --api-key KEY     Komari API key, sent as a Bearer token
  --timeout 10s     HTTP timeout
  --config PATH     config file path

Examples:
  ktui status
  ktui status --url https://komari.example.com
`)
}

func printUpdateHelp() {
	fmt.Printf(`ktui update - update ktui from Gitea Releases

Usage:
  ktui update check [flags]
  ktui update install [flags]

Flags:
  --api-url URL    Gitea repository API URL
  --timeout 60s    HTTP timeout

Install flags:
  --tag TAG        install a specific release tag, for example v0.1.0

Default API URL:
  %s

Examples:
  ktui update check
  ktui update install
  ktui update install --tag v0.1.0

Private repositories:
  KTUI_UPDATE_TOKEN=your_token ktui update install
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
  profile   active profile name
  url       Komari base URL
  api-key   Komari API key
  interval  refresh interval, for example 5s
  timeout   HTTP timeout, for example 10s
  realtime-window
            realtime chart time window: 1m, 5m, or 10m
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

func printProfileHelp() {
	fmt.Print(`ktui profile - manage Komari connection profiles

Usage:
  ktui profile list
  ktui profile current
  ktui profile use <name>
  ktui profile add <name> --url URL [--api-key KEY] [--use]
  ktui profile rename <old> <new>
  ktui profile remove <name>

Examples:
  ktui profile add prod --url https://komari.example.com --api-key your_api_key --use
  ktui profile add lab --url https://lab.example.com
  ktui profile list
  ktui profile use prod
  ktui profile rename lab staging
  ktui --profile lab
  KTUI_PROFILE=prod ktui status
`)
}

func printKeysHelp() {
	fmt.Print(keysHelpText())
}

func keysHelpText() string {
	return `ktui keys

List layer:
  Up/k, Down/j       select server
  Mouse wheel        select previous/next server
  Mouse click        open server detail
  Footer click       detail/search/sort/filter/settings/mode/refresh/ascii/quit
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
  ?                  open about
  u                  show available update details
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
  Footer click       back/tabs/window/scroll/settings/refresh
  s                  open settings
  ?                  open about
  u                  show available update details
  PgUp, PgDn         scroll faster

Chart focus:
  Esc, b, q, Enter   return to detail layer
  h/l, PgUp/PgDn     switch focused chart
  [, ]               switch time window
  Footer click       back/previous/next/window/refresh

Settings layer:
  Esc, q, s          return to previous layer
  Up/k, Down/j       select setting
  Mouse wheel/click  select setting
  Footer click       back/adjust/toggle
  Left/h, Right/l    adjust value
  Enter              toggle or advance value
  ?                  open about
  profile            switch active profile when multiple profiles exist
  rename_profile     Enter edit, Enter save, Esc cancel
  site/url/api_key   shown as read-only

About:
  Esc, q, ?          return to previous layer
  Up/k, Down/j       scroll
  PgUp, PgDn         scroll faster
  r                  refresh now
  u                  show available update details

Search:
  Type text          match node name, region, tags, group, IP, OS, UUID
  Backspace          delete one character
  Enter              apply search
  Esc                cancel editing
`
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
