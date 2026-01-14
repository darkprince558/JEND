package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ViewCode renders the code display block
func ViewCode(code string) string {
	return lipgloss.JoinVertical(lipgloss.Center,
		"Share this code with the receiver (copied to clipboard): ",
		CodeStyle.Render(code),
	)
}

// ViewProgress renders a simple progress bar
func ViewProgress(percent float64, width int) string {
	barWidth := width - 10
	filled := int(float64(barWidth) * percent)
	empty := barWidth - filled

	// Clamp values
	if filled < 0 {
		filled = 0
	}
	if empty < 0 {
		empty = 0
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	return fmt.Sprintf("%s %3.0f%%", bar, percent*100)
}
