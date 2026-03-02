package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// QROptions holds the user's choices from the QR prompt.
type QROptions struct {
	Mode         string        // "local" or "cloud"
	MaxDownloads int           // 0 = unlimited
	ExpireAfter  time.Duration // 0 = never
	AutoApprove  bool          // Skip manual Y/n prompt
	Cancelled    bool
}

var downloadChoices = []struct {
	label string
	value int
}{
	{"Unlimited", 0},
	{"1 download", 1},
	{"2 downloads", 2},
	{"3 downloads", 3},
	{"5 downloads", 5},
	{"10 downloads", 10},
}

var expireChoices = []struct {
	label string
	value time.Duration
}{
	{"Never", 0},
	{"5 minutes", 5 * time.Minute},
	{"15 minutes", 15 * time.Minute},
	{"30 minutes", 30 * time.Minute},
	{"1 hour", 1 * time.Hour},
}

type qrField int

const (
	fieldMode qrField = iota
	fieldDownloads
	fieldExpire
	fieldAutoApprove
	fieldCount // sentinel
)

// QRPromptModel is the Bubble Tea model for the QR options prompt.
type QRPromptModel struct {
	activeField   qrField
	modeIdx       int
	downloadIdx   int
	expireIdx     int
	approveIdx    int // 0 = No, 1 = Yes
	confirmed     bool
	cancelled     bool
	cloudDisabled bool // true if cloud mode isn't available yet
}

func NewQRPromptModel() QRPromptModel {
	return QRPromptModel{
		activeField:   fieldMode,
		modeIdx:       0, // Local
		downloadIdx:   0, // Unlimited
		expireIdx:     0, // Never
		approveIdx:    0, // No
		cloudDisabled: false,
	}
}

func (m QRPromptModel) Init() tea.Cmd { return nil }

func (m QRPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			m.confirmed = true
			return m, tea.Quit
		case "tab", "down", "j":
			m.activeField = (m.activeField + 1) % fieldCount
		case "shift+tab", "up", "k":
			m.activeField = (m.activeField - 1 + fieldCount) % fieldCount
		case "left", "h":
			switch m.activeField {
			case fieldMode:
				if m.modeIdx > 0 {
					m.modeIdx--
				}
			case fieldDownloads:
				if m.downloadIdx > 0 {
					m.downloadIdx--
				}
			case fieldExpire:
				if m.expireIdx > 0 {
					m.expireIdx--
				}
			case fieldAutoApprove:
				if m.approveIdx > 0 {
					m.approveIdx--
				}
			}
		case "right", "l":
			switch m.activeField {
			case fieldMode:
				if !m.cloudDisabled && m.modeIdx < 1 {
					m.modeIdx++
				}
			case fieldDownloads:
				if m.downloadIdx < len(downloadChoices)-1 {
					m.downloadIdx++
				}
			case fieldExpire:
				if m.expireIdx < len(expireChoices)-1 {
					m.expireIdx++
				}
			case fieldAutoApprove:
				if m.approveIdx < 1 {
					m.approveIdx++
				}
			}
		}
	}
	return m, nil
}

func (m QRPromptModel) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	labelStyle := lipgloss.NewStyle().Foreground(ColorSubtext).Width(14)
	dimStyle := lipgloss.NewStyle().Foreground(ColorSubtext).Faint(true)
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorText)
	pointerStyle := lipgloss.NewStyle().Foreground(ColorPrimary)

	s := "\n"
	s += titleStyle.Render("  QR Transfer Settings") + "\n\n"

	// Mode
	modes := []string{"Local", "Cloud"}
	modeDescs := []string{"Fast, home/hotspot WiFi", "Works everywhere (WebRTC)"}
	ptr := " "
	if m.activeField == fieldMode {
		ptr = pointerStyle.Render("›")
	}
	s += fmt.Sprintf("  %s %s ", ptr, labelStyle.Render("Mode:"))
	for i, mode := range modes {
		if i == m.modeIdx {
			s += selectedStyle.Render("[" + mode + "]")
		} else if i == 1 && m.cloudDisabled {
			s += dimStyle.Render(" " + mode + " (soon)")
		} else {
			s += dimStyle.Render(" " + mode + " ")
		}
		s += "  "
	}
	s += dimStyle.Render("— " + modeDescs[m.modeIdx])
	s += "\n"

	// Downloads
	ptr = " "
	if m.activeField == fieldDownloads {
		ptr = pointerStyle.Render("›")
	}
	s += fmt.Sprintf("  %s %s ", ptr, labelStyle.Render("Downloads:"))
	for i, ch := range downloadChoices {
		if i == m.downloadIdx {
			s += selectedStyle.Render("[" + ch.label + "]")
		} else {
			s += dimStyle.Render(" " + ch.label + " ")
		}
		s += " "
	}
	s += "\n"

	// Expire
	ptr = " "
	if m.activeField == fieldExpire {
		ptr = pointerStyle.Render("›")
	}
	s += fmt.Sprintf("  %s %s ", ptr, labelStyle.Render("Expires:"))
	for i, ch := range expireChoices {
		if i == m.expireIdx {
			s += selectedStyle.Render("[" + ch.label + "]")
		} else {
			s += dimStyle.Render(" " + ch.label + " ")
		}
		s += " "
	}
	s += "\n"

	// Auto-Approve
	approveChoices := []string{"No", "Yes"}
	ptr = " "
	if m.activeField == fieldAutoApprove {
		ptr = pointerStyle.Render("›")
	}
	s += fmt.Sprintf("  %s %s ", ptr, labelStyle.Render("Auto-Approve:"))
	for i, ch := range approveChoices {
		if i == m.approveIdx {
			s += selectedStyle.Render("[" + ch + "]")
		} else {
			s += dimStyle.Render(" " + ch + " ")
		}
		s += " "
	}
	s += "\n\n"

	helpStyle := lipgloss.NewStyle().Foreground(ColorSubtext).Faint(true)
	s += helpStyle.Render("  ↑↓ navigate · ←→ change · enter confirm · esc cancel") + "\n"

	return s
}

// Result returns the user's selections.
func (m QRPromptModel) Result() QROptions {
	mode := "local"
	if m.modeIdx == 1 {
		mode = "cloud"
	}
	return QROptions{
		Mode:         mode,
		MaxDownloads: downloadChoices[m.downloadIdx].value,
		ExpireAfter:  expireChoices[m.expireIdx].value,
		AutoApprove:  m.approveIdx == 1,
		Cancelled:    m.cancelled,
	}
}

// RunQRPrompt launches the interactive QR options prompt and returns the result.
func RunQRPrompt() (QROptions, error) {
	m := NewQRPromptModel()
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return QROptions{Cancelled: true}, err
	}
	return finalModel.(QRPromptModel).Result(), nil
}
