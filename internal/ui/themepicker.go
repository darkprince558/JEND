package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ThemeOrder defines the display order of themes in the picker.
var ThemeOrder = []string{"dark", "light", "dracula", "nord", "catppuccin", "solarized"}

// themeDescriptions are one-line summaries shown in the picker.
var themeDescriptions = map[string]string{
	"dark":       "Default JEND dark — purple & cyan on near-black",
	"light":      "Crisp light — deep purple on macOS-style white",
	"dracula":    "Dracula — lavender & green on deep charcoal",
	"nord":       "Nord — icy blue on Arctic dark blue",
	"catppuccin": "Catppuccin Mocha — pastel palette on soft dark",
	"solarized":  "Solarized Dark — muted tones, easy on the eyes",
}

// ThemePickerResult is returned by RunThemePicker.
type ThemePickerResult struct {
	Selected  string // name of chosen theme, empty if cancelled
	Cancelled bool
}

// ThemePickerModel is the Bubble Tea model for the theme picker.
type ThemePickerModel struct {
	cursor    int
	saved     string // the currently persisted theme name
	width     int
	height    int
	quitting  bool
	confirmed bool
	result    ThemePickerResult
}

// NewThemePickerModel creates a new theme picker model.
// savedTheme is what is currently saved in config (may be "" for auto).
func NewThemePickerModel(savedTheme string) ThemePickerModel {
	cursor := 0
	for i, name := range ThemeOrder {
		if name == savedTheme {
			cursor = i
			break
		}
	}
	return ThemePickerModel{
		cursor: cursor,
		saved:  savedTheme,
	}
}

func (m ThemePickerModel) Init() tea.Cmd { return nil }

func (m ThemePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quitting = true
			m.result = ThemePickerResult{Cancelled: true}
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(ThemeOrder)-1 {
				m.cursor++
			}

		case "enter", " ":
			m.confirmed = true
			m.result = ThemePickerResult{Selected: ThemeOrder[m.cursor]}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m ThemePickerModel) View() string {
	if m.quitting || m.confirmed {
		return ""
	}

	w := m.width
	if w < 60 {
		w = 60
	}

	// Resolve palette for highlighted theme
	highlighted := ThemeOrder[m.cursor]
	p := NamedThemes[highlighted]

	// ── Header ────────────────────────────────────────────────────────────
	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.Primary)).
		Bold(true).
		Render("  JEND Theme Picker")

	subtitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.Subtext)).
		Faint(true).
		Render("  Choose a color theme — colors update as you scroll")

	// ── Theme List ────────────────────────────────────────────────────────
	var listRows []string
	for i, name := range ThemeOrder {
		tp := NamedThemes[name]
		selected := i == m.cursor
		isSaved := name == m.saved

		// Name
		var nameStr string
		if selected {
			nameStr = lipgloss.NewStyle().
				Foreground(lipgloss.Color(tp.Primary)).
				Bold(true).
				Render("▶ " + strings.ToUpper(name))
		} else {
			nameStr = lipgloss.NewStyle().
				Foreground(lipgloss.Color(p.Subtext)).
				Render("  " + name)
		}

		// Saved indicator
		savedBadge := ""
		if isSaved {
			savedBadge = lipgloss.NewStyle().
				Foreground(lipgloss.Color(p.Secondary)).
				Render(" [saved]")
		}

		// Swatches — tiny colored blocks for this palette entry
		swatches := renderSwatches(tp)

		row := lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(22).Render(nameStr+savedBadge),
			lipgloss.NewStyle().Width(20).Render(swatches),
		)

		if selected {
			listRows = append(listRows, lipgloss.NewStyle().
				Background(lipgloss.Color(p.Panel)).
				Padding(0, 1).
				Render(row))
		} else {
			listRows = append(listRows, lipgloss.NewStyle().Padding(0, 1).Render(row))
		}
	}
	list := strings.Join(listRows, "\n")

	// ── Preview Panel ─────────────────────────────────────────────────────
	preview := renderThemePreview(p, highlighted)

	// ── Divider ──────────────────────────────────────────────────────────
	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.Panel)).
		Render(strings.Repeat("─", min(w-4, 60)))

	// ── Help bar ──────────────────────────────────────────────────────────
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.Subtext)).
		Faint(true).
		Render("  j/k or ↑↓ navigate   enter select   q cancel")

	// ── Assemble ──────────────────────────────────────────────────────────
	var s strings.Builder
	s.WriteString("\n")
	s.WriteString(header)
	s.WriteString("\n")
	s.WriteString(subtitle)
	s.WriteString("\n\n")
	s.WriteString(list)
	s.WriteString("\n\n")
	s.WriteString(divider)
	s.WriteString("\n")
	s.WriteString(preview)
	s.WriteString("\n")
	s.WriteString(divider)
	s.WriteString("\n")
	s.WriteString(help)
	s.WriteString("\n")

	return lipgloss.NewStyle().Padding(0, 2).Render(s.String())
}

