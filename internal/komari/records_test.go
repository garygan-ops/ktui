package komari

import (
	"encoding/json"
	"testing"
)

func TestLoadRecordsRespUnmarshalArray(t *testing.T) {
	raw := []byte(`{
		"count": 1,
		"records": [
			{"client":"node-1","time":"2026-06-29T22:00:00+08:00","cpu":12.5,"ram":10,"ram_total":100}
		],
		"from": "2026-06-29T21:00:00+08:00",
		"to": "2026-06-29T22:00:00+08:00"
	}`)

	var resp LoadRecordsResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal array records: %v", err)
	}
	if len(resp.Records) != 1 {
		t.Fatalf("records len = %d, want 1", len(resp.Records))
	}
	if resp.Records[0].Client != "node-1" || resp.Records[0].CPU != 12.5 {
		t.Fatalf("unexpected record: %+v", resp.Records[0])
	}
}

func TestLoadRecordsRespUnmarshalMap(t *testing.T) {
	raw := []byte(`{
		"count": 1,
		"records": {
			"node-1": [
				{"client":"node-1","time":"2026-06-29T22:00:00+08:00","cpu":12.5,"ram":10,"ram_total":100}
			]
		},
		"from": "2026-06-29T21:00:00+08:00",
		"to": "2026-06-29T22:00:00+08:00"
	}`)

	var resp LoadRecordsResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal mapped records: %v", err)
	}
	if len(resp.Records) != 1 {
		t.Fatalf("records len = %d, want 1", len(resp.Records))
	}
	if resp.Records[0].Client != "node-1" || resp.Records[0].CPU != 12.5 {
		t.Fatalf("unexpected record: %+v", resp.Records[0])
	}
}

func TestNodeMapUnmarshalMap(t *testing.T) {
	raw := []byte(`{
		"node-1": {"uuid":"node-1","name":"alpha","ipv4":"192.0.2.1"},
		"node-2": {"uuid":"node-2","name":"beta","ipv6":"2001:db8::1"}
	}`)

	var nodes NodeMap
	if err := json.Unmarshal(raw, &nodes); err != nil {
		t.Fatalf("unmarshal node map: %v", err)
	}
	if nodes["node-1"].IPv4 != "192.0.2.1" || nodes["node-2"].IPv6 != "2001:db8::1" {
		t.Fatalf("unexpected nodes: %+v", nodes)
	}
}

func TestNodeMapUnmarshalArray(t *testing.T) {
	raw := []byte(`[
		{"uuid":"node-1","name":"alpha","ipv4":"192.0.2.1"},
		{"uuid":"node-2","name":"beta","ipv6":"2001:db8::1"}
	]`)

	var nodes NodeMap
	if err := json.Unmarshal(raw, &nodes); err != nil {
		t.Fatalf("unmarshal node array: %v", err)
	}
	if nodes["node-1"].IPv4 != "192.0.2.1" || nodes["node-2"].IPv6 != "2001:db8::1" {
		t.Fatalf("unexpected nodes: %+v", nodes)
	}
}

func TestMergeNodeDetailsFillsIP(t *testing.T) {
	nodes := map[string]Node{
		"node-1": {UUID: "node-1", Name: "public"},
	}
	detailed := map[string]Node{
		"node-1": {UUID: "node-1", Name: "admin", IPv4: "192.0.2.1", IPv6: "2001:db8::1", Token: "secret"},
	}

	mergeNodeDetails(nodes, detailed)

	if nodes["node-1"].Name != "admin" || nodes["node-1"].IPv4 != "192.0.2.1" || nodes["node-1"].IPv6 != "2001:db8::1" {
		t.Fatalf("detail was not merged: %+v", nodes["node-1"])
	}
}
