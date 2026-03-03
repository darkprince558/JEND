package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/darkprince558/jend/internal/config"
	"github.com/darkprince558/jend/internal/core"
	"github.com/darkprince558/jend/internal/osutils"
	"github.com/darkprince558/jend/internal/signaling"
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
	recHost        string
	// Re-declare relay flags (could share but globals are messy)
	recRelayURL  string
	recRelayUser string
	recRelayPass string
	// QR Receive flags
	recUseQR    bool
	recQRLimit  int
	recQRExpire time.Duration
	recQRMode   string // "local" or "cloud"
)

var receiveCmd = &cobra.Command{
	Use:   "receive [code]",
	Short: "Receive a file using a code",
	Long: `Receive a file from a sender using the 3-word code they provided.
You can also use --qr to receive files from a phone via QR code.
Example:
  jend receive confident-blue-eagle
  jend receive --qr
  jend receive --qr --qr-mode=cloud
  jend receive --qr --dir ~/Downloads
  jend receive --relay-url "turn:my.relay.click" ...`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		code := ""

		// Interactive unified menu if no args and no QR flag are supplied
		if len(args) == 0 && !recUseQR && !headless {
			opts, err := ui.RunReceivePrompt()
			if err != nil || opts.Cancelled {
				fmt.Println("Cancelled.")
				return
			}

			if opts.Mode == "code" {
				code = opts.TransferCode // Fall through to standard P2P below
			} else if opts.Mode == "qr" {
				recUseQR = true
				recQRLimit = opts.QROpts.MaxDownloads
				recQRExpire = opts.QROpts.ExpireAfter
				recAutoApprove = opts.QROpts.AutoApprove
				if opts.QROpts.Mode == "cloud" {
					recQRMode = "cloud"
				}
			}
		}

		// QR mode: start upload server instead of P2P receiver
		if recUseQR {
			// Show interactive QR prompt if no explicit flags were set AND not headless
			// (If they came from the unified menu above, this prompt is skipped because flags like qr-limit are logically "set" by the struct logic, but cobra Flags().Changed() checks actual CLI flags. Let's fix that by ensuring the unified menu bypasses this inner prompt.)
			needsPrompt := !cmd.Flags().Changed("qr-limit") && !cmd.Flags().Changed("qr-expire") && !cmd.Flags().Changed("qr-mode") && !headless

			// We only need the inner prompt if they ran `jend receive --qr` directly.
			if needsPrompt && len(args) > 0 { // len(args) > 0 check ensures it didn't come from unified menu
				opts, err := ui.RunQRPrompt()
				if err != nil || opts.Cancelled {
					fmt.Println("Cancelled.")
					return
				}
				recQRLimit = opts.MaxDownloads
				recQRExpire = opts.ExpireAfter
				recAutoApprove = opts.AutoApprove

				if opts.Mode == "cloud" {
					recQRMode = "cloud"
				}
			}

			// Ensure output directory exists
			absDir, err := filepath.Abs(outputDir)
			if err != nil {
				fmt.Printf("Error resolving output directory: %v\n", err)
				os.Exit(1)
			}
			if err := os.MkdirAll(absDir, 0o755); err != nil {
				fmt.Printf("Error creating output directory: %v\n", err)
				os.Exit(1)
			}

			if recQRMode == "cloud" {
				startCloudQRReceiver(absDir)
			} else {
				startQRReceiver(absDir)
			}
			return
		}

		// Normal P2P receive — code is required
		if code == "" && len(args) > 0 {
			code = args[0]
		}

		if code == "" {
			fmt.Println("Error: code argument is required (use --qr for phone upload mode)")
			os.Exit(1)
		}

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
		startReceiver(code, headless, recHost, flagPort, outputDir, autoUnzip, recNoClipboard, recNoHistory, concurrency, turnCfg, recAutoApprove)
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
	receiveCmd.Flags().StringVar(&recHost, "host", "", "Connect directly to host (skip discovery)")

	// QR Receive Flags
	receiveCmd.Flags().BoolVar(&recUseQR, "qr", false, "Start a QR upload server to receive files from a phone")
	receiveCmd.Flags().IntVar(&recQRLimit, "qr-limit", 0, "Max number of uploads allowed (0 = unlimited)")
	receiveCmd.Flags().DurationVar(&recQRExpire, "qr-expire", 0, "Auto-expire the QR server after duration (e.g. 15m, 1h)")
	receiveCmd.Flags().StringVar(&recQRMode, "qr-mode", "local", "QR transfer mode: local or cloud")
}

