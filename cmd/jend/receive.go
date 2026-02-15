package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/darkprince558/jend/internal/config"
	"github.com/darkprince558/jend/internal/core"
	"github.com/darkprince558/jend/internal/transport"
	"github.com/darkprince558/jend/internal/ui"
	"github.com/spf13/cobra"
)

var (
	// Flags for Receive
	outputDir      string
	autoUnzip      bool
	recNoClipboard bool
	recNoHistory   bool
	recIncognito   bool
	concurrency    int
	recAutoApprove bool
	// Re-declare relay flags (could share but globals are messy)
	recRelayURL  string
	recRelayUser string
	recRelayPass string
)

var receiveCmd = &cobra.Command{
	Use:   "receive [code]",
	Short: "Receive a file using a code",
	Long: `Receive a file from a sender using the 3-word code they provided.
Example:
  jend receive confident-blue-eagle
  jend receive --relay-url "turn:my.relay.click" ...`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		code := args[0]

		// Handle Incognito
		if recIncognito {
			recNoHistory = true
			recNoClipboard = true
		}

		// Handle Relay Config
		// 1. Try Config File
		cfg, _ := config.Load()

		// 2. Override with Config File if flags missing
		// Note: We intentionally reuse the global vars recRelayURL etc.
		if recRelayURL == "" && cfg.RelayURL != "" {
			recRelayURL = cfg.RelayURL
			recRelayUser = cfg.RelayUser
			recRelayPass = cfg.RelayPass
		}

		var turnCfg *transport.CustomTurnConfig
		if recRelayURL != "" {
			turnCfg = &transport.CustomTurnConfig{
				URL:      recRelayURL,
				Username: recRelayUser,
				Password: recRelayPass,
			}
		}

		if flagPort == "" {
			flagPort = "9000"
		}
		startReceiver(code, headless, flagPort, outputDir, autoUnzip, recNoClipboard, recNoHistory, concurrency, turnCfg, recAutoApprove)
	},
}

func init() {
	rootCmd.AddCommand(receiveCmd)

	receiveCmd.Flags().StringVar(&outputDir, "dir", ".", "Output directory")
	receiveCmd.Flags().BoolVar(&autoUnzip, "unzip", false, "Automatically unzip received archives")
	receiveCmd.Flags().BoolVar(&recNoClipboard, "no-clipboard", false, "Disable clipboard copy")
	receiveCmd.Flags().BoolVar(&recNoHistory, "no-history", false, "Disable audit logging")
	receiveCmd.Flags().BoolVar(&recIncognito, "incognito", false, "Enable incognito mode (no history, no clipboard)")
	receiveCmd.Flags().IntVar(&concurrency, "concurrency", 4, "Number of parallel download streams")

	// Relay Flags
	receiveCmd.Flags().StringVar(&recRelayURL, "relay-url", "", "Custom TURN Relay URL")
	receiveCmd.Flags().StringVar(&recRelayUser, "relay-user", "", "TURN Relay Username")
	receiveCmd.Flags().StringVar(&recRelayPass, "relay-pass", "", "TURN Relay Password")
	receiveCmd.Flags().BoolVarP(&recAutoApprove, "yes", "y", false, "Automatically accept file transfer without prompt")
	receiveCmd.Flags().StringVar(&flagPort, "port", "9000", "Port to connect to (default 9000)")
}

// startReceiver initializes the receiver process.
// It connects using the provided code and starts the TUI or Headless mode.
func startReceiver(code string, headless bool, port string, outputDir string, autoUnzip bool, noClipboard bool, noHistory bool, concurrency int, turnCfg *transport.CustomTurnConfig, autoApprove bool) {
	if headless {
		// Headless Mode
		core.RunReceiver(nil, code, port, outputDir, autoUnzip, noClipboard, noHistory, concurrency, turnCfg, autoApprove)
	} else {
		model := ui.NewModel(ui.RoleReceiver, "", code)
		p := tea.NewProgram(model, tea.WithAltScreen())

		// Transfer Logic
		go func() {
			core.RunReceiver(p, code, port, outputDir, autoUnzip, noClipboard, noHistory, concurrency, turnCfg, autoApprove)
			p.Quit()
		}()

		if _, err := p.Run(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	}
}
