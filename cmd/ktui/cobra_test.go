package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"ktui/internal/config"
)

func TestCobraRootCommandTree(t *testing.T) {
	root := newRootCommand()
	for _, name := range []string{"status", "export", "config", "profile", "update", "version", "keys", "completion"} {
		cmd, _, err := root.Find([]string{name})
		if err != nil {
			t.Fatalf("find %q error = %v", name, err)
		}
		if cmd == nil || cmd.Name() != name {
			t.Fatalf("find %q = %v", name, cmd)
		}
	}
}

func TestCobraChangedFlagArgsSkipsConfig(t *testing.T) {
	root := newRootCommand()
	if err := root.ParseFlags([]string{"--mode", "line", "--config", "custom.json", "--ascii"}); err != nil {
		t.Fatal(err)
	}
	args := changedFlagArgs(root)
	if !hasString(args, "--mode=line") || !hasString(args, "--ascii=true") {
		t.Fatalf("changedFlagArgs = %#v, want changed root flags", args)
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "--config=") {
			t.Fatalf("changedFlagArgs should skip config: %#v", args)
		}
	}
}

func TestCobraCompletionUsesConfiguredProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	cfg, err := config.AddProfile(config.Default(), "prod", "https://prod.example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("KTUI_CONFIG", path)
	if _, err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	root := newRootCommand()
	status, _, err := root.Find([]string{"status"})
	if err != nil {
		t.Fatal(err)
	}
	if err := status.ParseFlags([]string{"--config", path}); err != nil {
		t.Fatal(err)
	}
	got := profileCompletions(status, "pr")
	if !hasCompletion(got, "prod") {
		t.Fatalf("profile completions = %#v, want prod", got)
	}
}

func TestCobraHiddenCompletionCommand(t *testing.T) {
	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"__complete", "completion", ""})

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"bash", "zsh", "fish", "powershell"} {
		if !strings.Contains(text, want) {
			t.Fatalf("hidden completion output missing %q:\n%s", want, text)
		}
	}
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasCompletion(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
