package main

import (
	"fmt"
	"os"

	"github.com/darkprince558/jend/internal/config"
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
		fmt.Println("🗑️  Configuration cleared. Reverted to defaults.")
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
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetRelayCmd)
	configCmd.AddCommand(configClearCmd)
	configCmd.AddCommand(configShowCmd)

	configSetRelayCmd.Flags().StringVar(&relayURL, "url", "", "TURN Relay URL")
	configSetRelayCmd.Flags().StringVar(&relayUser, "user", "", "TURN Relay Username")
	configSetRelayCmd.Flags().StringVar(&relayPass, "pass", "", "TURN Relay Password")
}
