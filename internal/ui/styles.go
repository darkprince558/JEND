package ui

import "github.com/charmbracelet/lipgloss"

// Color Palette
var (
	ColorPrimary   = lipgloss.Color("#7D56F4") // Purple
	ColorSecondary = lipgloss.Color("#9F7AEA") // Lighter Purple
	ColorSuccess   = lipgloss.Color("#38A169") // Green
	ColorError     = lipgloss.Color("#E53E3E") // Red
	ColorText      = lipgloss.Color("#FAFAFA") // White
	ColorSubtext   = lipgloss.Color("#A0AEC0") // Gray
)

// Styles
var (
	TitleStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			Padding(0, 1)

	StatusStyle = lipgloss.NewStyle().
			Foreground(ColorSubtext).
			Italic(true)

	CodeStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Background(lipgloss.Color("#2D3748")).
			Padding(0, 1).
			Bold(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorError).
			Bold(true)

	ContainerStyle = lipgloss.NewStyle().
			Padding(1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Width(60)

	// Matrix / Handshake Styles
	MatrixHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00FF00")). // Bright Green
				Background(lipgloss.Color("#000000")). // Black
				Bold(true).
				Padding(0, 1)

	MatrixTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00CC00")). // Matrix Green
			Background(lipgloss.Color("#000000"))

	// Telemetry Styles
	StatLabelStyle = lipgloss.NewStyle().
			Foreground(ColorSubtext).
			Width(12)

	StatValueStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)
)
