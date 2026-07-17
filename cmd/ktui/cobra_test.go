package main

import (
	"bytes"
	"io"
	"os"
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

func TestCobraHelpPreservesDetailedTopics(t *testing.T) {
	for _, tt := range []struct {
		topic string
		want  string
	}{
		{topic: "keys", want: "List layer:"},
		{topic: "config", want: "Precedence:"},
		{topic: "profile", want: "ktui profile add <name>"},
		{topic: "update", want: "Default API URL:"},
		{topic: "export", want: "-o, --output PATH"},
		{topic: "completion", want: "completion <bash|zsh|fish|powershell>"},
		{topic: "version", want: "ktui version - print version information"},
	} {
		t.Run(tt.topic, func(t *testing.T) {
			root := newRootCommand()
			root.SetArgs([]string{"help", tt.topic})
			got := captureStdout(t, func() {
				if err := root.Execute(); err != nil {
					t.Fatal(err)
				}
			})
			if !strings.Contains(got, tt.want) {
				t.Fatalf("ktui help %s output missing %q:\n%s", tt.topic, tt.want, got)
			}
		})
	}
}

func TestCobraProfileAddUsesConfigAndFlags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("KTUI_CONFIG", filepath.Join(t.TempDir(), "wrong.json"))
	root := newRootCommand()
	root.SetArgs([]string{
		"--config", path,
		"profile", "add", "prod",
		"--url", "prod.example.com",
		"--api-key", "secret",
		"--use",
	})
	captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
	})

	cfg, _, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profile != "prod" || cfg.URL != "https://prod.example.com" || cfg.APIKey != "secret" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = write
	defer func() {
		os.Stdout = original
	}()

	run()
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(read)
	if err != nil {
		t.Fatal(err)
	}
	if err := read.Close(); err != nil {
		t.Fatal(err)
	}
	return string(output)
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
