package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"ktui/internal/config"
	"ktui/internal/update"
)

var (
	completionShells = []string{"bash", "zsh", "fish", "powershell"}

	cobraExportFormats = []string{"markdown", "csv", "json"}
	cobraConfigKeys    = []string{
		"profile",
		"url",
		"api-key",
		"interval",
		"timeout",
		"realtime-window",
		"chart-y-axis",
		"warn-cpu",
		"warn-ram",
		"warn-disk",
		"warn-expiry-days",
		"mode",
		"ascii",
		"no-color",
	}
)

func execute() error {
	return newRootCommand().Execute()
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "ktui",
		Short:         "Komari terminal UI",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleTUI(changedFlagArgs(cmd))
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().String("config", defaultConfigPath(), "config file path")
	must(root.MarkPersistentFlagFilename("config"))

	addTUIFlags(root)
	addConnectionFlagCompletions(root)
	addTUIFlagCompletions(root)

	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return setConfigEnvFromCommand(cmd)
	}

	root.AddCommand(
		newStatusCommand(),
		newExportCommand(),
		newConfigCommand(),
		newProfileCommand(),
		newUpdateCommand(),
		newVersionCommand(),
		newKeysCommand(),
		newCompletionCommand(),
	)
	return root
}

func newStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Fetch once and print a node summary",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleStatus(changedFlagArgs(cmd))
		},
	}
	addStatusFlags(cmd)
	addConnectionFlagCompletions(cmd)
	return cmd
}

func newExportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:       "export <markdown|csv|json>",
		Short:     "Export current node status",
		ValidArgs: cobraExportFormats,
		Args:      cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return handleExport(nil)
			}
			next := append([]string{args[0]}, changedFlagArgs(cmd)...)
			return handleExport(next)
		},
	}
	addStatusFlags(cmd)
	cmd.Flags().StringP("output", "o", "", "write export to a file instead of stdout")
	must(cmd.MarkFlagFilename("output"))
	addConnectionFlagCompletions(cmd)
	return cmd
}

func newConfigCommand() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Create the config file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleConfig(append([]string{"init"}, changedFlagArgs(cmd)...))
		},
	}
	initCmd.Flags().Bool("force", false, "overwrite existing config")
	must(initCmd.RegisterFlagCompletionFunc("force", cobra.NoFileCompletions))

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage persistent settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleConfig(nil)
		},
	}
	cmd.AddCommand(
		initCmd,
		&cobra.Command{
			Use:   "path",
			Short: "Print the config path",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return handleConfig([]string{"path"})
			},
		},
		&cobra.Command{
			Use:   "show",
			Short: "Print the config file with secrets redacted",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return handleConfig([]string{"show"})
			},
		},
		&cobra.Command{
			Use:               "set <key> <value>",
			Short:             "Set a config value",
			Args:              cobra.ExactArgs(2),
			ValidArgsFunction: configSetCompletions,
			RunE: func(cmd *cobra.Command, args []string) error {
				return handleConfig([]string{"set", args[0], args[1]})
			},
		},
		&cobra.Command{
			Use:   "help",
			Short: "Show config help",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return handleConfig([]string{"help"})
			},
		},
	)
	return cmd
}

func newProfileCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage Komari connection profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleProfile(nil)
		},
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List configured profiles",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return handleProfile([]string{"list"})
			},
		},
		&cobra.Command{
			Use:   "current",
			Short: "Print the active profile",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return handleProfile([]string{"current"})
			},
		},
		&cobra.Command{
			Use:               "use <name>",
			Short:             "Switch active profile",
			Args:              cobra.ExactArgs(1),
			ValidArgsFunction: profileNameArgCompletions,
			RunE: func(cmd *cobra.Command, args []string) error {
				return handleProfile([]string{"use", args[0]})
			},
		},
		newProfileAddCommand(),
		&cobra.Command{
			Use:               "rename <old> <new>",
			Short:             "Rename a profile",
			Args:              cobra.ExactArgs(2),
			ValidArgsFunction: profileRenameArgCompletions,
			RunE: func(cmd *cobra.Command, args []string) error {
				return handleProfile([]string{"rename", args[0], args[1]})
			},
		},
		&cobra.Command{
			Use:               "remove <name>",
			Short:             "Remove a profile",
			Args:              cobra.ExactArgs(1),
			ValidArgsFunction: profileNameArgCompletions,
			RunE: func(cmd *cobra.Command, args []string) error {
				return handleProfile([]string{"remove", args[0]})
			},
		},
		&cobra.Command{
			Use:   "help",
			Short: "Show profile help",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return handleProfile([]string{"help"})
			},
		},
	)
	return cmd
}

func newProfileAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			next := append([]string{"add", args[0]}, changedFlagArgs(cmd)...)
			return handleProfile(next)
		},
	}
	cmd.Flags().String("url", "", "Komari base URL")
	cmd.Flags().String("api-key", "", "Komari API key")
	cmd.Flags().Bool("use", false, "make this profile active")
	must(cmd.RegisterFlagCompletionFunc("url", cobra.NoFileCompletions))
	must(cmd.RegisterFlagCompletionFunc("api-key", cobra.NoFileCompletions))
	must(cmd.RegisterFlagCompletionFunc("use", cobra.NoFileCompletions))
	cmd.ValidArgsFunction = cobra.NoFileCompletions
	return cmd
}

func newUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check or install ktui updates",
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleUpdate(nil)
		},
	}
	cmd.AddCommand(
		newUpdateCheckCommand(),
		newUpdateInstallCommand(),
		&cobra.Command{
			Use:   "help",
			Short: "Show update help",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return handleUpdate([]string{"help"})
			},
		},
	)
	return cmd
}

func newUpdateCheckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check for a ktui update",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleUpdate(append([]string{"check"}, changedFlagArgs(cmd)...))
		},
	}
	addUpdateFlags(cmd, false)
	return cmd
}

func newUpdateInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install a ktui update",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleUpdate(append([]string{"install"}, changedFlagArgs(cmd)...))
		},
	}
	addUpdateFlags(cmd, true)
	return cmd
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			printVersion()
		},
	}
}

func newKeysCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "keys",
		Short: "Show TUI key bindings",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			printKeysHelp()
		},
	}
}

func newCompletionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "completion <bash|zsh|fish|powershell>",
		Short:                 "Generate shell completion scripts",
		ValidArgs:             completionShells,
		Args:                  cobra.RangeArgs(0, 1),
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return fmt.Errorf("unknown completion shell %q: use bash, zsh, fish, or powershell", args[0])
			}
		},
	}
	return cmd
}

func addTUIFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	addConnectionFlags(flags, 10*time.Second)
	flags.Duration("interval", 5*time.Second, "refresh interval")
	flags.Duration("realtime-window", time.Minute, "realtime chart time window: 1m, 5m, or 10m")
	flags.String("chart-y-axis", "absolute", "percent chart Y axis mode: absolute or relative")
	flags.String("mode", "sheet", "view mode: sheet or line")
	flags.Bool("ascii", false, "use ASCII-only rendering for terminals/fonts with Unicode issues")
	flags.Bool("no-color", false, "disable ANSI color and inverse video")
}

func addStatusFlags(cmd *cobra.Command) {
	addConnectionFlags(cmd.Flags(), 10*time.Second)
}

func addConnectionFlags(flags *pflag.FlagSet, timeoutDefault time.Duration) {
	flags.String("profile", config.DefaultProfile, "profile name")
	flags.String("url", "", "Komari base URL")
	flags.String("api-key", "", "Komari API key (sent as Bearer token)")
	flags.Duration("timeout", timeoutDefault, "HTTP timeout")
}

func addUpdateFlags(cmd *cobra.Command, withTag bool) {
	cmd.Flags().String("api-url", update.DefaultAPIBaseURL, "Gitea repository API URL")
	cmd.Flags().Duration("timeout", 60*time.Second, "update HTTP timeout")
	if withTag {
		cmd.Flags().String("tag", "", "install a specific release tag instead of the latest release")
		must(cmd.RegisterFlagCompletionFunc("tag", cobra.NoFileCompletions))
	}
	must(cmd.RegisterFlagCompletionFunc("api-url", cobra.NoFileCompletions))
	must(cmd.RegisterFlagCompletionFunc("timeout", cobra.NoFileCompletions))
}

func addConnectionFlagCompletions(cmd *cobra.Command) {
	must(cmd.RegisterFlagCompletionFunc("profile", profileFlagCompletions))
	must(cmd.RegisterFlagCompletionFunc("url", cobra.NoFileCompletions))
	must(cmd.RegisterFlagCompletionFunc("api-key", cobra.NoFileCompletions))
	must(cmd.RegisterFlagCompletionFunc("timeout", cobra.NoFileCompletions))
}