// startReceiver initializes the receiver process.
// It connects using the provided code and starts the TUI or Headless mode.
func startReceiver(code string, headless bool, host string, port string, outputDir string, autoUnzip bool, noClipboard bool, noHistory bool, concurrency int, turnCfg *transport.CustomTurnConfig, autoApprove bool) {
	if headless {
		// Headless Mode
		core.RunReceiver(nil, code, host, port, outputDir, autoUnzip, noClipboard, noHistory, concurrency, turnCfg, autoApprove)
	} else {
		model := ui.NewModel(ui.RoleReceiver, "", code)
		p := tea.NewProgram(model, tea.WithAltScreen())

		// Transfer Logic
		go func() {
			core.RunReceiver(p, code, host, port, outputDir, autoUnzip, noClipboard, noHistory, concurrency, turnCfg, autoApprove)
			// p.Quit() removed to keep UI open for text viewing.
			// The UI model will handle the decision to quit or stay open.
		}()

		if _, err := p.Run(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	}
}

// startQRReceiver starts a local HTTP upload server and prints a QR code.
// Users scan the QR on their phone to upload files to the laptop.
func startQRReceiver(outputDir string) {
	// Context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, osutils.ShutdownSignals...)
	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		cancel()
	}()

	logActiveStyle := lipgloss.NewStyle().Foreground(ui.ColorAccent)
	logSuccessStyle := lipgloss.NewStyle().Foreground(ui.ColorSuccess)
	logWarnStyle := lipgloss.NewStyle().Foreground(ui.ColorWarning)
	logMutedStyle := lipgloss.NewStyle().Foreground(ui.ColorSubtext)

	srv := core.NewQRUploadServer(core.QRUploadServerConfig{
		OutputDir:   outputDir,
		Port:        8888,
		MaxUploads:  recQRLimit,
		ExpireAfter: recQRExpire,
		OnUploadStart: func(filename string) {
			fmt.Printf("\r  %s %s\033[K", logActiveStyle.Render("⬇"), filename)
		},
		OnProgress: func(recv, total int64) {
			if total > 0 {
				pct := float64(recv) / float64(total) * 100
				fmt.Printf("\r  %s %-30s %.0f%%\033[K", logActiveStyle.Render("⬇"), "Receiving...", pct)
			}
		},
		OnApprovalRequired: func(name string, size int64) bool {
			if recAutoApprove {
				return true
			}
			return ui.PromptApproval(name, formatBytesReceive(size))
		},
		OnComplete: func(filename string, uploadCount int) {
			fmt.Printf("\r  %s %s %s\033[K\n", logSuccessStyle.Render("✓"), filename, logMutedStyle.Render(fmt.Sprintf("(file #%d)", uploadCount)))
			if recQRLimit > 0 {
				remaining := recQRLimit - uploadCount
				if remaining > 0 {
					fmt.Printf("  %s %s\n", logMutedStyle.Render("..."), logMutedStyle.Render(fmt.Sprintf("%d/%d uploads used. Ctrl+C to stop.", uploadCount, recQRLimit)))
				}
			} else {
				fmt.Printf("  %s %s\n", logMutedStyle.Render("..."), logMutedStyle.Render("Waiting for more uploads... (Ctrl+C to stop)"))
			}
		},
		OnTextComplete: func(text string) {
			fmt.Printf("\r  %s Received Text:\033[K\n\n%s\n\n", logSuccessStyle.Render("✓"), ui.CodeStyle.Render(text))
			if !recNoClipboard {
				if err := clipboard.WriteAll(text); err == nil {
					fmt.Printf("  %s\n", logMutedStyle.Render("(Copied to clipboard)"))
				}
			}
			fmt.Printf("  %s %s\n", logMutedStyle.Render("..."), logMutedStyle.Render("Waiting for more... (Ctrl+C to stop)"))
		},
		OnLimitReached: func() {
			fmt.Printf("\n  %s %s\n", logWarnStyle.Render("!"), logWarnStyle.Render(fmt.Sprintf("Upload limit (%d) reached. Shutting down...", recQRLimit)))
			go func() {
				time.Sleep(2 * time.Second)
				cancel()
			}()
		},
		OnExpire: func() {
			fmt.Printf("\n  %s %s\n", logWarnStyle.Render("!"), logWarnStyle.Render(fmt.Sprintf("QR code expired after %s. Shutting down...", recQRExpire)))
		},
	})

	ipv4URL, ipv6URL := srv.URLs()

	// Use IPv4 for QR code (Safari/mobile browsers don't support IPv6 link-local URLs)
	qrURL := ipv4URL

	// Print banner and QR
	fmt.Println(ui.RenderBanner())
	fmt.Println()
	for _, line := range strings.Split(ui.RenderQR(qrURL), "\n") {
		fmt.Println("    " + line)
	}
	fmt.Println()

	// Print info below QR
	hintStyle := lipgloss.NewStyle().Foreground(ui.ColorSubtext).Faint(true)
	urlStyle := lipgloss.NewStyle().Foreground(ui.ColorAccent).Bold(true)
	dirStyle := lipgloss.NewStyle().Foreground(ui.ColorText).Bold(true)
	modeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#2CB67D")).Bold(true)

	fmt.Printf("  %s %s\n", hintStyle.Render("Mode:"), modeStyle.Render("Receive (QR Upload)"))
	fmt.Printf("  %s %s\n", hintStyle.Render("URL:"), urlStyle.Render(qrURL))
	if ipv6URL != "" && ipv4URL != "" {
		fmt.Printf("  %s %s\n", hintStyle.Render("IPv4:"), urlStyle.Render(ipv4URL))
	}
	fmt.Printf("  %s %s\n", hintStyle.Render("Save to:"), dirStyle.Render(outputDir))
	if recQRLimit > 0 {
		fmt.Printf("  %s %d allowed\n", hintStyle.Render("Uploads:"), recQRLimit)
	}
	if recQRExpire > 0 {
		fmt.Printf("  %s %s\n", hintStyle.Render("Expires:"), recQRExpire.String())
	}
	fmt.Println()

	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6b6b")).Bold(true)
	fmt.Println("  " + warnStyle.Render("⚠️  WARNING: Anyone on your local WiFi network with this QR"))
	fmt.Println("  " + warnStyle.Render("             code or URL can upload files directly to your laptop."))
	fmt.Println("  " + warnStyle.Render("             Executables and scripts are renamed to .jend-quarantine"))

	fmt.Println()
	fmt.Println(hintStyle.Render("  Scan the QR code on your phone to upload files. (Ctrl+C to cancel)"))
	fmt.Println()

	if err := srv.Start(ctx); err != nil {
		fmt.Printf("Server error: %v\n", err)
		os.Exit(1)
	}
}

