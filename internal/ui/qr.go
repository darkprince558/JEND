package ui

import (
	"bytes"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mdp/qrterminal/v3"
)

// RenderQR generates a terminal-friendly QR code string for the given URL,
// styled with JEND's purple theme.
func RenderQR(url string) string {
	var buf bytes.Buffer
	qrterminal.GenerateWithConfig(url, qrterminal.Config{
		Level:     qrterminal.L,
		Writer:    &buf,
		BlackChar: qrterminal.BLACK,
		WhiteChar: qrterminal.WHITE,
		QuietZone: 2,
	})

	// Apply purple coloring to the QR blocks
	qrStyle := lipgloss.NewStyle().Foreground(ColorPrimary)
	lines := strings.Split(buf.String(), "\n")
	var styled []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			styled = append(styled, qrStyle.Render(line))
		}
	}

	return strings.Join(styled, "\n")
}