// renderSwatches renders a compact row of colored block characters
// representing the key palette colors for a given theme.
func renderSwatches(p ThemePalette) string {
	block := "██"
	// Show: primary, secondary, accent, error, text/bg as swatches
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Primary)).Render(block) +
		lipgloss.NewStyle().Foreground(lipgloss.Color(p.Secondary)).Render(block) +
		lipgloss.NewStyle().Foreground(lipgloss.Color(p.Accent)).Render(block) +
		lipgloss.NewStyle().Foreground(lipgloss.Color(p.Error)).Render(block) +
		lipgloss.NewStyle().Foreground(lipgloss.Color(p.Warning)).Render(block)
}

// renderThemePreview renders a sample UI "card" using the given palette,
// simulating how the JEND TUI looks in that theme.
func renderThemePreview(p ThemePalette, name string) string {
	var s strings.Builder

	primaryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Primary)).Bold(true)
	secondaryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Secondary))
	accentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Accent)).Bold(true)
	subtextStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Subtext)).Faint(true)
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Text))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Error))
	panelStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(p.Panel)).
		Foreground(lipgloss.Color(p.Text)).
		Padding(0, 2)

	desc := themeDescriptions[name]

	// Description
	s.WriteString("  ")
	s.WriteString(subtextStyle.Render(desc))
	s.WriteString("\n\n")

	// --- Mini UI Preview ---
	// Simulate a JEND transfer status card
	cardContent := fmt.Sprintf(
		"  %s\n\n  %s %s\n  %s %s\n  %s %s\n  %s %s",
		primaryStyle.Render("SENDING"),
		subtextStyle.Render("File:    "),
		textStyle.Render("project.zip"),
		subtextStyle.Render("Code:    "),
		accentStyle.Render("brave-orange-falcon"),
		subtextStyle.Render("Status:  "),
		secondaryStyle.Render("Waiting for receiver..."),
		subtextStyle.Render("Error:   "),
		errorStyle.Render("(sample error text)"),
	)

	card := panelStyle.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(p.Primary)).
		Width(54).
		Render(cardContent)

	s.WriteString(lipgloss.NewStyle().PaddingLeft(2).Render(card))
	s.WriteString("\n")

	// Swatch row with labels
	s.WriteString("\n  ")
	swatchRow := []struct{ label, color string }{
		{"primary  ", p.Primary},
		{"accent   ", p.Accent},
		{"success  ", p.Secondary},
		{"error    ", p.Error},
	}
	for _, sw := range swatchRow {
		swatch := lipgloss.NewStyle().
			Foreground(lipgloss.Color(sw.color)).
			Render("●")
		label := subtextStyle.Render(sw.label)
		s.WriteString(fmt.Sprintf("%s %s  ", swatch, label))
	}
	s.WriteString("\n")

	return s.String()
}

// Result returns the final result after the TUI exits.
func (m ThemePickerModel) Result() ThemePickerResult {
	return m.result
}

// RunThemePicker starts the interactive theme picker TUI.
// savedTheme is the currently saved theme name (may be "" for auto).
// Returns the chosen theme name, or "" if cancelled.
func RunThemePicker(savedTheme string) (ThemePickerResult, error) {
	model := NewThemePickerModel(savedTheme)
	p := tea.NewProgram(model, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return ThemePickerResult{Cancelled: true}, err
	}
	return final.(ThemePickerModel).Result(), nil
}
