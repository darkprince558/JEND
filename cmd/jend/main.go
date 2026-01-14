package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/darkprince558/jend/internal/audit"
	"github.com/darkprince558/jend/internal/core"
	"github.com/darkprince558/jend/internal/ui"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	petname "github.com/dustinkirkland/golang-petname"
)

func main() {
	headless := false
	timeoutStr := "10m"
	outputDir := "."
	forceTar := false
	forceZip := false
	autoUnzip := false
	var args []string

	// Poor man's flag parsing to avoid rearranging all args
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--headless" {
			headless = true
		} else if arg == "--tar" {
			forceTar = true
		} else if arg == "--zip" {
			forceZip = true
		} else if arg == "--unzip" {
			autoUnzip = true
		} else if arg == "--timeout" {
			if i+1 < len(os.Args) {
				timeoutStr = os.Args[i+1]
				i++ // Skip value
			} else {
				fmt.Println("Error: --timeout requires a duration (e.g. 10m)")
				os.Exit(1)
			}
		} else if arg == "--dir" {
			if i+1 < len(os.Args) {
				outputDir = os.Args[i+1]
				i++ // Skip value
			} else {
				fmt.Println("Error: --dir requires a path")
				os.Exit(1)
			}
		} else {
			args = append(args, arg)
		}
	}

	if len(args) < 1 { // args doesn't include program name in this slice construction
		fmt.Println("Usage: jend [flags] <send|receive|history> [args]")
		os.Exit(1)
	}

	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		fmt.Printf("Invalid timeout format: %v\n", err)
		os.Exit(1)
	}

	command := args[0]
	switch command {
	case "send":
		if len(args) < 2 {
			fmt.Println("Usage: jend send <file>")
			os.Exit(1)
		}
		startSender(args[1], headless, timeout, forceTar, forceZip)
	case "receive":
		if len(args) < 2 {
			fmt.Println("Usage: jend receive <code>")
			os.Exit(1)
		}
		startReceiver(args[1], headless, outputDir, autoUnzip)
	case "history":
		audit.ShowHistory()
	default:
		fmt.Println("Unknown command:", command)
		os.Exit(1)
	}
}

func startSender(filePath string, headless bool, timeout time.Duration, forceTar, forceZip bool) {
	// Generate Code (3 words)
	code := petname.Generate(3, "-")

	// Copy to Clipboard
	clipboard.WriteAll(code) // Ignore error, just try best effort

	if headless {
		fmt.Printf("Code: %s\n", code)
		core.RunSender(nil, ui.RoleSender, filePath, code, timeout, forceTar, forceZip)
	} else {
		// Init UI
		model := ui.NewModel(ui.RoleSender, filepath.Base(filePath), code)
		p := tea.NewProgram(model)

		// Transfer Logic
		go core.RunSender(p, ui.RoleSender, filePath, code, timeout, forceTar, forceZip)

		if _, err := p.Run(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	}
}

func startReceiver(code string, headless bool, outputDir string, autoUnzip bool) {
	if headless {
		core.RunReceiver(nil, code, outputDir, autoUnzip)
	} else {
		model := ui.NewModel(ui.RoleReceiver, "", code)
		p := tea.NewProgram(model)

		// Transfer Logic
		go core.RunReceiver(p, code, outputDir, autoUnzip)

		if _, err := p.Run(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	}
}
