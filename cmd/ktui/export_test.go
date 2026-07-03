package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"ktui/internal/config"
	"ktui/internal/komari"
)

func TestBuildExportDocumentIncludesAlertsAndThresholds(t *testing.T) {
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	doc := buildExportDocument(testExportSnapshot(now), testExportConfig(), now)

	if doc.SiteName != "Demo Komari" || doc.Summary.Online != 1 || doc.Summary.Total != 2 {
		t.Fatalf("doc summary = %+v site=%q", doc.Summary, doc.SiteName)
	}
	if doc.Thresholds.WarnCPU != 90 || doc.Thresholds.WarnExpiryDays != 7 {
		t.Fatalf("thresholds = %+v", doc.Thresholds)
	}
	if len(doc.Nodes) != 2 {
		t.Fatalf("nodes len = %d, want 2", len(doc.Nodes))
	}
	alpha := doc.Nodes[0]
	if alpha.AlertLevel != "critical" || !hasExportReason(alpha.AlertReasons, "cpu") || !hasExportReason(alpha.AlertReasons, "expires") || !hasExportReason(alpha.AlertReasons, "traffic") {
		t.Fatalf("alpha alert = %s %#v, want critical cpu expires traffic", alpha.AlertLevel, alpha.AlertReasons)
	}
	if alpha.Expiry != "3d" {
		t.Fatalf("alpha expiry = %q, want 3d", alpha.Expiry)
	}
	if alpha.TrafficPercent != 92.5 || alpha.TrafficLimit != "Sum(500.00 GB)" {
		t.Fatalf("alpha traffic = %.1f %q, want 92.5 Sum(500.00 GB)", alpha.TrafficPercent, alpha.TrafficLimit)
	}
	beta := doc.Nodes[1]
	if beta.Status != "offline" || beta.AlertLevel != "critical" || !hasExportReason(beta.AlertReasons, "offline") {
		t.Fatalf("beta = %+v, want offline critical", beta)
	}
}

func TestWriteExportJSON(t *testing.T) {
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	doc := buildExportDocument(testExportSnapshot(now), testExportConfig(), now)
	var out bytes.Buffer

	if err := writeExport(exportFormatJSON, &out, doc); err != nil {
		t.Fatal(err)
	}
	var got exportDocument
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if got.Nodes[0].Name != "alpha|edge" || got.Nodes[0].CPUPercent != 92.4 || got.Nodes[0].TrafficPercent != 92.5 {
		t.Fatalf("json node = %+v", got.Nodes[0])
	}
}

func TestWriteExportCSV(t *testing.T) {
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	doc := buildExportDocument(testExportSnapshot(now), testExportConfig(), now)
	var out bytes.Buffer

	if err := writeExport(exportFormatCSV, &out, doc); err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(strings.NewReader(out.String())).ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v\n%s", err, out.String())
	}
	if len(records) != 3 {
		t.Fatalf("records len = %d, want header + 2 rows", len(records))
	}
	if records[0][0] != "fetched_at" || records[0][2] != "name" {
		t.Fatalf("header = %#v", records[0])
	}
	if records[1][2] != "alpha|edge" || records[1][12] != "92.4" || records[1][19] != "92.5" || records[1][20] != "Sum(500.00 GB)" || !strings.Contains(records[1][27], "traffic") {
		t.Fatalf("first row = %#v", records[1])
	}
}

func TestWriteExportMarkdown(t *testing.T) {
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	doc := buildExportDocument(testExportSnapshot(now), testExportConfig(), now)
	var out bytes.Buffer

	if err := writeExport(exportFormatMarkdown, &out, doc); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{
		"# ktui node export",
		"- Online: 1/2",
		"| Node | Status | Region | CPU | RAM | Disk | Net | Traffic | Limit | Expiry | Alert |",
		"alpha\\|edge",
		"up 229.59 GB / down 233.03 GB",
		"92.5% Sum(500.00 GB)",
		"critical (cpu expires traffic)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("markdown missing %q:\n%s", want, text)
		}
	}
}

func testExportConfig() config.Config {
	return config.Config{
		WarnCPU:        90,
		WarnRAM:        85,
		WarnDisk:       90,
		WarnExpiryDays: 7,
	}.WithDefaults()
}

func testExportSnapshot(now time.Time) komari.Snapshot {
	alpha := komari.Node{
		UUID:         "n1",
		Name:         "alpha|edge",
		Region:       "tokyo",
		Group:        "prod",
		Tags:         "web",
		IPv4:         "192.0.2.1",
		OS:           "linux",
		Arch:         "amd64",
		MemTotal:     1000,
		DiskTotal:    2000,
		TrafficLimit: 500 * 1024 * 1024 * 1024,
		Price:        5,
		ExpiredAt:    komari.NullTime{Time: now.Add(72 * time.Hour), Valid: true},
	}
	beta := komari.Node{
		UUID:      "n2",
		Name:      "beta",
		Region:    "london",
		MemTotal:  1000,
		DiskTotal: 2000,
	}
	return komari.Snapshot{
		Nodes:      []komari.Node{alpha, beta},
		FetchedAt:  now.Add(-time.Minute),
		SourceURL:  "https://komari.example.com",
		Public:     komari.PublicInfo{SiteName: "Demo Komari"},
		Version:    komari.VersionInfo{Version: "1.0.0"},
		RPCVersion: "2.0",
		Online:     1,
		Total:      2,
		TotalUp:    246_520_385_372,
		TotalDown:  250_214_057_246,
		SpeedUp:    1024,
		SpeedDown:  2048,
		RegionList: []string{"tokyo"},
		Status: map[string]komari.Status{
			"n1": {
				Online:       true,
				CPU:          92.44,
				RAM:          400,
				RAMTotal:     1000,
				Disk:         500,
				DiskTotal:    2000,
				NetIn:        2048,
				NetOut:       1024,
				NetTotalUp:   246_520_385_372,
				NetTotalDown: 250_214_057_246,
				Uptime:       90061,
			},
			"n2": {
				Online:    false,
				CPU:       10,
				RAM:       100,
				RAMTotal:  1000,
				Disk:      200,
				DiskTotal: 2000,
			},
		},
	}
}

func hasExportReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
