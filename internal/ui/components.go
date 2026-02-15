package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// ViewCode renders the code display block
func ViewCode(code string) string {
	label := lipgloss.NewStyle().
		Foreground(ColorSubtext).
		Italic(true).
		Render("Share this code with the receiver:")

	codeBox := lipgloss.NewStyle().
		Foreground(ColorAccent).
		Background(ColorPanel).
		Padding(1, 6).
		Bold(true).
		Align(lipgloss.Center).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Render(code)

	return lipgloss.JoinVertical(lipgloss.Center,
		label,
		"",
		codeBox,
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
