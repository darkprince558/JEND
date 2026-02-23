package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/darkprince558/jend/internal/config"
	"github.com/darkprince558/jend/internal/core"
	"github.com/darkprince558/jend/internal/signaling"
	"github.com/darkprince558/jend/internal/transport"
	"github.com/darkprince558/jend/internal/ui"
	petname "github.com/dustinkirkland/golang-petname"
	"github.com/spf13/cobra"
)

var (
	// Flags for Send
	forceTar        bool
	forceZip        bool
	textContent     string
	sendNoHistory   bool
	sendNoClipboard bool
	sendIncognito   bool
	relayURL        string
	relayUser       string
	relayPass       string
	flagPort        string
	useS3           bool
	useQR           bool
	qrLimit         int
	qrExpire        time.Duration
	qrMode          string
)

var sendCmd = &cobra.Command{
	Use:   "send [file] [file2 ...]",
	Short: "Send a file, directory, or text snippet",
	Long: `Generate a secure code to send a file or text snippet to another device.
You can pass multiple files or directories and JEND will bundle them into a zip.
Piping from stdin is also supported.
Example:
  jend send my_file.txt
  jend send file1.txt file2.txt image.png
  cat notes.txt | jend send
  jend send --text "Hello world"
  jend send --incognito secret.txt
  jend send --relay-url "turn:my.relay.click:3478" --relay-user foo --relay-pass bar`,
	Args: func(cmd *cobra.Command, args []string) error {
		// Allow no args — file picker will handle it in interactive mode
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		isText := false
		filePath := ""

		// Check if data is being piped in via stdin
		if textContent == "" {
			if stdinInfo, err := os.Stdin.Stat(); err == nil {
				if (stdinInfo.Mode() & os.ModeCharDevice) == 0 {
					// stdin is a pipe or redirect
					data, err := io.ReadAll(os.Stdin)
					if err != nil {
						fmt.Printf("Error reading from stdin: %v\n", err)
						os.Exit(1)
					}
					if len(data) > 0 {
						textContent = string(data)
						isText = true
					}
				}
			}
		}

		ranWizard := false
		if textContent != "" {
			isText = true
		} else if len(args) > 1 {
			// Multiple files — bundle into a temp zip
			fmt.Printf("Bundling %d items into a zip archive...\n", len(args))
			bundlePath, err := core.BundleFiles(args)
			if err != nil {
				fmt.Printf("Error creating bundle: %v\n", err)
				os.Exit(1)
			}
			defer func() { _ = os.Remove(bundlePath) }()
			filePath = bundlePath
		} else if len(args) == 1 {
			filePath = args[0]
		} else {
			// No file arg — launch interactive wizard (unless headless)
			if headless {
				fmt.Println("Error: file path required in headless mode (use --text for text)")
				os.Exit(1)
			}
			wizResult, err := ui.RunSendWizard()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			if wizResult.Cancelled {
				fmt.Println("Cancelled.")
				os.Exit(0)
			}

			// Map wizard results to send params
			ranWizard = true
			filePath = wizResult.FilePath
			textContent = wizResult.TextContent
			isText = wizResult.IsText
			useS3 = wizResult.UseS3
			useQR = wizResult.UseQR
			qrMode = wizResult.QRMode
			qrLimit = wizResult.QRLimit
			qrExpire = wizResult.QRExpire
			forceZip = wizResult.ForceZip
			forceTar = wizResult.ForceTar
			if wizResult.Incognito {
				sendIncognito = true
			}
			if wizResult.NoClipboard {
				sendNoClipboard = true
			}
			if wizResult.NoHistory {
				sendNoHistory = true
			}
		}

		// Handle Incognito
		if sendIncognito {
			sendNoHistory = true
			sendNoClipboard = true
		}

		// Handle Relay Config
		// 1. Try Config File
		cfg, _ := config.Load()

		// 2. Override with Config File if flags missing
		if relayURL == "" && cfg.RelayURL != "" {
			relayURL = cfg.RelayURL
			relayUser = cfg.RelayUser
			relayPass = cfg.RelayPass
		}

		var turnCfg *transport.CustomTurnConfig
		if relayURL != "" {
			turnCfg = &transport.CustomTurnConfig{
				URL:      relayURL,
				Username: relayUser,
				Password: relayPass,
			}
		}

		timeout := getTimeout()
		if flagPort == "" {
			flagPort = "9000"
		}

		// QR Mode: start local HTTP server instead of P2P sender
		if useQR {
			// Show interactive prompt if no explicit flags were set AND we didn't just use the full wizard
			if !ranWizard && !cmd.Flags().Changed("qr-limit") && !cmd.Flags().Changed("qr-expire") && !cmd.Flags().Changed("qr-mode") && !headless {
				opts, err := ui.RunQRPrompt()
				if err != nil || opts.Cancelled {
					fmt.Println("Cancelled.")
					return
				}
				qrLimit = opts.MaxDownloads
				qrExpire = opts.ExpireAfter
				qrMode = opts.Mode
			}

			if qrMode == "cloud" {
				startCloudQRSender(filePath, textContent, isText, forceTar, forceZip)
			} else {
				startQRSender(filePath, textContent, isText, forceTar, forceZip)
			}
			return
		}

		startSender(filePath, textContent, isText, headless, flagPort, timeout, forceTar, forceZip, sendNoHistory, sendNoClipboard, turnCfg, useS3)
	},
}

