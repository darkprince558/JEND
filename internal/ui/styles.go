package ui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ── Color Palette ─────────────────────────────────────────────────────────────
// All vars are mutable so InitTheme() can swap them at startup without
// touching any other files.

var (
	ColorPrimary   = lipgloss.Color("#7F5AF0") // Soft Purple
	ColorSecondary = lipgloss.Color("#2CB67D") // Muted Green
	ColorSuccess   = lipgloss.Color("#2CB67D") // Alias
	ColorAccent    = lipgloss.Color("#00F0FF") // Cyan Accent
	ColorError     = lipgloss.Color("#EF4565") // Soft Red
	ColorWarning   = lipgloss.Color("#F9C74E") // Muted Yellow
	ColorText      = lipgloss.Color("#FFFFFE") // Off-White
	ColorSubtext   = lipgloss.Color("#94A1B2") // Blue-Gray
	ColorBg        = lipgloss.Color("#16161A") // Dark Background
	ColorPanel     = lipgloss.Color("#242629") // Panel Background
)

// ── Theme Palettes ────────────────────────────────────────────────────────────

type themePalette struct {
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Accent    lipgloss.Color
	Error     lipgloss.Color
	Warning   lipgloss.Color
	Text      lipgloss.Color
	Subtext   lipgloss.Color
	Bg        lipgloss.Color
	Panel     lipgloss.Color
}

var darkPalette = themePalette{
	Primary:   "#7F5AF0",
	Secondary: "#2CB67D",
	Accent:    "#00F0FF",
	Error:     "#EF4565",
	Warning:   "#F9C74E",
	Text:      "#FFFFFE",
	Subtext:   "#94A1B2",
	Bg:        "#16161A",
	Panel:     "#242629",
}

var lightPalette = themePalette{
	Primary:   "#5A32D9", // Deeper purple — readable on white
	Secondary: "#1A7D57", // Darker green
	Accent:    "#007ACC", // Dark cyan/blue
	Error:     "#C62045", // Dark red
	Warning:   "#B8860B", // Dark gold
	Text:      "#16161A", // Near-black text
	Subtext:   "#5C6370", // Medium gray
	Bg:        "#F5F5F7", // Light gray bg (like macOS)
	Panel:     "#E0E0E6", // Slightly darker panel
}

// isDark is set by InitTheme and exported for other packages if needed.
var isDark = true

// IsDarkTheme returns whether the dark theme is active.
func IsDarkTheme() bool { return isDark }

// InitTheme applies the appropriate color palette and rebuilds all styles.
// Call this once at program startup (before any rendering).
func InitTheme(dark bool) {
	isDark = dark
	p := darkPalette
	if !dark {
		p = lightPalette
	}

	ColorPrimary = p.Primary
	ColorSecondary = p.Secondary
	ColorSuccess = p.Secondary
	ColorAccent = p.Accent
	ColorError = p.Error
	ColorWarning = p.Warning
	ColorText = p.Text
	ColorSubtext = p.Subtext
	ColorBg = p.Bg
	ColorPanel = p.Panel

	// Rebuild all styles that depend on colors.
	rebuildStyles()
}

// DetectTheme checks the terminal background and returns true if dark.
// It respects the JEND_THEME env var ("dark"/"light") as an escape hatch.
func DetectTheme() bool {
	if v := strings.ToLower(os.Getenv("JEND_THEME")); v != "" {
		return v != "light"
	}
	return lipgloss.HasDarkBackground()
}

// rebuildStyles recreates all package-level style vars using the current Color* vars.
// This must be called after changing any Color* var.
func rebuildStyles() {
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

	WizardHeaderStyle = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true).
		MarginBottom(1).
		Italic(true)

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
}

// ── Style Vars ────────────────────────────────────────────────────────────────
// These must be var (not const) so rebuildStyles() can overwrite them.

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

	WizardHeaderStyle = lipgloss.NewStyle().
				Foreground(ColorPrimary).
				Bold(true).
				MarginBottom(1).
				Italic(true)

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
