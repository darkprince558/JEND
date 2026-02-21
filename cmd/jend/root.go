package main

import (
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/darkprince558/jend/internal/config"
	"github.com/darkprince558/jend/internal/ui"
	"github.com/spf13/cobra"
)

var (
	// Persistent Flags
	headless   bool
	timeoutStr string
	themeFlag  string // "auto", "dark", or "light"

	// App Version (injected at build time)
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// RootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "jend",
	Short: "Secure, direct file transfer tool",
	Long: `JEND is a secure, direct file transfer tool.
It allows you to send files, text, and directories directly between devices
on the same network or over the internet using a simple code.`,
	// PersistentPreRun runs before every command and its subcommands.
	// It resolves the theme: --theme flag > config file > auto-detect.
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		themeName := themeFlag // default "auto"

		// If flag is still "auto", check config file for a saved theme
		if themeName == "auto" {
			if cfg, err := config.Load(); err == nil && cfg.Theme != "" {
				themeName = cfg.Theme
			}
		}

		// Load per-color overrides from config
		var overrides map[string]string
		if cfg, err := config.Load(); err == nil && cfg.ThemeColors != nil {
			overrides = cfg.ThemeColors
		}

		ui.InitTheme(themeName, overrides)
	},
	Run: func(cmd *cobra.Command, args []string) {
		banner := ui.RenderBannerWithTagline()

		versionLine := lipgloss.NewStyle().
			Foreground(ui.ColorSubtext).
			Align(lipgloss.Center).
			Render(fmt.Sprintf("v%s", version))

		usage := lipgloss.NewStyle().
			Foreground(ui.ColorText).
			Align(lipgloss.Left).
			Render("> jend send <file>     Send a file\n> jend receive <code>  Receive a file\n> jend --help          Show all commands")

		output := lipgloss.JoinVertical(lipgloss.Center,
			banner,
			"",
			versionLine,
			"",
			usage,
			"",
		)

		fmt.Println(lipgloss.NewStyle().Padding(1, 2).Render(output))
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of JEND",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("JEND v%s\n", version)
		fmt.Printf("Commit: %s\n", commit)
		fmt.Printf("Built:  %s\n", date)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(versionCmd)

	// Persistent Flags (Available to all commands)
	rootCmd.PersistentFlags().BoolVar(&headless, "headless", false, "Run in headless mode (no TUI)")
	rootCmd.PersistentFlags().StringVar(&timeoutStr, "timeout", "10m", "Global timeout duration (e.g. 10m)")
	rootCmd.PersistentFlags().StringVar(&themeFlag, "theme", "auto", "Color theme: auto, dark, or light")
}

func getTimeout() time.Duration {
	d, err := time.ParseDuration(timeoutStr)
	if err != nil {
		fmt.Printf("Invalid timeout format: %v\n", err)
		os.Exit(1)
	}
	return d
}