func init() {
	rootCmd.AddCommand(sendCmd)

	sendCmd.Flags().BoolVar(&forceTar, "tar", false, "Force tar.gz compression")
	sendCmd.Flags().BoolVar(&forceZip, "zip", false, "Force zip compression")
	sendCmd.Flags().StringVar(&textContent, "text", "", "Send text content directly")
	sendCmd.Flags().BoolVar(&sendNoHistory, "no-history", false, "Disable audit logging")
	sendCmd.Flags().BoolVar(&sendNoClipboard, "no-clipboard", false, "Disable clipboard copy of code")
	sendCmd.Flags().BoolVar(&sendIncognito, "incognito", false, "Enable incognito mode (no history, no clipboard)")

	// Relay Flags
	sendCmd.Flags().StringVar(&relayURL, "relay-url", "", "Custom TURN Relay URL (e.g. turn:host:port)")
	sendCmd.Flags().StringVar(&relayUser, "relay-user", "", "TURN Relay Username")
	sendCmd.Flags().StringVar(&relayPass, "relay-pass", "", "TURN Relay Password")
	sendCmd.Flags().StringVar(&flagPort, "port", "9000", "Port to listen on (default 9000)")
	sendCmd.Flags().BoolVar(&useS3, "s3", false, "Use S3 for file transfer (limit 200MB)")
	sendCmd.Flags().BoolVar(&useQR, "qr", false, "Generate a QR code for browser-based download (no JEND needed on receiver)")
	sendCmd.Flags().IntVar(&qrLimit, "qr-limit", 0, "Max number of QR downloads allowed (0 = unlimited)")
	sendCmd.Flags().DurationVar(&qrExpire, "qr-expire", 0, "Auto-expire the QR server after duration (e.g. 15m, 1h)")
	sendCmd.Flags().StringVar(&qrMode, "qr-mode", "local", "QR transfer mode: local or cloud")
}

