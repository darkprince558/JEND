package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// PromptApproval visually asks the user to physically accept an incoming file.
// Displays the file name, size, and a [Y/n] prompt.
func PromptApproval(name, sizeStr string) bool {
	// Simple standard Go prompt to avoid complex TUI disruption
	// since the server is actively running in the background.

	promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E2B93B")).Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(ColorSubtext).Faint(true)

	msg := fmt.Sprintf("\n  %s %s %s",
		promptStyle.Render("[?]"),
		"Incoming file:",
		nameStyle.Render(name),
	)
	if sizeStr != "" {
		msg += dimStyle.Render(fmt.Sprintf(" (%s)", sizeStr))
	}
	msg += "  Accept? [Y/n]: "

	fmt.Print(msg)

	var response string
	fmt.Scanln(&response)

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "" || response == "y" || response == "yes"
}
