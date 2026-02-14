package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FilePickerModel wraps the bubbles filepicker with JEND styling.
type FilePickerModel struct {
	filepicker   filepicker.Model
	selectedFile string
	quitting     bool
	err          error
}

// NewFilePickerModel creates a file picker starting in the current directory.
func NewFilePickerModel() FilePickerModel {
	fp := filepicker.New()

	// Start in CWD
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	fp.CurrentDirectory = cwd

	// Style with JEND palette
	fp.Styles.Cursor = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	fp.Styles.Symlink = lipgloss.NewStyle().Foreground(ColorSubtext).Italic(true)
	fp.Styles.Directory = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	fp.Styles.File = lipgloss.NewStyle().Foreground(ColorText)
	fp.Styles.Selected = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	fp.Styles.DisabledCursor = lipgloss.NewStyle().Foreground(ColorSubtext)
	fp.Styles.DisabledFile = lipgloss.NewStyle().Foreground(ColorSubtext)
	fp.Styles.Permission = lipgloss.NewStyle().Foreground(ColorSubtext)
	fp.Styles.FileSize = lipgloss.NewStyle().Foreground(ColorSubtext).Width(8).Align(lipgloss.Right)

	// Show sizes and permissions
	fp.ShowSize = true
	fp.ShowPermissions = false

	// Height
	fp.Height = 15

	return FilePickerModel{
		filepicker: fp,
	}
}

func (m FilePickerModel) Init() tea.Cmd {
	return m.filepicker.Init()
}

func (m FilePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			return m, tea.Quit
		}
		if msg.String() == "q" {
			m.quitting = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.filepicker, cmd = m.filepicker.Update(msg)

	// Did the user select a file?
	if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
		m.selectedFile = path
		return m, tea.Quit
	}

	// Did the user select a disabled file?
	if didSelect, path := m.filepicker.DidSelectDisabledFile(msg); didSelect {
		// Show error for disabled file
		m.err = fmt.Errorf("cannot select %s", path)
		m.selectedFile = ""
		return m, cmd
	}

	return m, cmd
}

func (m FilePickerModel) View() string {
	if m.quitting {
		return ""
	}

	var s strings.Builder

	// Header
	header := lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true).
		Padding(0, 0, 1, 0).
		Render("📂 Select a file to send")

	s.WriteString(header)
	s.WriteString("\n")
	s.WriteString(m.filepicker.View())
	s.WriteString("\n")

	// Error message
	if m.err != nil {
		s.WriteString(ErrorStyle.Render(m.err.Error()))
		s.WriteString("\n")
	}

	// Help footer
	help := lipgloss.NewStyle().
		Foreground(ColorSubtext).
		Faint(true).
		Render("↑↓ navigate · enter select · esc cancel")
	s.WriteString("\n")
	s.WriteString(help)

	return s.String()
}

// RunFilePicker launches the interactive file picker TUI.
// Returns the selected file path, or empty string if cancelled.
func RunFilePicker() (string, error) {
	m := NewFilePickerModel()
	tm, err := tea.NewProgram(m).Run()
	if err != nil {
		return "", err
	}

	result := tm.(FilePickerModel)
	if result.quitting || result.selectedFile == "" {
		return "", nil
	}
	return result.selectedFile, nil
}