// startSender initializes the sender process.
// It generates a code, optionally copies it to clipbaord, and starts the TUI or Headless mode.
func startSender(filePath string, textContent string, isText bool, headless bool, port string, timeout time.Duration, forceTar, forceZip bool, noHistory bool, noClipboard bool, turnCfg *transport.CustomTurnConfig, useS3 bool) {
	// Generate Code (3 words)
	code := petname.Generate(3, "-")

	// Copy to Clipboard
	if !noClipboard {
		_ = clipboard.WriteAll(code) // Ignore error, just try best effort
	}

	// Context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	if headless {
		fmt.Printf("Code: %s\n", code)
		core.RunSender(ctx, nil, ui.RoleSender, filePath, textContent, isText, code, port, timeout, forceTar, forceZip, noHistory, turnCfg, useS3)
	} else {
		// Init UI
		var displayName string
		if isText {
			displayName = "Text Snippet"
		} else {
			displayName = filepath.Base(filePath)
		}
		model := ui.NewModel(ui.RoleSender, displayName, code)
		p := tea.NewProgram(model, tea.WithAltScreen())

		var wg sync.WaitGroup
		wg.Add(1)

		// Transfer Logic
		go func() {
			defer wg.Done()
			core.RunSender(ctx, p, ui.RoleSender, filePath, textContent, isText, code, port, timeout, forceTar, forceZip, noHistory, turnCfg, useS3)
		}()

		if _, err := p.Run(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		// When TUI exits, cancel context to stop sender
		cancel()
		wg.Wait()
	}
}

// startQRSender handles the --qr mode: starts a local HTTP server and prints a QR code.
func startQRSender(filePath string, textContent string, isText bool, forceTar, forceZip bool) {
	var fileName string
	var fileSize int64
	var fileHash string

	if isText {
		fileName = "text-snippet.txt"
		fileSize = int64(len(textContent))
		// Compute hash of text content
		h := sha256.Sum256([]byte(textContent))
		fileHash = hex.EncodeToString(h[:])
	} else {
		// Check if path is a directory → compress first
		info, err := os.Stat(filePath)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		if info.IsDir() || forceTar {
			fmt.Println("Compressing to .tar.gz...")
			tempPath, err := core.CompressPath(filePath, "tar.gz")
			if err != nil {
				fmt.Printf("Error compressing: %v\n", err)
				os.Exit(1)
			}
			defer func() { _ = os.Remove(tempPath) }()
			filePath = tempPath
			fileName = filepath.Base(filePath) + ".tar.gz"
		} else if forceZip {
			fmt.Println("Compressing to .zip...")
			tempPath, err := core.CompressPath(filePath, "zip")
			if err != nil {
				fmt.Printf("Error compressing: %v\n", err)
				os.Exit(1)
			}
			defer func() { _ = os.Remove(tempPath) }()
			filePath = tempPath
			fileName = filepath.Base(filePath) + ".zip"
		} else {
			fileName = info.Name()
		}

		// Re-stat (may have changed due to compression)
		info, err = os.Stat(filePath)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fileSize = info.Size()

		fileHash, err = core.HashFile(filePath)
		if err != nil {
			fmt.Printf("Error hashing file: %v\n", err)
			os.Exit(1)
		}
	}

	// Context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Create server
	srv := core.NewQRServer(core.QRServerConfig{
		FilePath:     filePath,
		FileName:     fileName,
		FileSize:     fileSize,
		FileHash:     fileHash,
		IsText:       isText,
		TextContent:  textContent,
		Port:         8888,
		MaxDownloads: qrLimit,
		ExpireAfter:  qrExpire,
		OnProgress: func(sent, total int64) {
			pct := float64(sent) / float64(total) * 100
			fmt.Printf("\r  Sending... %.0f%%", pct)
		},
		OnComplete: func(downloadCount int) {
			fmt.Printf("\n\n  Download #%d complete\n", downloadCount)
			if qrLimit > 0 {
				remaining := qrLimit - downloadCount
				if remaining > 0 {
					fmt.Printf("  %d/%d downloads used. Ctrl+C to stop.\n", downloadCount, qrLimit)
				}
			} else {
				fmt.Println("  Waiting for more downloads... (Ctrl+C to stop)")
			}
		},
		OnLimitReached: func() {
			fmt.Printf("\n  Download limit (%d) reached. Shutting down...\n", qrLimit)
			go func() {
				time.Sleep(2 * time.Second)
				cancel()
			}()
		},
		OnExpire: func() {
			fmt.Printf("\n  QR code expired after %s. Shutting down...\n", qrExpire)
		},
	})

	ipv4URL, ipv6URL := srv.URLs()

	// Use IPv4 for QR code (Safari/mobile browsers don't support IPv6 link-local URLs)
	qrURL := ipv4URL

	// Print banner and QR
	fmt.Println(ui.RenderBanner())
	fmt.Println()
	// Indent QR code by 4 spaces to align with banner/text
	for _, line := range strings.Split(ui.RenderQR(qrURL), "\n") {
		fmt.Println("    " + line)
	}
	fmt.Println()

	// Print info below QR
	hintStyle := lipgloss.NewStyle().Foreground(ui.ColorSubtext).Faint(true)
	urlStyle := lipgloss.NewStyle().Foreground(ui.ColorAccent).Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(ui.ColorText).Bold(true)
	sizeStyle := lipgloss.NewStyle().Foreground(ui.ColorSubtext)

	fmt.Printf("  %s %s\n", hintStyle.Render("URL:"), urlStyle.Render(qrURL))
	if ipv6URL != "" && ipv4URL != "" {
		fmt.Printf("  %s %s\n", hintStyle.Render("IPv4:"), urlStyle.Render(ipv4URL))
	}
	fmt.Printf("  %s %s\n", hintStyle.Render("File:"), nameStyle.Render(fileName)+" "+sizeStyle.Render("("+formatBytesLocal(fileSize)+")"))
	if qrLimit > 0 {
		fmt.Printf("  %s %s\n", hintStyle.Render("Downloads:"), sizeStyle.Render(fmt.Sprintf("%d allowed", qrLimit)))
	}
	if qrExpire > 0 {
		fmt.Printf("  %s %s\n", hintStyle.Render("Expires:"), sizeStyle.Render(qrExpire.String()))
	}
	fmt.Println()
	fmt.Println(hintStyle.Render("  Waiting for download... (Ctrl+C to cancel)"))
	fmt.Println()

	if err := srv.Start(ctx); err != nil {
		fmt.Printf("Server error: %v\n", err)
		os.Exit(1)
	}
}

