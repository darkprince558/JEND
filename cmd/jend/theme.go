package main

import (
	"fmt"
	"os"

	"github.com/darkprince558/jend/internal/config"
	"github.com/darkprince558/jend/internal/ui"
	"github.com/spf13/cobra"
)

var themeCmd = &cobra.Command{
	Use:   "theme",
	Short: "Pick a color theme interactively",
	Long: `Launch an interactive theme picker to scroll through available
color themes and see a live preview before selecting.

The selected theme is saved to your config and applied on every
future jend command automatically.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Get the currently saved theme name from config
		savedTheme := ""
		if cfg, err := config.Load(); err == nil {
			savedTheme = cfg.Theme
		}

		result, err := ui.RunThemePicker(savedTheme)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		if result.Cancelled || result.Selected == "" {
			fmt.Println("No changes made.")
			return
		}

		// Save the selected theme
		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		cfg.Theme = result.Selected
		if err := config.Save(cfg); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
			os.Exit(1)
		}

		// Apply and show confirmation with the theme's own colors
		ui.InitTheme(result.Selected, cfg.ThemeColors)

		fmt.Printf("\n  Theme set to: ")
		fmt.Printf("\033[1m%s\033[0m\n", result.Selected)
		fmt.Println("  It will be applied on your next jend command.")
		fmt.Println()
	},
}

func init() {
	rootCmd.AddCommand(themeCmd)
}
