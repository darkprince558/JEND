package ui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ── Color Palette ─────────────────────────────────────────────────────────────
// All vars are mutable so InitTheme() can swap them at startup.

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

// ThemePalette holds a complete set of colors for a theme.
type ThemePalette struct {
	Primary   string
	Secondary string
	Accent    string
	Error     string
	Warning   string
	Text      string
	Subtext   string
	Bg        string
	Panel     string
}

// NamedThemes is the registry of built-in themes. Exported so the config
// command can enumerate valid names.
var NamedThemes = map[string]ThemePalette{
	"dark": {
		Primary: "#7F5AF0", Secondary: "#2CB67D", Accent: "#00F0FF",
		Error: "#EF4565", Warning: "#F9C74E",
		Text: "#FFFFFE", Subtext: "#94A1B2",
		Bg: "#16161A", Panel: "#242629",
	},
	"light": {
		Primary: "#5A32D9", Secondary: "#1A7D57", Accent: "#007ACC",
		Error: "#C62045", Warning: "#B8860B",
		Text: "#16161A", Subtext: "#5C6370",
		Bg: "#F5F5F7", Panel: "#E0E0E6",
	},
	"dracula": {
		Primary: "#BD93F9", Secondary: "#50FA7B", Accent: "#8BE9FD",
		Error: "#FF5555", Warning: "#F1FA8C",
		Text: "#F8F8F2", Subtext: "#6272A4",
		Bg: "#282A36", Panel: "#44475A",
	},
	"nord": {
		Primary: "#88C0D0", Secondary: "#A3BE8C", Accent: "#81A1C1",
		Error: "#BF616A", Warning: "#EBCB8B",
		Text: "#ECEFF4", Subtext: "#D8DEE9",
		Bg: "#2E3440", Panel: "#3B4252",
	},
	"catppuccin": {
		Primary: "#CBA6F7", Secondary: "#A6E3A1", Accent: "#89DCEB",
		Error: "#F38BA8", Warning: "#F9E2AF",
		Text: "#CDD6F4", Subtext: "#A6ADC8",
		Bg: "#1E1E2E", Panel: "#313244",
	},
	"solarized": {
		Primary: "#268BD2", Secondary: "#859900", Accent: "#2AA198",
		Error: "#DC322F", Warning: "#B58900",
		Text: "#FDF6E3", Subtext: "#93A1A1",
		Bg: "#002B36", Panel: "#073642",
	},
}

// isDark is set by InitTheme.
var isDark = true

// IsDarkTheme returns whether the current theme has a dark background.
func IsDarkTheme() bool { return isDark }

// InitTheme applies a named palette and rebuilds all styles.
// themeArg can be "auto", "dark", "light", or any named theme key.
// colorOverrides is an optional map of per-key hex color overrides
// (keys: primary, secondary, accent, error, warning, text, subtext, bg, panel).
func InitTheme(themeArg string, colorOverrides map[string]string) {
	// ── Standard: respect NO_COLOR  ──────────────────────────────────────
	// https://no-color.org — when set, tools should disable ANSI color.
	// lipgloss handles this natively, but we also skip palette work so
	// our custom hex values don't fight a monochrome terminal.
	if os.Getenv("NO_COLOR") != "" {
		return
	}

	// ── Resolve palette ─────────────────────────────────────────────────
	name := strings.ToLower(themeArg)
	if name == "" || name == "auto" {
		if DetectDarkBackground() {
			name = "dark"
		} else {
			name = "light"
		}
	}

	p, ok := NamedThemes[name]
	if !ok {
		p = NamedThemes["dark"] // safe fallback
	}

	// Determine if this is a dark or light scheme (for any code that
	// needs to branch — e.g. ASCII art inversion).
	isDark = name != "light" && name != "solarized-light"

	// ── Apply base palette ──────────────────────────────────────────────
	ColorPrimary = lipgloss.Color(p.Primary)
	ColorSecondary = lipgloss.Color(p.Secondary)
	ColorSuccess = lipgloss.Color(p.Secondary)
	ColorAccent = lipgloss.Color(p.Accent)
	ColorError = lipgloss.Color(p.Error)
	ColorWarning = lipgloss.Color(p.Warning)
	ColorText = lipgloss.Color(p.Text)
	ColorSubtext = lipgloss.Color(p.Subtext)
	ColorBg = lipgloss.Color(p.Bg)
	ColorPanel = lipgloss.Color(p.Panel)

	// ── Apply per-color overrides ───────────────────────────────────────
	// These always win. Keys are lowercase palette field names.
	for k, v := range colorOverrides {
		if v == "" {
			continue
		}
		switch strings.ToLower(k) {
		case "primary":
			ColorPrimary = lipgloss.Color(v)
		case "secondary":
			ColorSecondary = lipgloss.Color(v)
			ColorSuccess = lipgloss.Color(v)
		case "accent":
			ColorAccent = lipgloss.Color(v)
		case "error":
			ColorError = lipgloss.Color(v)
		case "warning":
			ColorWarning = lipgloss.Color(v)
		case "text":
			ColorText = lipgloss.Color(v)
		case "subtext":
			ColorSubtext = lipgloss.Color(v)
		case "bg":
			ColorBg = lipgloss.Color(v)
		case "panel":
			ColorPanel = lipgloss.Color(v)
		}
	}

	// ── Rebuild styles ──────────────────────────────────────────────────
	rebuildStyles()
}

// DetectDarkBackground checks the terminal background color.
// It respects the COLORFGBG env var (set by many terminal emulators and
// theme tools like base16-shell) and JEND_THEME as an escape hatch.
func DetectDarkBackground() bool {
	// 1. Explicit env override
	if v := strings.ToLower(os.Getenv("JEND_THEME")); v != "" {
		return v != "light"
	}

	// 2. COLORFGBG — standard across rxvt, xterm, iTerm2, etc.
	//    Format: "fg;bg" where bg >= 8 usually means dark.
	//    Many terminal theme tools (base16-shell, etc.) set this.
	if fgbg := os.Getenv("COLORFGBG"); fgbg != "" {
		parts := strings.Split(fgbg, ";")
		if len(parts) >= 2 {
			bg := parts[len(parts)-1]
			// ANSI color indices 0-6 are dark, 7+ are light
			if bg == "0" || bg == "1" || bg == "2" || bg == "3" || bg == "4" || bg == "5" || bg == "6" {
				return true
			}
			if bg == "7" || bg == "15" {
				return false
			}
		}
	}

	// 3. lipgloss's own detection (queries terminal via OSC 11)
	return lipgloss.HasDarkBackground()
}

// ── Style Rebuilder ───────────────────────────────────────────────────────────

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
// Initialized with dark palette defaults. rebuildStyles() overwrites them.

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
