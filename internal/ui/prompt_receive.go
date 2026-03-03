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
				trimmed := strings.TrimSpace(m.code)
				if trimmed == "" {
					m.errCodeValid = "Code cannot be empty"
					return m, nil
				}
				parts := strings.Split(trimmed, "-")
				// Simple validation for 3 words or a 6 char cloud code.
				if len(trimmed) >= 6 || len(parts) >= 3 {
					m.result = ReceiveOptions{
						Mode:         "code",
						TransferCode: trimmed,
					}
					return m, tea.Quit
				}
				m.errCodeValid = "Format must be 3 words (e.g. apple-brave-cat) or 6 chars (e.g. A1b2C3)"
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
				// Only accept a-z, A-Z, 0-9, and hyphens. Max reasonable length ~50
				s := msg.String()
				if len(s) == 1 && len(m.code) < 50 {
					c := s[0]
					if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
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

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	selectedItemStyle := lipgloss.NewStyle().Foreground(ColorText).Bold(true)
	unselectedItemStyle := lipgloss.NewStyle().Foreground(ColorSubtext)
	helpStyle := lipgloss.NewStyle().Foreground(ColorSubtext).Faint(true)
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6b6b"))
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(0, 1)

	s := "\n"
	s += titleStyle.Render("  How would you like to receive files?") + "\n\n"

	if m.phase == phaseReceiveMode {
		choices := []string{
			"Enter Transfer Code (P2P)",
			"Scan QR (Upload from Phone)",
		}

		for i, choice := range choices {
			if i == m.modeIdx {
				s += fmt.Sprintf("  %s %s\n", selectedItemStyle.Render("›"), selectedItemStyle.Render(choice))
			} else {
				s += fmt.Sprintf("    %s\n", unselectedItemStyle.Render(choice))
			}
		}
		s += "\n"
		s += helpStyle.Render("  ↑↓ navigate · enter select · esc cancel") + "\n"

	} else if m.phase == phaseEnterCode {
		s += "  Enter the 3-word code or 6-char Cloud code:\n"
		s += fmt.Sprintf("  %s\n", boxStyle.Render(m.code+"█"))

		if m.errCodeValid != "" {
			s += "\n  " + errorStyle.Render("⚠ "+m.errCodeValid) + "\n"
		} else {
			s += "\n" // spacing
		}

		s += "\n" + helpStyle.Render("  type code · enter confirm · esc back") + "\n"
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
