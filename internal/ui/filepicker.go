package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// fileItem represents a file or directory in the list
type fileItem struct {
	name    string
	path    string
	isDir   bool
	size    int64
	modTime time.Time
}

func (i fileItem) Title() string {
	if i.isDir {
		return "📁 " + i.name
	}
	return "📄 " + i.name
}

func (i fileItem) Description() string {
	if i.isDir {
		return "Folder"
	}
	return fmt.Sprintf("%s • Modified: %s", FormatBytes(i.size), i.modTime.Format("Jan _2 15:04"))
}

func (i fileItem) FilterValue() string { return i.name }

// FilePickerModel handles the dual-pane file selection
type FilePickerModel struct {
	list          list.Model
	searchInput   textinput.Model
	currentDir    string
	directoryMode bool

	// Multi-select tracking: map of absolute path to fileItem
	selectedFiles map[string]fileItem

	// State
	searching bool
	quitting  bool
	err       error
	width     int
	height    int
}

// Custom styles for the file picker
var (
	paneBorder   = lipgloss.RoundedBorder()
	activeBorder = lipgloss.NewStyle().Border(paneBorder).BorderForeground(ColorAccent)
	dimmedBorder = lipgloss.NewStyle().Border(paneBorder).BorderForeground(ColorSubtext)
	sidebarStyle = lipgloss.NewStyle().Border(paneBorder).BorderForeground(ColorPrimary)
	searchStyle  = lipgloss.NewStyle().Border(paneBorder).BorderForeground(ColorAccent).Padding(0, 1)
)

func NewFilePickerModel(directoryMode bool) FilePickerModel {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	abs, _ := filepath.Abs(cwd)

	// Setup List
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(ColorAccent).BorderLeftForeground(ColorAccent)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(ColorSubtext).BorderLeftForeground(ColorAccent)

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "Directory: " + abs
	l.SetShowStatusBar(false)
	l.SetShowFilter(false) // We use our custom "Spotlight" search
	l.SetShowHelp(false)

	// Setup Spotlight Search
	ti := textinput.New()
	ti.Placeholder = "Search files, or paste path to navigate..."
	ti.PromptStyle = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(ColorText)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(ColorAccent)
	ti.Prompt = "🔍 "

	m := FilePickerModel{
		list:          l,
		searchInput:   ti,
		currentDir:    abs,
		directoryMode: directoryMode,
		selectedFiles: make(map[string]fileItem),
	}
	return m
}

func (m *FilePickerModel) readDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		m.err = err
		return
	}
	m.err = nil

	var items []list.Item

	// Always add parent directory if not root
	if dir != "/" {
		items = append(items, fileItem{name: "..", path: filepath.Dir(dir), isDir: true})
	}

	// Read and sort files
	var dirs, files []fileItem
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		item := fileItem{
			name:    entry.Name(),
			path:    filepath.Join(dir, entry.Name()),
			isDir:   entry.IsDir(),
			size:    info.Size(),
			modTime: info.ModTime(),
		}
		if entry.IsDir() {
			dirs = append(dirs, item)
		} else {
			files = append(files, item)
		}
	}

	// Sort alphabetically
	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i].name) < strings.ToLower(dirs[j].name) })
	sort.Slice(files, func(i, j int) bool { return strings.ToLower(files[i].name) < strings.ToLower(files[j].name) })

	for _, d := range dirs {
		items = append(items, d)
	}
	for _, f := range files {
		items = append(items, f)
	}

	m.list.SetItems(items)
	m.list.Title = "📁 " + dir
	m.list.ResetSelected()
}

func (m FilePickerModel) Init() tea.Cmd {
	m.readDir(m.currentDir)
	return nil
}

