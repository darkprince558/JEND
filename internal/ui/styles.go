package ui

import "github.com/charmbracelet/lipgloss"

// Color Palette (Modern/Neon)
var (
	ColorPrimary   = lipgloss.Color("#8A2BE2") // Vibrant BlueViolet
	ColorSecondary = lipgloss.Color("#00FFFF") // Cyan/Aqua for accents
	ColorSuccess   = lipgloss.Color("#00FF00") // Neon Green
	ColorError     = lipgloss.Color("#FF0055") // Neon Red/Pink
	ColorWarning   = lipgloss.Color("#FFFF00") // Neon Yellow
	ColorText      = lipgloss.Color("#FFFFFF") // Pure White
	ColorSubtext   = lipgloss.Color("#A0AEC0") // Cool Gray
	ColorBg        = lipgloss.Color("#1A202C") // Dark Blue-Gray Background
)

// Styles
var (
	TitleStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			Padding(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary)

	StatusStyle = lipgloss.NewStyle().
			Foreground(ColorSubtext).
			Italic(true)

	CodeStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Background(lipgloss.Color("#2D3748")).
			Padding(1, 2).
			Margin(1, 0).
			Bold(true).
			Align(lipgloss.Center).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorSecondary)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorError).
			Bold(true).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(ColorError).
			Padding(1)

	ContainerStyle = lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Width(65).
			Align(lipgloss.Center)

	// Matrix / Handshake Styles (Cyberpunk Theme)
	MatrixHeaderStyle = lipgloss.NewStyle().
				Foreground(ColorSuccess).
				Bold(true).
				Padding(0, 1).
				Border(lipgloss.NormalBorder(), false, false, true, false). // Bottom border only
				BorderForeground(ColorSuccess)

	MatrixTextStyle = lipgloss.NewStyle().
			Foreground(ColorSuccess).
			Background(lipgloss.Color("#000000")).
			Padding(0, 1)

	// Telemetry Styles
	StatLabelStyle = lipgloss.NewStyle().
			Foreground(ColorSubtext).
			Width(12).
			Align(lipgloss.Right).
			PaddingRight(1)

	StatValueStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true).
			Width(20).
			Align(lipgloss.Left)
)
