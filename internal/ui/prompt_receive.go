package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ReceiveOptions holds the user's choice from the top-level receive prompt.
type ReceiveOptions struct {
	Mode         string    // "code" or "qr"
	TransferCode string    // Used if Mode == "code"
	QROpts       QROptions // Used if Mode == "qr"
	Cancelled    bool
}

type receivePhase int

const (
	phaseReceiveMode receivePhase = iota
	phaseEnterCode
	phaseQRSettings // Handled by delegating to QRPromptModel
)

// ReceivePromptModel is the top-level interactive wizard for `jend receive`
// when no arguments are provided.
type ReceivePromptModel struct {
	phase        receivePhase
	modeIdx      int
	code         string
	errCodeValid string
	qrModel      QRPromptModel
	result       ReceiveOptions
	cancelled    bool
}

func NewReceivePromptModel() ReceivePromptModel {
	return ReceivePromptModel{
		phase:   phaseReceiveMode,
		modeIdx: 0,
		qrModel: NewQRPromptModel(),
	}
}

func (m ReceivePromptModel) Init() tea.Cmd {
	return m.qrModel.Init()
}

// normalizeCode converts spaces to hyphens and lowercases the input
// so users can type "apple brave cat" and it becomes "apple-brave-cat".
func normalizeCode(raw string) string {
	trimmed := strings.TrimSpace(raw)
	// Replace spaces with hyphens
	normalized := strings.ReplaceAll(trimmed, " ", "-")
	// Collapse multiple hyphens into one
	for strings.Contains(normalized, "--") {
		normalized = strings.ReplaceAll(normalized, "--", "-")
	}
	// Trim trailing hyphens
	normalized = strings.Trim(normalized, "-")
	return normalized
}

func (m ReceivePromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// If we are in the QR settings phase, delegate all updates to the sub-model.
	if m.phase == phaseQRSettings {
		updatedQR, cmd := m.qrModel.Update(msg)
		m.qrModel = updatedQR.(QRPromptModel)

		// Check if the QR model finished or was cancelled
		if m.qrModel.confirmed {
			m.result = ReceiveOptions{
				Mode:   "qr",
				QROpts: m.qrModel.Result(),
			}
			return m, tea.Quit
		}
		if m.qrModel.cancelled {
			// Back out to the main menu instead of completely quitting
			m.phase = phaseReceiveMode
			m.qrModel.cancelled = false // reset for next time
			return m, nil
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			if m.phase == phaseEnterCode {
				m.phase = phaseReceiveMode
				m.code = ""
				m.errCodeValid = ""
				return m, nil
			}
			m.result.Cancelled = true
			return m, tea.Quit

		case "up", "k":
			if m.phase == phaseReceiveMode && m.modeIdx > 0 {
				m.modeIdx--
			}
		case "down", "j":
			if m.phase == phaseReceiveMode && m.modeIdx < 1 {
				m.modeIdx++
			}
		case "enter", "return":
			if m.phase == phaseReceiveMode {
				if m.modeIdx == 0 {
					m.phase = phaseEnterCode
				} else {
					m.phase = phaseQRSettings
				}
				return m, nil
			}

			if m.phase == phaseEnterCode {
				normalized := normalizeCode(m.code)
				if normalized == "" {
					m.errCodeValid = "Please enter a transfer code"
					return m, nil
				}
				parts := strings.Split(normalized, "-")
				// Accept 3+ hyphenated words OR a 6+ character cloud code
				if len(parts) >= 3 || len(normalized) >= 6 {
					m.result = ReceiveOptions{
						Mode:         "code",
						TransferCode: normalized,
					}
					return m, tea.Quit
				}
				m.errCodeValid = "Enter 3 words (e.g. apple brave cat) or a 6-char code (e.g. A1b2C3)"
				return m, nil
			}

		case "backspace", "delete":
			if m.phase == phaseEnterCode {
				if len(m.code) > 0 {
					m.code = m.code[:len(m.code)-1]
					m.errCodeValid = "" // clear error on edit
				}
			}

		default:
			if m.phase == phaseEnterCode {
				s := msg.String()
				if len(s) == 1 && len(m.code) < 60 {
					c := s[0]
					// Accept letters, digits, hyphens, and spaces
					if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == ' ' {
						m.code += s
						m.errCodeValid = ""
					}
				}
			}
		}
	}

	return m, nil
}

