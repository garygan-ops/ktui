package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"ktui/internal/config"
	"ktui/internal/komari"
)

const (
	exportFormatJSON     = "json"
	exportFormatCSV      = "csv"
	exportFormatMarkdown = "markdown"
)

type exportDocument struct {
	GeneratedAt string            `json:"generated_at"`
	FetchedAt   string            `json:"fetched_at"`
	SourceURL   string            `json:"source_url"`
	SiteName    string            `json:"site_name"`
	Version     string            `json:"version,omitempty"`
	RPCVersion  string            `json:"rpc_version,omitempty"`
	Auth        string            `json:"auth"`
	Summary     exportSummary     `json:"summary"`
	Thresholds  exportThresholds  `json:"thresholds"`
	Messages    []string          `json:"messages,omitempty"`
	Nodes       []exportNodeState `json:"nodes"`
}

type exportSummary struct {
	Online         int      `json:"online"`
	Total          int      `json:"total"`
	Regions        []string `json:"regions"`
	TotalUpBytes   int64    `json:"total_up_bytes"`
	TotalDownBytes int64    `json:"total_down_bytes"`
	SpeedUpBps     int64    `json:"speed_up_bps"`
	SpeedDownBps   int64    `json:"speed_down_bps"`
}

type exportThresholds struct {
	WarnCPU        float64 `json:"warn_cpu"`
	WarnRAM        float64 `json:"warn_ram"`
	WarnDisk       float64 `json:"warn_disk"`
	WarnExpiryDays int     `json:"warn_expiry_days"`
}

type exportNodeState struct {
	Name              string   `json:"name"`
	UUID              string   `json:"uuid"`
	Status            string   `json:"status"`
	Online            bool     `json:"online"`
	Region            string   `json:"region,omitempty"`
	Group             string   `json:"group,omitempty"`
	Tags              string   `json:"tags,omitempty"`
	IPv4              string   `json:"ipv4,omitempty"`
	IPv6              string   `json:"ipv6,omitempty"`
	OS                string   `json:"os,omitempty"`
	Arch              string   `json:"arch,omitempty"`
	CPUPercent        float64  `json:"cpu_percent"`
	RAMPercent        float64  `json:"ram_percent"`
	DiskPercent       float64  `json:"disk_percent"`
	RAMUsedBytes      int64    `json:"ram_used_bytes"`
	RAMTotalBytes     int64    `json:"ram_total_bytes"`
	DiskUsedBytes     int64    `json:"disk_used_bytes"`
	DiskTotalBytes    int64    `json:"disk_total_bytes"`
	NetInBps          int64    `json:"net_in_bps"`
	NetOutBps         int64    `json:"net_out_bps"`
	NetTotalUpBytes   int64    `json:"net_total_up_bytes"`
	NetTotalDownBytes int64    `json:"net_total_down_bytes"`
	TrafficTotal      string   `json:"traffic_total"`
	TrafficLimitBytes int64    `json:"traffic_limit_bytes,omitempty"`
	TrafficLimitType  string   `json:"traffic_limit_type,omitempty"`
	TrafficLimit      string   `json:"traffic_limit,omitempty"`
	TrafficPercent    float64  `json:"traffic_percent,omitempty"`
	UptimeSeconds     int64    `json:"uptime_seconds"`
	Uptime            string   `json:"uptime"`
	ExpiresAt         string   `json:"expires_at,omitempty"`
	Expiry            string   `json:"expiry"`
	AlertLevel        string   `json:"alert_level"`
	AlertReasons      []string `json:"alert_reasons,omitempty"`
	Message           string   `json:"message,omitempty"`
}

type exportAlert struct {
	Level   string
	Reasons []string
}