// startCloudQRReceiver starts a WebRTC-based upload receiver for cloud mode.
// It connects to MQTT signaling, generates a QR code pointing to the
// CloudFront-hosted upload page, and receives files streamed via DataChannel.
func startCloudQRReceiver(outputDir string) {
	// Generate a short 6-char alphanumeric transfer code.
	token := generateReceiveTransferCode()

	// Connect to MQTT signaling.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalClient, err := signaling.NewIoTClient(ctx, "jend-qr-recv-"+token)
	if err != nil {
		fmt.Printf("Error connecting to signaling server: %v\n", err)
		fmt.Println("Cloud mode requires internet access for signaling.")
		os.Exit(1)
	}
	defer signalClient.Disconnect()

	logActiveStyle := lipgloss.NewStyle().Foreground(ui.ColorAccent)
	logSuccessStyle := lipgloss.NewStyle().Foreground(ui.ColorSuccess)
	logWarnStyle := lipgloss.NewStyle().Foreground(ui.ColorWarning)
	logErrorStyle := lipgloss.NewStyle().Foreground(ui.ColorError)
	logMutedStyle := lipgloss.NewStyle().Foreground(ui.ColorSubtext)

	// Create WebRTC upload receiver.
	receiver := transport.NewWebRTCUploadReceiver(signalClient, transport.WebRTCUploadReceiverConfig{
		Token:     token,
		OutputDir: outputDir,
		OnConnected: func() {
			fmt.Printf("\r  %s Phone connected via WebRTC\033[K\n", logSuccessStyle.Render("✓"))
		},
		OnFileStart: func(name string, size int64) {
			fmt.Printf("\r  %s %s %s\033[K", logActiveStyle.Render("⬇"), name, logMutedStyle.Render("("+formatBytesReceive(size)+")"))
		},
		OnProgress: func(recv, total int64) {
			if total > 0 {
				pct := float64(recv) / float64(total) * 100
				fmt.Printf("\r  %s %-30s %.0f%%\033[K", logActiveStyle.Render("⬇"), "Receiving...", pct)
			}
		},
		OnApprovalRequired: func(name string, size int64) bool {
			if recAutoApprove {
				return true
			}
			return ui.PromptApproval(name, formatBytesReceive(size))
		},
		OnFileComplete: func(name string, fileCount int) {
			fmt.Printf("\r  %s %s %s\033[K\n", logSuccessStyle.Render("✓"), name, logMutedStyle.Render(fmt.Sprintf("(file #%d)", fileCount)))
			if recQRLimit > 0 && fileCount >= recQRLimit {
				fmt.Printf("\n  %s %s\n", logWarnStyle.Render("!"), logWarnStyle.Render(fmt.Sprintf("Limit of %d files reached. Shutting down...", recQRLimit)))
				cancel()
			} else {
				fmt.Printf("  %s %s\n", logMutedStyle.Render("..."), logMutedStyle.Render("Waiting for more files... (Ctrl+C to stop)"))
			}
		},
		OnTextComplete: func(text string) {
			fmt.Printf("\r  %s Received Text:\033[K\n\n%s\n\n", logSuccessStyle.Render("✓"), ui.CodeStyle.Render(text))
			if !recNoClipboard {
				if err := clipboard.WriteAll(text); err == nil {
					fmt.Printf("  %s\n", logMutedStyle.Render("(Copied to clipboard)"))
				}
			}
			fmt.Printf("  %s %s\n", logMutedStyle.Render("..."), logMutedStyle.Render("Waiting for more... (Ctrl+C to stop)"))
		},
		OnError: func(err error) {
			fmt.Printf("\n  %s %s\n", logErrorStyle.Render("✗"), logErrorStyle.Render(err.Error()))
		},
	})

	// Handle interrupt.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, osutils.ShutdownSignals...)
	go func() {
		<-sigChan
		fmt.Println("\n\nShutting down...")
		cancel()
	}()

	// Generate QR code pointing to the CloudFront upload page.
	cloudURL := fmt.Sprintf("https://d36yyit6n9gsha.cloudfront.net/qr/upload.html#%s", token)

	// Print banner and QR.
	fmt.Println(ui.RenderBanner())
	fmt.Println()
	for _, line := range strings.Split(ui.RenderQR(cloudURL), "\n") {
		fmt.Println("    " + line)
	}
	fmt.Println()

	// Print info below QR.
	hintStyle := lipgloss.NewStyle().Foreground(ui.ColorSubtext).Faint(true)
	urlStyle := lipgloss.NewStyle().Foreground(ui.ColorAccent).Bold(true)
	dirStyle := lipgloss.NewStyle().Foreground(ui.ColorText).Bold(true)
	modeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00F0FF")).Bold(true)
	codeStyle := lipgloss.NewStyle().
		Foreground(ui.ColorText).
		Background(lipgloss.Color("#242629")).
		Bold(true).
		Padding(0, 2).
		MarginLeft(2)

	fmt.Printf("  %s %s\n", hintStyle.Render("Mode:"), modeStyle.Render("Cloud Receive (WebRTC)"))
	fmt.Printf("  %s %s\n", hintStyle.Render("URL:"), urlStyle.Render("d36yyit6n9gsha.cloudfront.net/qr/upload"))
	fmt.Printf("  %s %s\n", hintStyle.Render("Save to:"), dirStyle.Render(outputDir))
	fmt.Println()
	fmt.Printf("  %s\n", hintStyle.Render("No QR scanner? Enter this code at the URL above:"))
	fmt.Println(codeStyle.Render(token))
	fmt.Println()

	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6b6b")).Bold(true)
	fmt.Println("  " + warnStyle.Render("⚠️  WARNING: Anyone with this code/URL can upload files"))
	fmt.Println("  " + warnStyle.Render("             directly to your laptop over the internet."))
	fmt.Println("  " + warnStyle.Render("             Executables and scripts are renamed to .jend-quarantine"))

	fmt.Println()
	fmt.Println(hintStyle.Render("  Waiting for connection... (Ctrl+C to cancel)"))
	fmt.Println()

	// Start the WebRTC receiver (blocks until ctx is cancelled).
	if err := receiver.Run(ctx); err != nil {
		fmt.Printf("WebRTC error: %v\n", err)
	}
}

// generateReceiveTransferCode creates a 6-character alphanumeric code.
// Uses crypto/rand so codes are unpredictable.
func generateReceiveTransferCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789"
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	code := make([]byte, 6)
	for i := range code {
		code[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(code)
}

// formatBytesReceive formats a byte count for display.
func formatBytesReceive(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	suffixes := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), suffixes[exp])
}