func addTUIFlagCompletions(cmd *cobra.Command) {
	must(cmd.RegisterFlagCompletionFunc("interval", cobra.NoFileCompletions))
	must(cmd.RegisterFlagCompletionFunc("realtime-window", cobra.FixedCompletions([]cobra.Completion{"1m", "5m", "10m"}, cobra.ShellCompDirectiveNoFileComp)))
	must(cmd.RegisterFlagCompletionFunc("chart-y-axis", cobra.FixedCompletions([]cobra.Completion{"absolute", "relative"}, cobra.ShellCompDirectiveNoFileComp)))
	must(cmd.RegisterFlagCompletionFunc("mode", cobra.FixedCompletions([]cobra.Completion{"sheet", "line"}, cobra.ShellCompDirectiveNoFileComp)))
	must(cmd.RegisterFlagCompletionFunc("ascii", cobra.NoFileCompletions))
	must(cmd.RegisterFlagCompletionFunc("no-color", cobra.NoFileCompletions))
}

func changedFlagArgs(cmd *cobra.Command) []string {
	args := []string{}
	cmd.Flags().Visit(func(flag *pflag.Flag) {
		if flag.Name == "config" {
			return
		}
		args = append(args, "--"+flag.Name+"="+flag.Value.String())
	})
	return args
}

func setConfigEnvFromCommand(cmd *cobra.Command) error {
	value, err := cmd.Flags().GetString("config")
	if err != nil {
		return err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return os.Setenv("KTUI_CONFIG", value)
}

func defaultConfigPath() string {
	path, err := config.Path()
	if err != nil {
		return ""
	}
	return path
}

func profileFlagCompletions(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	return profileCompletions(cmd, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func profileNameArgCompletions(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return profileCompletions(cmd, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func profileRenameArgCompletions(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return profileCompletions(cmd, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
	if len(args) == 1 {
		return profileCompletionsExcept(cmd, toComplete, args[0]), cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func configSetCompletions(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return filterCompletions(cobraConfigKeys, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
	if len(args) == 1 {
		return filterCompletions(configValueCompletionsForCobra(cmd, args[0]), toComplete), cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func configValueCompletionsForCobra(cmd *cobra.Command, key string) []string {
	switch strings.TrimSpace(strings.ToLower(key)) {
	case "profile":
		return completionProfileNamesForCommand(cmd)
	case "mode":
		return []string{"sheet", "line"}
	case "realtime-window", "realtime_window":
		return []string{"1m", "5m", "10m"}
	case "chart-y-axis", "chart_y_axis":
		return []string{"absolute", "relative"}
	case "ascii", "no-color", "no_color":
		return []string{"true", "false"}
	default:
		return nil
	}
}

func profileCompletions(cmd *cobra.Command, toComplete string) []cobra.Completion {
	return filterCompletions(completionProfileNamesForCommand(cmd), toComplete)
}

func profileCompletionsExcept(cmd *cobra.Command, toComplete string, exclude string) []cobra.Completion {
	names := completionProfileNamesForCommand(cmd)
	out := make([]string, 0, len(names))
	for _, name := range names {
		if name != exclude {
			out = append(out, name)
		}
	}
	return filterCompletions(out, toComplete)
}

func completionProfileNamesForCommand(cmd *cobra.Command) []string {
	cfg, err := loadCompletionConfig(cmd)
	if err != nil {
		return []string{config.DefaultProfile}
	}
	return cfg.ProfileNames()
}

func loadCompletionConfig(cmd *cobra.Command) (config.Config, error) {
	path, _ := cmd.Flags().GetString("config")
	path = strings.TrimSpace(path)
	if path == "" {
		cfg, _, err := config.Load()
		return cfg, err
	}
	old, hadOld := os.LookupEnv("KTUI_CONFIG")
	if err := os.Setenv("KTUI_CONFIG", path); err != nil {
		return config.Config{}, err
	}
	defer func() {
		if hadOld {
			_ = os.Setenv("KTUI_CONFIG", old)
		} else {
			_ = os.Unsetenv("KTUI_CONFIG")
		}
	}()
	cfg, _, err := config.Load()
	return cfg, err
}

func filterCompletions(values []string, prefix string) []cobra.Completion {
	out := make([]cobra.Completion, 0, len(values))
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			out = append(out, value)
		}
	}
	return out
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