func (m FilePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Recalculate component dimensions based on window size
		bannerHeight := lipgloss.Height(RenderBanner())
		availableHeight := m.height - bannerHeight - 2 // -2 for margin

		searchHeight := 3 // Border takes 2 + input 1
		listHeight := availableHeight - searchHeight - 1

		leftWidth := (m.width * 70) / 100

		m.list.SetSize(leftWidth-4, listHeight-2) // -2 for borders
		m.searchInput.Width = m.width - 6         // Padding compensation

		return m, nil

	case tea.KeyMsg:
		if m.searching {
			switch msg.Type {
			case tea.KeyEsc:
				m.searching = false
				m.searchInput.Blur()
				return m, nil
			case tea.KeyEnter:
				// Process Spotlight search
				query := m.searchInput.Value()
				if strings.HasPrefix(query, "/") || strings.HasPrefix(query, "~") {
					path := query
					if strings.HasPrefix(path, "~") {
						if home, err := os.UserHomeDir(); err == nil {
							path = home + path[1:]
						}
					}
					if info, err := os.Stat(path); err == nil {
						if info.IsDir() {
							m.currentDir = path
							m.readDir(m.currentDir)
						}
					}
				} else {
					// Apply List Filter
					m.list.SetShowFilter(true)
					// We simulate a typing event downstream if needed
				}
				m.searching = false
				m.searchInput.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				return m, cmd
			}
		}

		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case " ":
			// Toggle Selection
			if item, ok := m.list.SelectedItem().(fileItem); ok {
				if item.name == ".." {
					return m, nil
				}
				if m.directoryMode && !item.isDir {
					return m, nil
				}
				if !m.directoryMode && item.isDir {
					return m, nil
				}

				if _, exists := m.selectedFiles[item.path]; exists {
					delete(m.selectedFiles, item.path)
				} else {
					m.selectedFiles[item.path] = item
				}
			}
			return m, nil
		case "enter":
			if item, ok := m.list.SelectedItem().(fileItem); ok {
				if item.isDir {
					// Navigate into directory
					m.currentDir = item.path
					m.list.ResetSelected()
					m.readDir(m.currentDir)
					return m, nil
				} else {
					if !m.directoryMode {
						// It's a file. If nothing in multi-select, add it implicitly and quit.
						if len(m.selectedFiles) == 0 {
							m.selectedFiles[item.path] = item
						}
						m.quitting = true
						return m, tea.Quit
					}
				}
			}
		case "d":
			if m.directoryMode {
				// Select current directory if we press d
				fi := fileItem{name: filepath.Base(m.currentDir), path: m.currentDir, isDir: true}
				m.selectedFiles[m.currentDir] = fi
				m.quitting = true
				return m, tea.Quit
			}
		case "/":
			m.searching = true
			m.searchInput.Focus()
			return m, textinput.Blink
		case "left", "h":
			// Go up
			if m.currentDir != "/" {
				m.currentDir = filepath.Dir(m.currentDir)
				m.list.ResetSelected()
				m.readDir(m.currentDir)
			}
			return m, nil
		case "right", "l":
			if item, ok := m.list.SelectedItem().(fileItem); ok && item.isDir && item.name != ".." {
				m.currentDir = item.path
				m.list.ResetSelected()
				m.readDir(m.currentDir)
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m FilePickerModel) View() string {
	if m.quitting {
		return ""
	}

	banner := RenderBanner()

	bannerHeight := lipgloss.Height(banner)
	availableHeight := m.height - bannerHeight - 2
	if availableHeight < 10 { // Fallback for very small terminals
		return "Terminal too small"
	}

	searchHeight := 3
	listHeight := availableHeight - searchHeight - 1

	leftWidth := (m.width * 70) / 100
	rightWidth := m.width - leftWidth - 2

	// Render Left Pane
	leftPaneStyle := activeBorder
	if m.searching {
		leftPaneStyle = dimmedBorder
	}
	leftPane := leftPaneStyle.Width(leftWidth - 2).Height(listHeight).Render(m.list.View())

	// Render Right Pane (Staging Area)
	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render("📥 Selected to Send:"))
	sb.WriteString("\n\n")

	var totalSize int64
	for path, item := range m.selectedFiles {
		totalSize += item.size
		name := item.name
		if len(name) > (rightWidth-10) && rightWidth > 15 {
			name = name[:rightWidth-15] + "..."
		}

		icon := "📄"
		if item.isDir {
			icon = "📁"
		}

		lineStyle := lipgloss.NewStyle().Foreground(ColorText)
		line := fmt.Sprintf("%s %s\n  %s", icon, name, lipgloss.NewStyle().Foreground(ColorSubtext).Render(FormatBytes(item.size)))
		sb.WriteString(lineStyle.Render(line) + "\n\n")
		_ = path
	}

	if len(m.selectedFiles) == 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(ColorSubtext).Faint(true).Render("Press Space to select items."))
	} else {
		// Footer of sidebar
		sb.WriteString(fmt.Sprintf("\n\nTotal Size: %s\n", FormatBytes(totalSize)))
		sb.WriteString(lipgloss.NewStyle().Foreground(ColorPrimary).Render("Press Enter to send"))
	}

	rightPane := sidebarStyle.Width(rightWidth - 2).Height(listHeight).Render(sb.String())

	// Combine Panes
	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, "  ", rightPane)

	// Search Pane
	sPaneStyle := searchStyle
	if !m.searching {
		sPaneStyle = sPaneStyle.BorderForeground(ColorSubtext)
		m.searchInput.Placeholder = "[ / to Spotlight Search ]"
	} else {
		m.searchInput.Placeholder = "Search files, or paste path to navigate..."
	}

	errText := ""
	if m.err != nil {
		errText = lipgloss.NewStyle().Foreground(ColorError).Render(" " + m.err.Error())
	}
	searchPane := sPaneStyle.Width(m.width - 4).Render(m.searchInput.View() + errText)

	body := lipgloss.JoinVertical(lipgloss.Left, mainContent, searchPane)

	return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, lipgloss.JoinVertical(lipgloss.Left, banner, body))
}

// RunFilePicker returns active file paths to be bundled/sent.
func RunFilePicker(directoryMode bool) ([]string, error) {
	m := NewFilePickerModel(directoryMode)
	tm, err := tea.NewProgram(&m, tea.WithAltScreen()).Run()
	if err != nil {
		return nil, err
	}

	result := tm.(*FilePickerModel)
	if result.quitting && len(result.selectedFiles) > 0 {
		var paths []string
		for p := range result.selectedFiles {
			paths = append(paths, p)
		}
		return paths, nil
	}
	return nil, nil
}
