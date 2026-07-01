package main

import "testing"

func TestSplitConfigArgSeparateValue(t *testing.T) {
	args, path := splitConfigArg([]string{"--config", "/tmp/ktui.json", "config", "show"})
	if path != "/tmp/ktui.json" {
		t.Fatalf("path = %q", path)
	}
	want := []string{"config", "show"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %#v", args)
		}
	}
}

func TestSplitConfigArgEqualsValue(t *testing.T) {
	args, path := splitConfigArg([]string{"config", "show", "--config=/tmp/ktui.json"})
	if path != "/tmp/ktui.json" {
		t.Fatalf("path = %q", path)
	}
	want := []string{"config", "show"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %#v", args)
		}
	}
}

func TestLooksLikeCommand(t *testing.T) {
	if !looksLikeCommand("add") {
		t.Fatal("add should look like a command")
	}
	if looksLikeCommand("--sheet") {
		t.Fatal("--sheet should not look like a command")
	}
	if looksLikeCommand("") {
		t.Fatal("empty string should not look like a command")
	}
}