func handleExport(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printExportHelp()
		return nil
	}
	format := strings.ToLower(strings.TrimSpace(args[0]))
	if !validExportFormat(format) {
		return fmt.Errorf("unknown export format %q: use json, csv, or markdown", args[0])
	}

	cfg, cfgPath, err := loadEffectiveConfig()
	if err != nil {
		return err
	}
	timeoutDefault, err := cfg.TimeoutDuration()
	if err != nil {
		return err
	}

	var (
		baseURL string
		apiKey  string
		timeout time.Duration
		output  string
	)
	fs := flag.NewFlagSet("ktui export "+format, flag.ExitOnError)
	fs.StringVar(&baseURL, "url", cfg.URL, "Komari base URL")
	fs.StringVar(&apiKey, "api-key", cfg.APIKey, "Komari API key (sent as Bearer token)")
	fs.DurationVar(&timeout, "timeout", timeoutDefault, "HTTP timeout")
	fs.StringVar(&output, "output", "", "write export to a file instead of stdout")
	fs.StringVar(&output, "o", "", "write export to a file instead of stdout")
	fs.String("config", cfgPath, "config file path")
	fs.Usage = printExportHelp
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected export argument %q", fs.Arg(0))
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
	doc := buildExportDocument(snapshot, cfg, time.Now())

	writer := io.Writer(os.Stdout)
	var file *os.File
	if strings.TrimSpace(output) != "" {
		file, err = os.Create(output)
		if err != nil {
			return err
		}
		defer file.Close()
		writer = file
	}
	if err := writeExport(format, writer, doc); err != nil {
		return err
	}
	return nil
}

func validExportFormat(format string) bool {
	switch format {
	case exportFormatJSON, exportFormatCSV, exportFormatMarkdown:
		return true
	default:
		return false
	}
}

func buildExportDocument(snapshot komari.Snapshot, cfg config.Config, now time.Time) exportDocument {
	siteName := snapshot.Public.SiteName
	if siteName == "" {
		siteName = "Komari"
	}
	auth := "guest"
	if snapshot.Me.LoggedIn {
		auth = valueOr(snapshot.Me.Username, "api-key")
	}
	messages := []string{}
	if snapshot.LastErr != nil {
		messages = append(messages, "public_info: "+snapshot.LastErr.Error())
	}
	if snapshot.NodeDetailErr != nil {
		messages = append(messages, "node_detail: "+snapshot.NodeDetailErr.Error())
	}

	doc := exportDocument{
		GeneratedAt: exportTime(now),
		FetchedAt:   exportTime(snapshot.FetchedAt),
		SourceURL:   snapshot.SourceURL,
		SiteName:    siteName,
		Version:     snapshot.Version.Version,
		RPCVersion:  snapshot.RPCVersion,
		Auth:        auth,
		Summary: exportSummary{
			Online:         snapshot.Online,
			Total:          snapshot.Total,
			Regions:        append([]string(nil), snapshot.RegionList...),
			TotalUpBytes:   snapshot.TotalUp,
			TotalDownBytes: snapshot.TotalDown,
			SpeedUpBps:     snapshot.SpeedUp,
			SpeedDownBps:   snapshot.SpeedDown,
		},
		Thresholds: exportThresholds{
			WarnCPU:        cfg.WarnCPU,
			WarnRAM:        cfg.WarnRAM,
			WarnDisk:       cfg.WarnDisk,
			WarnExpiryDays: cfg.WarnExpiryDays,
		},
		Messages: messages,
		Nodes:    make([]exportNodeState, 0, len(snapshot.Nodes)),
	}

	for _, node := range snapshot.Nodes {
		st := snapshot.Status[node.UUID]
		alert := exportAlertForNode(node, st, cfg, now)
		ramTotal := firstNonZero(st.RAMTotal, node.MemTotal)
		diskTotal := firstNonZero(st.DiskTotal, node.DiskTotal)
		trafficLimitType := ""
		trafficLimitText := ""
		if node.TrafficLimit > 0 {
			trafficLimitType = exportTrafficLimitType(node.TrafficLimitType)
			trafficLimitText = exportTrafficLimitText(node.TrafficLimit, node.TrafficLimitType)
		}
		status := "offline"
		if st.Online {
			status = "online"
		}
		expiresAt := ""
		if node.ExpiredAt.Valid {
			expiresAt = exportTime(node.ExpiredAt.Time)
		}
		doc.Nodes = append(doc.Nodes, exportNodeState{
			Name:              node.Name,
			UUID:              node.UUID,
			Status:            status,
			Online:            st.Online,
			Region:            node.Region,
			Group:             node.Group,
			Tags:              node.Tags,
			IPv4:              node.IPv4,
			IPv6:              node.IPv6,
			OS:                node.OS,
			Arch:              node.Arch,
			CPUPercent:        exportRound1(st.CPU),
			RAMPercent:        exportRound1(percentage(st.RAM, ramTotal)),
			DiskPercent:       exportRound1(percentage(st.Disk, diskTotal)),
			RAMUsedBytes:      st.RAM,
			RAMTotalBytes:     ramTotal,
			DiskUsedBytes:     st.Disk,
			DiskTotalBytes:    diskTotal,
			NetInBps:          st.NetIn,
			NetOutBps:         st.NetOut,
			NetTotalUpBytes:   st.NetTotalUp,
			NetTotalDownBytes: st.NetTotalDown,
			TrafficTotal:      fmt.Sprintf("%s %s %s %s", "up", exportTrafficBytes(st.NetTotalUp), "down", exportTrafficBytes(st.NetTotalDown)),
			TrafficLimitBytes: node.TrafficLimit,
			TrafficLimitType:  trafficLimitType,
			TrafficLimit:      trafficLimitText,
			TrafficPercent:    exportRound1(exportTrafficPercent(st.NetTotalUp, st.NetTotalDown, node.TrafficLimit, node.TrafficLimitType)),
			UptimeSeconds:     st.Uptime,
			Uptime:            exportDuration(st.Uptime),
			ExpiresAt:         expiresAt,
			Expiry:            exportExpiryText(node, now),
			AlertLevel:        alert.Level,
			AlertReasons:      alert.Reasons,
			Message:           st.Message,
		})
	}
	return doc
}

