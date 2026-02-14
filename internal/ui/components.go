package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// ViewCode renders the code display block
func ViewCode(code string) string {
	return lipgloss.JoinVertical(lipgloss.Center,
		"Share this code with the receiver (copied to clipboard): ",
		CodeStyle.Render(code),
	)
}

// FormatBytes returns a human-readable byte string (e.g., "4.2 MB").
func FormatBytes(b int64) string {
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
