package ui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Regex for extracting URLs
var urlRegex = regexp.MustCompile(`(https?://[^\s]+)`)

// FormatTextWithLinks takes raw content and returns a string with ANSI escape codes
// that make valid URLs clickable (OSC 8) and styled (Purple/Underline).
func FormatTextWithLinks(content string) string {
	// Find all matches
	matches := urlRegex.FindAllStringIndex(content, -1)
	if len(matches) == 0 {
		return content
	}

	var sb strings.Builder
	lastIndex := 0

	linkStyle := lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Underline(true)

	for _, match := range matches {
		start, end := match[0], match[1]

		// Append text before the match
		sb.WriteString(content[lastIndex:start])

		// Extract the URL
		url := content[start:end]

		// Format as OSC 8 Hyperlink with styling
		// OSC 8: \x1b]8;;{url}\x1b\\{text}\x1b]8;;\x1b\\
		// We also apply lipgloss style to the text part for visual indication
		styledText := linkStyle.Render(url)
		hyperlink := fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", url, styledText)

		sb.WriteString(hyperlink)

		lastIndex = end
	}

	// Append remaining text
	sb.WriteString(content[lastIndex:])

	return sb.String()
}