func writeExport(format string, writer io.Writer, doc exportDocument) error {
	switch format {
	case exportFormatJSON:
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(doc)
	case exportFormatCSV:
		return writeExportCSV(writer, doc)
	case exportFormatMarkdown:
		_, err := fmt.Fprint(writer, exportMarkdown(doc))
		return err
	default:
		return fmt.Errorf("unknown export format %q", format)
	}
}

func writeExportCSV(writer io.Writer, doc exportDocument) error {
	cw := csv.NewWriter(writer)
	if err := cw.Write([]string{
		"fetched_at",
		"site",
		"name",
		"uuid",
		"status",
		"region",
		"group",
		"tags",
		"ipv4",
		"ipv6",
		"os",
		"arch",
		"cpu_percent",
		"ram_percent",
		"disk_percent",
		"net_in_bps",
		"net_out_bps",
		"net_total_up_bytes",
		"net_total_down_bytes",
		"traffic_percent",
		"traffic_limit",
		"traffic_limit_bytes",
		"traffic_limit_type",
		"uptime_seconds",
		"expires_at",
		"expiry",
		"alert_level",
		"alert_reasons",
	}); err != nil {
		return err
	}
	for _, node := range doc.Nodes {
		if err := cw.Write([]string{
			doc.FetchedAt,
			doc.SiteName,
			node.Name,
			node.UUID,
			node.Status,
			node.Region,
			node.Group,
			node.Tags,
			node.IPv4,
			node.IPv6,
			node.OS,
			node.Arch,
			fmt.Sprintf("%.1f", node.CPUPercent),
			fmt.Sprintf("%.1f", node.RAMPercent),
			fmt.Sprintf("%.1f", node.DiskPercent),
			fmt.Sprintf("%d", node.NetInBps),
			fmt.Sprintf("%d", node.NetOutBps),
			fmt.Sprintf("%d", node.NetTotalUpBytes),
			fmt.Sprintf("%d", node.NetTotalDownBytes),
			exportPercentCell(node.TrafficLimitBytes, node.TrafficPercent),
			node.TrafficLimit,
			exportInt64Cell(node.TrafficLimitBytes),
			exportLimitTypeCell(node.TrafficLimitBytes, node.TrafficLimitType),
			fmt.Sprintf("%d", node.UptimeSeconds),
			node.ExpiresAt,
			node.Expiry,
			node.AlertLevel,
			strings.Join(node.AlertReasons, " "),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func exportMarkdown(doc exportDocument) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# ktui node export\n\n")
	fmt.Fprintf(&b, "- Site: %s\n", markdownCell(doc.SiteName))
	fmt.Fprintf(&b, "- Source: %s\n", markdownCell(valueOr(doc.SourceURL, "-")))
	fmt.Fprintf(&b, "- Fetched: %s\n", markdownCell(valueOr(doc.FetchedAt, "-")))
	fmt.Fprintf(&b, "- Online: %d/%d\n", doc.Summary.Online, doc.Summary.Total)
	fmt.Fprintf(&b, "- Traffic: up %s / down %s\n", exportBytesIEC(doc.Summary.TotalUpBytes), exportBytesIEC(doc.Summary.TotalDownBytes))
	if len(doc.Messages) > 0 {
		fmt.Fprintf(&b, "- Messages: %s\n", markdownCell(strings.Join(doc.Messages, "; ")))
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| Node | Status | Region | CPU | RAM | Disk | Net | Traffic | Limit | Expiry | Alert |")
	fmt.Fprintln(&b, "| --- | --- | --- | ---: | ---: | ---: | --- | --- | --- | --- | --- |")
	for _, node := range doc.Nodes {
		alert := node.AlertLevel
		if len(node.AlertReasons) > 0 {
			alert += " (" + strings.Join(node.AlertReasons, " ") + ")"
		}
		net := fmt.Sprintf("in %s / out %s", exportSpeedIEC(node.NetInBps), exportSpeedIEC(node.NetOutBps))
		traffic := fmt.Sprintf("up %s / down %s", exportTrafficBytes(node.NetTotalUpBytes), exportTrafficBytes(node.NetTotalDownBytes))
		limit := "-"
		if node.TrafficLimitBytes > 0 {
			limit = fmt.Sprintf("%.1f%% %s", node.TrafficPercent, node.TrafficLimit)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %.1f%% | %.1f%% | %.1f%% | %s | %s | %s | %s | %s |\n",
			markdownCell(node.Name),
			markdownCell(node.Status),
			markdownCell(valueOr(node.Region, "-")),
			node.CPUPercent,
			node.RAMPercent,
			node.DiskPercent,
			markdownCell(net),
			markdownCell(traffic),
			markdownCell(limit),
			markdownCell(node.Expiry),
			markdownCell(alert),
		)
	}
	return b.String()
}

func exportAlertForNode(node komari.Node, st komari.Status, cfg config.Config, now time.Time) exportAlert {
	reasons := []string{}
	critical := false
	warning := false
	if !st.Online {
		critical = true
		reasons = append(reasons, "offline")
	}
	if st.CPU >= cfg.WarnCPU {
		critical = true
		reasons = append(reasons, "cpu")
	}
	if percentage(st.RAM, firstNonZero(st.RAMTotal, node.MemTotal)) >= cfg.WarnRAM {
		critical = true
		reasons = append(reasons, "ram")
	}
	if percentage(st.Disk, firstNonZero(st.DiskTotal, node.DiskTotal)) >= cfg.WarnDisk {
		critical = true
		reasons = append(reasons, "disk")
	}
	if node.ExpiredAt.Valid && node.Price >= 0 {
		until := node.ExpiredAt.Time.Sub(now)
		if until < 0 {
			critical = true
			reasons = append(reasons, "expired")
		} else if until <= time.Duration(cfg.WarnExpiryDays)*24*time.Hour {
			warning = true
			reasons = append(reasons, "expires")
		}
	}
	if node.TrafficLimit > 0 {
		trafficPct := exportTrafficPercent(st.NetTotalUp, st.NetTotalDown, node.TrafficLimit, node.TrafficLimitType)
		if trafficPct >= 100 {
			critical = true
			reasons = append(reasons, "traffic")
		} else if trafficPct >= 90 {
			warning = true
			reasons = append(reasons, "traffic")
		}
	}
	switch {
	case critical:
		return exportAlert{Level: "critical", Reasons: reasons}
	case warning:
		return exportAlert{Level: "warning", Reasons: reasons}
	default:
		return exportAlert{Level: "ok"}
	}
}

func exportPercentCell(limit int64, value float64) string {
	if limit <= 0 {
		return ""
	}
	return fmt.Sprintf("%.1f", value)
}

func exportInt64Cell(value int64) string {
	if value <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", value)
}

func exportLimitTypeCell(limit int64, limitType string) string {
	if limit <= 0 {
		return ""
	}
	return exportTrafficLimitType(limitType)
}

func exportTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func exportExpiryText(node komari.Node, now time.Time) string {
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

func exportDuration(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	days := seconds / 86400
	seconds %= 86400
	hours := seconds / 3600
	seconds %= 3600
	minutes := seconds / 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case minutes > 0:
		return fmt.Sprintf("%dm", minutes)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}

func exportTrafficPercent(up, down, limit int64, limitType string) float64 {
	if limit <= 0 {
		return 0
	}
	var used int64
	switch strings.ToLower(limitType) {
	case "max":
		if up > down {
			used = up
		} else {
			used = down
		}
	case "min":
		if up < down {
			used = up
		} else {
			used = down
		}
	case "up":
		used = up
	case "down":
		used = down
	default:
		used = up + down
	}
	return float64(used) / float64(limit) * 100
}

func exportTrafficLimitType(limitType string) string {
	switch strings.ToLower(strings.TrimSpace(limitType)) {
	case "up":
		return "Up"
	case "down":
		return "Down"
	case "max":
		return "Max"
	case "min":
		return "Min"
	default:
		return "Sum"
	}
}

func exportTrafficLimitText(limit int64, limitType string) string {
	if limit <= 0 {
		return ""
	}
	return fmt.Sprintf("%s(%s)", exportTrafficLimitType(limitType), exportTrafficBytes(limit))
}

func exportBytesIEC(n int64) string {
	negative := n < 0
	if negative {
		n = -n
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	value := float64(n)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	var out string
	switch {
	case unit == 0:
		out = fmt.Sprintf("%d %s", n, units[unit])
	case value >= 100:
		out = fmt.Sprintf("%.0f %s", value, units[unit])
	case value >= 10:
		out = fmt.Sprintf("%.1f %s", value, units[unit])
	default:
		out = fmt.Sprintf("%.2f %s", value, units[unit])
	}
	if negative {
		return "-" + out
	}
	return out
}

func exportSpeedIEC(n int64) string {
	return exportBytesIEC(n) + "/s"
}

func exportTrafficBytes(n int64) string {
	negative := n < 0
	if negative {
		n = -n
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	value := float64(n)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	var out string
	if unit == 0 {
		out = fmt.Sprintf("%d %s", n, units[unit])
	} else {
		out = fmt.Sprintf("%.2f %s", value, units[unit])
	}
	if negative {
		return "-" + out
	}
	return out
}

func exportRound1(value float64) float64 {
	return math.Round(value*10) / 10
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", `\|`)
	return value
}

func printExportHelp() {
	fmt.Print(`ktui export - export current Komari node status

Usage:
  ktui export <markdown|csv|json> [flags]

Flags:
  --url URL          Komari base URL
  --api-key KEY     Komari API key, sent as a Bearer token
  --timeout 10s     HTTP timeout
  -o, --output PATH write export to a file instead of stdout
  --config PATH     config file path

Examples:
  ktui export markdown -o report.md
  ktui export csv --output nodes.csv
  ktui export json
`)
}