func (m ReceivePromptModel) View() string {
	if m.result.Cancelled || (m.result.Mode != "") {
		return "" // Clear screen when done
	}

	if m.phase == phaseQRSettings {
		return m.qrModel.View()
	}

	// Styles
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	pointerStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(ColorText).Bold(true)
	unselectedStyle := lipgloss.NewStyle().Foreground(ColorSubtext)
	helpStyle := lipgloss.NewStyle().Foreground(ColorSubtext).Faint(true)
	errorStyle := lipgloss.NewStyle().Foreground(ColorError).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(ColorSubtext)
	codeDisplayStyle := lipgloss.NewStyle().
		Foreground(ColorAccent).
		Bold(true)
	inputBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(0, 2).
		Width(42)
	previewStyle := lipgloss.NewStyle().
		Foreground(ColorSuccess).
		Bold(true)

	s := "\n"
	s += titleStyle.Render("  How would you like to receive files?") + "\n\n"

	if m.phase == phaseReceiveMode {
		choices := []string{
			"Enter Transfer Code",
			"Scan QR (Upload from Phone)",
		}
		descs := []string{
			"Receive via P2P using a 3-word or cloud code",
			"Start a local server and scan QR from your phone",
		}

		for i, choice := range choices {
			if i == m.modeIdx {
				s += fmt.Sprintf("  %s %s\n", pointerStyle.Render("›"), selectedStyle.Render(choice))
				s += fmt.Sprintf("    %s\n", helpStyle.Render(descs[i]))
			} else {
				s += fmt.Sprintf("    %s\n", unselectedStyle.Render(choice))
				s += fmt.Sprintf("    %s\n", helpStyle.Render(descs[i]))
			}
		}
		s += "\n"
		s += helpStyle.Render("  ↑↓ navigate · enter select · esc cancel") + "\n"

	} else if m.phase == phaseEnterCode {
		s += labelStyle.Render("  Enter your transfer code below.") + "\n"
		s += helpStyle.Render("  You can use spaces or hyphens between words.") + "\n\n"

		// Render the input box with the raw typed text
		displayText := m.code + "█"
		s += "  " + inputBoxStyle.Render(codeDisplayStyle.Render(displayText)) + "\n"

		// Show the normalized preview if there is input
		normalized := normalizeCode(m.code)
		if normalized != "" {
			s += "\n"
			s += fmt.Sprintf("  %s  %s\n", labelStyle.Render("Code:"), previewStyle.Render(normalized))
		}

		if m.errCodeValid != "" {
			s += "\n"
			s += "  " + errorStyle.Render("✗ "+m.errCodeValid) + "\n"
		}

		s += "\n"

		// Show accepted format examples
		exampleStyle := lipgloss.NewStyle().Foreground(ColorAccent).Faint(true)
		s += helpStyle.Render("  Examples: ") +
			exampleStyle.Render("apple brave cat") +
			helpStyle.Render("  or  ") +
			exampleStyle.Render("A1b2C3") + "\n"
		s += helpStyle.Render("  enter confirm · esc back") + "\n"
	}

	return s
}

// Result returns the finalized options chosen by the user.
func (m ReceivePromptModel) Result() ReceiveOptions {
	return m.result
}

// RunReceivePrompt launches the root interactive receive wizard.
func RunReceivePrompt() (ReceiveOptions, error) {
	m := NewReceivePromptModel()
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return ReceiveOptions{Cancelled: true}, err
	}
	return finalModel.(ReceivePromptModel).Result(), nil
}
