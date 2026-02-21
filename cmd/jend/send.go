package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/darkprince558/jend/internal/config"
	"github.com/darkprince558/jend/internal/core"
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
)

var sendCmd = &cobra.Command{
	Use:   "send [file]",
	Short: "Send a file, directory, or text snippet",
	Long: `Generate a secure code to send a file or text snippet to another device.
Example:
  jend send my_file.txt
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

		if textContent != "" {
			isText = true
		} else if len(args) > 0 {
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
			filePath = wizResult.FilePath
			textContent = wizResult.TextContent
			isText = wizResult.IsText
			useS3 = wizResult.UseS3
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
			startQRSender(filePath, textContent, isText, forceTar, forceZip)
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
	var err error

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

	// Get local IP
	localIP, err := core.GetLocalIP()
	if err != nil {
		fmt.Printf("Error detecting local IP: %v\n", err)
		os.Exit(1)
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
		FilePath:    filePath,
		FileName:    fileName,
		FileSize:    fileSize,
		FileHash:    fileHash,
		IsText:      isText,
		TextContent: textContent,
		Port:        8888,
		OnProgress: func(sent, total int64) {
			pct := float64(sent) / float64(total) * 100
			fmt.Printf("\r  Sending... %.0f%%", pct)
		},
		OnComplete: func() {
			fmt.Println("\n\n  ✅ Download complete! File sent successfully.")
			// Give a moment for the HTTP response to flush, then shut down
			go func() {
				time.Sleep(2 * time.Second)
				cancel()
			}()
		},
	})

	downloadURL := srv.URL(localIP)

	// Print banner and QR
	fmt.Println(ui.RenderBanner())
	fmt.Println()
	fmt.Println(ui.RenderQR(downloadURL))
	fmt.Println()

	// Print info below QR
	hintStyle := lipgloss.NewStyle().Foreground(ui.ColorSubtext).Faint(true)
	urlStyle := lipgloss.NewStyle().Foreground(ui.ColorAccent).Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(ui.ColorText).Bold(true)
	sizeStyle := lipgloss.NewStyle().Foreground(ui.ColorSubtext)

	fmt.Printf("  %s %s\n", hintStyle.Render("Scan to download:"), urlStyle.Render(downloadURL))
	fmt.Printf("  %s %s\n", hintStyle.Render("File:"), nameStyle.Render(fileName)+" "+sizeStyle.Render("("+formatBytesLocal(fileSize)+")"))
	fmt.Println()
	fmt.Println(hintStyle.Render("  Waiting for download... (Ctrl+C to cancel)"))
	fmt.Println()

	if err := srv.Start(ctx); err != nil {
		fmt.Printf("Server error: %v\n", err)
		os.Exit(1)
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