// startCloudQRSender handles the --qr --qr-mode=cloud mode.
// It connects to MQTT for signaling, starts a WebRTC engine, and prints a QR code
// pointing to the public web app at jend.app/qr#token.
func startCloudQRSender(filePath string, textContent string, isText bool, forceTar, forceZip bool) {
	var fileName string
	var fileSize int64
	var fileHash string

	if isText {
		fileName = "text-snippet.txt"
		fileSize = int64(len(textContent))
		h := sha256.Sum256([]byte(textContent))
		fileHash = hex.EncodeToString(h[:])
	} else {
		info, err := os.Stat(filePath)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		if info.IsDir() || forceTar {
			fmt.Println("Compressing to .tar.gz...")
			tempPath, err := core.CompressPath(filePath, "tar.gz")
			if err != nil {
				fmt.Printf("Error compressing: %v\n", err)
				os.Exit(1)
			}
			defer func() { _ = os.Remove(tempPath) }()
			filePath = tempPath
			fileName = filepath.Base(filePath) + ".tar.gz"
		} else if forceZip {
			fmt.Println("Compressing to .zip...")
			tempPath, err := core.CompressPath(filePath, "zip")
			if err != nil {
				fmt.Printf("Error compressing: %v\n", err)
				os.Exit(1)
			}
			defer func() { _ = os.Remove(tempPath) }()
			filePath = tempPath
			fileName = filepath.Base(filePath) + ".zip"
		} else {
			fileName = info.Name()
		}

		info, err = os.Stat(filePath)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fileSize = info.Size()

		fileHash, err = core.HashFile(filePath)
		if err != nil {
			fmt.Printf("Error hashing file: %v\n", err)
			os.Exit(1)
		}
	}

	// Generate a short 6-char alphanumeric transfer code (e.g. Af38HJ)
	token := generateTransferCode()

	// Connect to MQTT signaling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalClient, err := signaling.NewIoTClient(ctx, "jend-qr-"+token)
	if err != nil {
		fmt.Printf("Error connecting to signaling server: %v\n", err)
		fmt.Println("Cloud mode requires internet access for signaling.")
		os.Exit(1)
	}
	defer signalClient.Disconnect()

	// Create WebRTC sender
	sender := transport.NewWebRTCSender(signalClient, transport.WebRTCSenderConfig{
		Token:       token,
		FilePath:    filePath,
		FileName:    fileName,
		FileSize:    fileSize,
		FileHash:    fileHash,
		IsText:      isText,
		TextContent: textContent,
		OnProgress: func(sent, total int64) {
			pct := float64(sent) / float64(total) * 100
			fmt.Printf("\r  Sending... %.0f%%", pct)
		},
		OnComplete: func(downloadCount int) {
			fmt.Printf("\n\n  Download #%d complete\n", downloadCount)
			fmt.Println("  Waiting for more downloads... (Ctrl+C to stop)")
		},
	})

	// Handle interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n\nShutting down...")
		cancel()
	}()

	// Generate QR code pointing to the public web app
	cloudURL := fmt.Sprintf("https://d36yyit6n9gsha.cloudfront.net/qr/index.html#%s", token)

	// Print banner and QR
	fmt.Println(ui.RenderBanner())
	fmt.Println()
	for _, line := range strings.Split(ui.RenderQR(cloudURL), "\n") {
		fmt.Println("    " + line)
	}
	fmt.Println()

	hintStyle := lipgloss.NewStyle().Foreground(ui.ColorSubtext).Faint(true)
	urlStyle := lipgloss.NewStyle().Foreground(ui.ColorAccent).Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(ui.ColorText).Bold(true)
	sizeStyle := lipgloss.NewStyle().Foreground(ui.ColorSubtext)
	modeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00F0FF")).Bold(true)
	codeStyle := lipgloss.NewStyle().
		Foreground(ui.ColorText).
		Background(lipgloss.Color("#242629")).
		Bold(true).
		Padding(0, 2).
		MarginLeft(2)

	fmt.Printf("  %s %s\n", hintStyle.Render("Mode:"), modeStyle.Render("Cloud (WebRTC)"))
	fmt.Printf("  %s %s\n", hintStyle.Render("URL:"), urlStyle.Render("d36yyit6n9gsha.cloudfront.net/qr"))
	fmt.Printf("  %s %s\n", hintStyle.Render("File:"), nameStyle.Render(fileName)+" "+sizeStyle.Render("("+formatBytesLocal(fileSize)+")"))
	fmt.Printf("  %s %s\n", hintStyle.Render("SHA-256:"), sizeStyle.Render(fileHash[:16]+"..."))
	fmt.Println()
	fmt.Printf("  %s\n", hintStyle.Render("No QR scanner? Enter this code at the URL above:"))
	fmt.Println(codeStyle.Render(token))
	fmt.Println()
	fmt.Println(hintStyle.Render("  Waiting for download... (Ctrl+C to cancel)"))
	fmt.Println()

	// Start the WebRTC sender (blocks until ctx is cancelled)
	if err := sender.Run(ctx); err != nil {
		fmt.Printf("WebRTC error: %v\n", err)
	}
}

func formatBytesLocal(b int64) string {
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

// generateTransferCode creates a 6-character alphanumeric code (e.g. "Af38HJ").
// Uses crypto/rand so codes are unpredictable.
func generateTransferCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789"
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	code := make([]byte, 6)
	for i := range code {
		code[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(code)
}
