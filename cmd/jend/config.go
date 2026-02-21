package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/darkprince558/jend/internal/config"
	"github.com/darkprince558/jend/internal/ui"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long:  `View and modify persistent configuration settings like relay details.`,
}

var configSetRelayCmd = &cobra.Command{
	Use:   "set-relay",
	Short: "Configure custom TURN relay credentials",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		if relayURL != "" {
			cfg.RelayURL = relayURL
		}
		if relayUser != "" {
			cfg.RelayUser = relayUser
		}
		if relayPass != "" {
			cfg.RelayPass = relayPass
		}

		if err := config.Save(cfg); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Configuration updated.")
		fmt.Printf("   Relay URL:  %s\n", cfg.RelayURL)
		fmt.Printf("   Relay User: %s\n", cfg.RelayUser)
	},
}

var configClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all saved configuration",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := &config.Config{} // Empty config
		if err := config.Save(cfg); err != nil {
			fmt.Printf("Error clearing config: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Configuration cleared. Reverted to defaults.")
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Current Configuration:")
		fmt.Println("----------------------")
		if cfg.RelayURL == "" {
			fmt.Println("Relay: Default (AWS/Internal)")
		} else {
			fmt.Printf("Relay URL:  %s\n", cfg.RelayURL)
			fmt.Printf("Relay User: %s\n", cfg.RelayUser)
			fmt.Printf("Relay Pass: ******\n")
		}

		fmt.Println("")
		if cfg.Theme == "" {
			fmt.Println("Theme: auto (dark/light auto-detected)")
		} else {
			fmt.Printf("Theme: %s\n", cfg.Theme)
		}
		if len(cfg.ThemeColors) > 0 {
			fmt.Println("Color Overrides:")
			keys := make([]string, 0, len(cfg.ThemeColors))
			for k := range cfg.ThemeColors {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Printf("  %s = %s\n", k, cfg.ThemeColors[k])
			}
		}
	},
}

var configSetThemeCmd = &cobra.Command{
	Use:   "set-theme <name>",
	Short: "Set the color theme (dark, light, dracula, nord, catppuccin, solarized)",
	Long: `Choose a named color theme that persists across sessions.
Available themes: dark, light, dracula, nord, catppuccin, solarized.
Use 'auto' to revert to auto-detection based on your terminal background.

Example:
  jend config set-theme dracula
  jend config set-theme auto`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := strings.ToLower(args[0])

		// Validate
		if name != "auto" {
			if _, ok := ui.NamedThemes[name]; !ok {
				var validNames []string
				for k := range ui.NamedThemes {
					validNames = append(validNames, k)
				}
				sort.Strings(validNames)
				fmt.Printf("Unknown theme: %s\n", name)
				fmt.Printf("Available themes: %s\n", strings.Join(validNames, ", "))
				os.Exit(1)
			}
		}

		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		if name == "auto" {
			cfg.Theme = "" // empty = auto-detect
		} else {
			cfg.Theme = name
		}

		if err := config.Save(cfg); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
			os.Exit(1)
		}

		if name == "auto" {
			fmt.Println("Theme set to auto (will detect from terminal).")
		} else {
			fmt.Printf("Theme set to: %s\n", name)
		}
	},
}

var configSetColorCmd = &cobra.Command{
	Use:   "set-color <key> <hex>",
	Short: "Override a single theme color (e.g. primary, accent)",
	Long: `Set a custom hex color for a specific palette key.
These overrides are applied on top of whatever named theme you have selected.

Keys: primary, secondary, accent, error, warning, text, subtext, bg, panel

Example:
  jend config set-color primary "#FF6B6B"
  jend config set-color accent "#FFE66D"`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		key := strings.ToLower(args[0])
		hex := args[1]

		// Validate key
		validKeys := []string{"primary", "secondary", "accent", "error", "warning", "text", "subtext", "bg", "panel"}
		valid := false
		for _, v := range validKeys {
			if key == v {
				valid = true
				break
			}
		}
		if !valid {
			fmt.Printf("Unknown color key: %s\n", key)
			fmt.Printf("Valid keys: %s\n", strings.Join(validKeys, ", "))
			os.Exit(1)
		}

		// Basic hex validation
		if !strings.HasPrefix(hex, "#") || (len(hex) != 4 && len(hex) != 7) {
			fmt.Printf("Invalid hex color: %s (expected format: #RGB or #RRGGBB)\n", hex)
			os.Exit(1)
		}

		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		if cfg.ThemeColors == nil {
			cfg.ThemeColors = make(map[string]string)
		}
		cfg.ThemeColors[key] = hex

		if err := config.Save(cfg); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Color override set: %s = %s\n", key, hex)
	},
}

var configClearColorsCmd = &cobra.Command{
	Use:   "clear-colors",
	Short: "Remove all custom color overrides",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		cfg.ThemeColors = nil
		if err := config.Save(cfg); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Color overrides cleared.")
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetRelayCmd)
	configCmd.AddCommand(configClearCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetThemeCmd)
	configCmd.AddCommand(configSetColorCmd)
	configCmd.AddCommand(configClearColorsCmd)

	configSetRelayCmd.Flags().StringVar(&relayURL, "url", "", "TURN Relay URL")
	configSetRelayCmd.Flags().StringVar(&relayUser, "user", "", "TURN Relay Username")
	configSetRelayCmd.Flags().StringVar(&relayPass, "pass", "", "TURN Relay Password")
}
