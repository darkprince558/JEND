package ui

import "github.com/charmbracelet/lipgloss"

// Color Palette (Minimal & Cool)
var (
	ColorPrimary   = lipgloss.Color("#7F5AF0") // Soft Purple
	ColorSecondary = lipgloss.Color("#2CB67D") // Muted Green
	ColorSuccess   = lipgloss.Color("#2CB67D") // Alias for backward compatibility
	ColorAccent    = lipgloss.Color("#00F0FF") // Cyan Accent
	ColorError     = lipgloss.Color("#EF4565") // Soft Red
	ColorWarning   = lipgloss.Color("#F9C74E") // Muted Yellow
	ColorText      = lipgloss.Color("#FFFFFE") // Off-White
	ColorSubtext   = lipgloss.Color("#94A1B2") // Blue-Gray
	ColorBg        = lipgloss.Color("#16161A") // Dark Background
	ColorPanel     = lipgloss.Color("#242629") // Panel Background
)

// Styles - Borderless & Minimal
var (
	TitleStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			Padding(1, 0, 0, 0).
			MarginBottom(1)

	StatusStyle = lipgloss.NewStyle().
			Foreground(ColorSubtext).
			Italic(true).
			MarginTop(1)

	SubTextStyle = lipgloss.NewStyle().
			Foreground(ColorSubtext).
			Italic(true)

	CodeStyle = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Background(ColorPanel).
			Padding(1, 4).
			Margin(1, 0).
			Bold(true).
			Align(lipgloss.Center)
		// No Border

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorError).
			Bold(true).
			Padding(0, 1).
			MarginBottom(1)

	WarningStyle = lipgloss.NewStyle().
			Foreground(ColorWarning).
			Bold(true).
			Padding(0, 1).
			MarginBottom(1)

	InfoStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true).
			Padding(0, 1).
			MarginBottom(1)

	ContainerStyle = lipgloss.NewStyle().
			Padding(2, 4).
			Width(80).
			Align(lipgloss.Center)

	// Telemetry Styles
	StatLabelStyle = lipgloss.NewStyle().
			Foreground(ColorSubtext).
			Width(10).
			Align(lipgloss.Right).
			PaddingRight(1)

	StatValueStyle = lipgloss.NewStyle().
			Foreground(ColorText).
			Bold(true).
			Width(18).
			Align(lipgloss.Left)

	// Wizard Styles
	WizardHeaderStyle = lipgloss.NewStyle().
				Foreground(ColorPrimary).
				Bold(true).
				MarginBottom(1)

	RadioActiveStyle = lipgloss.NewStyle().
				Foreground(ColorAccent).
				Bold(true)

	RadioInactiveStyle = lipgloss.NewStyle().
				Foreground(ColorSubtext)

	ToggleOnStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)

	ToggleOffStyle = lipgloss.NewStyle().
			Foreground(ColorSubtext)

	StepDotActive = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)

	StepDotInactive = lipgloss.NewStyle().
			Foreground(ColorSubtext)

	ConfirmLabelStyle = lipgloss.NewStyle().
				Foreground(ColorSubtext).
				Width(12).
				Align(lipgloss.Right).
				PaddingRight(2)

	ConfirmValueStyle = lipgloss.NewStyle().
				Foreground(ColorText).
				Bold(true)

	ConfirmCardStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPanel).
				Padding(1, 3).
				MarginTop(1).
				MarginBottom(1)

	WizardHelpStyle = lipgloss.NewStyle().
			Foreground(ColorSubtext).
			Faint(true).
			MarginTop(1)
)
