package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FilePickerModel wraps the bubbles filepicker with JEND styling + search.
type FilePickerModel struct {
	filepicker   filepicker.Model
	selectedFile string
	quitting     bool
	err          error
	// Search mode
	searchInput textinput.Model
	searching   bool
	searchErr   string
	// Terminal size
	width  int
	height int
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

	// Show sizes
	fp.ShowSize = true
	fp.ShowPermissions = false
	fp.Height = 15

	// Search text input
	ti := textinput.New()
	ti.Placeholder = "type a path or filename..."
	ti.PromptStyle = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(ColorText)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorSubtext).Faint(true)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(ColorAccent)
	ti.Prompt = "/ "
	ti.CharLimit = 256

	return FilePickerModel{
		filepicker:  fp,
		searchInput: ti,
	}
}

func (m FilePickerModel) Init() tea.Cmd {
	return m.filepicker.Init()
}

func (m FilePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Adjust filepicker height for available space
		fpHeight := msg.Height - 16
		if fpHeight < 8 {
			fpHeight = 8
		}
		if fpHeight > 25 {
			fpHeight = 25
		}
		m.filepicker.Height = fpHeight
		return m, nil

	case tea.KeyMsg:
		// Handle search mode
		if m.searching {
			switch msg.Type {
			case tea.KeyEsc:
				m.searching = false
				m.searchInput.Blur()
				m.searchInput.SetValue("")
				m.searchErr = ""
				return m, nil
			case tea.KeyEnter:
				// Navigate to the typed path
				path := strings.TrimSpace(m.searchInput.Value())
				if path == "" {
					m.searching = false
					m.searchInput.Blur()
					return m, nil
				}

				// Expand ~ to home dir
				if strings.HasPrefix(path, "~") {
					if home, err := os.UserHomeDir(); err == nil {
						path = home + path[1:]
					}
				}

				info, err := os.Stat(path)
				if err != nil {
					m.searchErr = "path not found"
					return m, nil
				}

				if info.IsDir() {
					// Navigate filepicker to this directory
					m.filepicker.CurrentDirectory = path
					m.searching = false
					m.searchInput.Blur()
					m.searchInput.SetValue("")
					m.searchErr = ""
					return m, m.filepicker.Init()
				}

				// It's a file — select it directly
				m.selectedFile = path
				m.searching = false
				return m, tea.Quit

			default:
				m.searchErr = ""
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				return m, cmd
			}
		}

		// Normal filepicker mode
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			return m, tea.Quit
		}

		// `/` activates search mode
		if msg.String() == "/" {
			m.searching = true
			m.searchInput.SetValue("")
			m.searchInput.Focus()
			m.searchErr = ""
			return m, textinput.Blink
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

	width := m.width
	height := m.height
	if width == 0 {
		width = 80
	}
	if height == 0 {
		height = 24
	}

	banner := RenderBanner()
	header := SectionHeaderStyle.Render("SELECT FILE")

	// Breadcrumb
	dir := m.filepicker.CurrentDirectory
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(dir, home) {
		dir = "~" + strings.TrimPrefix(dir, home)
	}
	breadcrumb := lipgloss.NewStyle().
		Foreground(ColorSubtext).
		Align(lipgloss.Center).
		Render(dir)

	// File picker view
	fpView := m.filepicker.View()

	var parts []string
	parts = append(parts, banner, "", header, breadcrumb, "")

	// Search bar (if active)
	if m.searching {
		searchBar := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorAccent).
			Padding(0, 1).
			Width(50).
			Render(m.searchInput.View())

		parts = append(parts, searchBar)

		if m.searchErr != "" {
			errLine := lipgloss.NewStyle().
				Foreground(ColorError).
				Faint(true).
				Align(lipgloss.Center).
				Render(m.searchErr)
			parts = append(parts, errLine)
		}
		parts = append(parts, "")
	}

	parts = append(parts, fpView, "")

	// Error message
	if m.err != nil {
		parts = append(parts, ErrorStyle.Render(m.err.Error()), "")
	}

	// Help footer
	helpText := "/ go to path  ·  enter select  ·  esc cancel"
	if m.searching {
		helpText = "enter navigate  ·  esc cancel search"
	}

	help := lipgloss.NewStyle().
		Foreground(ColorSubtext).
		Faint(true).
		Align(lipgloss.Center).
		Render(helpText)
	parts = append(parts, help)

	content := lipgloss.JoinVertical(lipgloss.Center, parts...)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

// RunFilePicker launches the interactive file picker TUI.
// Returns the selected file path, or empty string if cancelled.
func RunFilePicker() (string, error) {
	m := NewFilePickerModel()
	tm, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return "", err
	}

	result := tm.(FilePickerModel)
	if result.quitting || result.selectedFile == "" {
		return "", nil
	}
	return result.selectedFile, nil
}
