package ui

import (
	"fmt"
	"io"
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

type activePane int

const (
	PaneBrowser activePane = iota
	PaneStaging
	PaneSearch
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
		return i.name + "/"
	}
	return i.name
}

func (i fileItem) Description() string { return "" }
func (i fileItem) FilterValue() string { return i.name }

// ── File Icon Helper ──

func fileIcon(name string, isDir bool) string {
	if name == ".." {
		return ".."
	}
	if isDir {
		return "/"
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".go", ".py", ".js", ".ts", ".rs", ".c", ".cpp", ".java", ".rb", ".sh":
		return "*"
	case ".jpg", ".jpeg", ".png", ".gif", ".svg", ".webp", ".bmp":
		return "~"
	case ".mp4", ".mov", ".avi", ".mkv", ".webm":
		return "~"
	case ".mp3", ".wav", ".flac", ".aac", ".ogg":
		return "~"
	case ".zip", ".tar", ".gz", ".rar", ".7z", ".bz2":
		return "#"
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".pptx", ".txt", ".md":
		return "="
	default:
		return "."
	}
}

// ── Browser Delegate (custom rendering with > and >> cursors) ──

type browserDelegate struct {
	width         int
	selectedFiles map[string]fileItem
}

func (d browserDelegate) Height() int                             { return 1 }
func (d browserDelegate) Spacing() int                            { return 0 }
func (d browserDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d browserDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(fileItem)
	if !ok {
		return
	}

	isSelected := index == m.Index()
	_, isStaged := d.selectedFiles[item.path]

	// Build the icon
	icon := fileIcon(item.name, item.isDir)

	// Build the name
	name := item.name
	if item.isDir && item.name != ".." {
		name = name + "/"
	}

	// Truncate long names
	maxNameLen := d.width - 30
	if maxNameLen < 15 {
		maxNameLen = 15
	}
	if len(name) > maxNameLen {
		name = name[:maxNameLen-3] + "..."
	}

	// Size column (right-aligned)
	sizeStr := ""
	if !item.isDir && item.name != ".." {
		sizeStr = FormatBytes(item.size)
	}

	// Date column
	dateStr := ""
	if item.name != ".." {
		dateStr = item.modTime.Format("Jan 02 15:04")
	}

	// Build the cursor prefix
	var prefix string
	if isSelected {
		prefix = ">>"
	} else {
		prefix = " >"
	}

	// Staged marker
	staged := " "
	if isStaged {
		staged = "+"
	}

	// Build the full line
	line := fmt.Sprintf(" %s %s %s %-*s  %8s  %s",
		prefix, staged, icon, maxNameLen, name, sizeStr, dateStr)

	// Apply styling
	var style lipgloss.Style
	if isSelected {
		if item.isDir {
			style = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
		} else {
			style = lipgloss.NewStyle().Foreground(ColorText).Bold(true)
		}
	} else if isStaged {
		style = lipgloss.NewStyle().Foreground(ColorSecondary)
	} else if item.isDir {
		style = lipgloss.NewStyle().Foreground(ColorPrimary)
	} else {
		style = lipgloss.NewStyle().Foreground(ColorSubtext)
	}

	rendered := style.Width(d.width).Render(line)
	fmt.Fprint(w, rendered)
}

// ── Staging Delegate ──

type stagedDelegate struct{ width int }

func (d stagedDelegate) Height() int                             { return 1 }
func (d stagedDelegate) Spacing() int                            { return 0 }
func (d stagedDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d stagedDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(fileItem)
	if !ok {
		return
	}

	icon := fileIcon(i.name, i.isDir)
	name := i.Title()
	size := ""
	if !i.isDir {
		size = FormatBytes(i.size)
	}

	if index == m.Index() {
		line := fmt.Sprintf(" >> %s %s  %s", icon, name, size)
		fmt.Fprint(w, lipgloss.NewStyle().Width(d.width).Foreground(ColorAccent).Bold(true).Render(line))
	} else {
		line := fmt.Sprintf("  > %s %s  %s", icon, name, size)
		fmt.Fprint(w, lipgloss.NewStyle().Width(d.width).Foreground(ColorText).Render(line))
	}
}

// ── FilePickerModel ──

type FilePickerModel struct {
	browserList   list.Model
	stagingList   list.Model
	searchInput   textinput.Model
	currentDir    string
	directoryMode bool

	activePane  activePane
	compactMode bool

	// Multi-select tracking
	selectedFiles map[string]fileItem

	// State
	quitting   bool
	err        error
	width      int
	height     int
	termWidth  int
	termHeight int

	// Show hidden files
	showHidden bool
}

// Custom styles
var (
	paneBorder          = lipgloss.RoundedBorder()
	activeBorder        = lipgloss.NewStyle().Border(paneBorder).BorderForeground(ColorAccent)
	inactiveBorder      = lipgloss.NewStyle().Border(paneBorder).BorderForeground(ColorSubtext)
	searchActiveStyle   = lipgloss.NewStyle().Border(paneBorder).BorderForeground(ColorAccent).Padding(0, 1)
	searchInactiveStyle = lipgloss.NewStyle().Border(paneBorder).BorderForeground(ColorSubtext).Padding(0, 1)
)

func NewFilePickerModel(directoryMode bool, previousPaths []string) *FilePickerModel {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	abs, _ := filepath.Abs(cwd)

	// Setup Browser List (Left Pane) — custom delegate
	bl := list.New([]list.Item{}, browserDelegate{width: 40}, 0, 0)
	bl.Title = abs
	bl.SetShowStatusBar(false)
	bl.SetShowFilter(false)
	bl.SetShowHelp(false)
	bl.SetShowTitle(false) // We render our own breadcrumb

	// Setup Staging List (Right Pane)
	sl := list.New([]list.Item{}, stagedDelegate{width: 20}, 0, 0)
	sl.Title = "Selected"
	sl.Styles.Title = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	sl.Styles.TitleBar = sl.Styles.TitleBar.PaddingBottom(1)
	sl.SetShowTitle(true)
	sl.SetShowStatusBar(false)
	sl.SetShowFilter(false)
	sl.SetShowHelp(false)

	// Setup Search
	ti := textinput.New()
	ti.Placeholder = "[ / to search ]"
	ti.PromptStyle = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(ColorText)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(ColorAccent)
	ti.Prompt = "/ "

	selected := make(map[string]fileItem)
	for _, p := range previousPaths {
		fi, err := os.Stat(p)
		if err == nil {
			selected[p] = fileItem{
				name:    filepath.Base(p),
				path:    p,
				isDir:   fi.IsDir(),
				size:    fi.Size(),
				modTime: fi.ModTime(),
			}
		}
	}

	m := &FilePickerModel{
		browserList:   bl,
		stagingList:   sl,
		searchInput:   ti,
		currentDir:    abs,
		directoryMode: directoryMode,
		activePane:    PaneBrowser,
		selectedFiles: selected,
	}

	if len(previousPaths) > 0 {
		m.refreshStagingList()
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
	if dir != "/" {
		items = append(items, fileItem{name: "..", path: filepath.Dir(dir), isDir: true})
	}

	var dirs, files []fileItem
	for _, entry := range entries {
		// Skip hidden files unless toggle is on
		if !m.showHidden && strings.HasPrefix(entry.Name(), ".") {
			continue
		}
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

	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i].name) < strings.ToLower(dirs[j].name) })
	sort.Slice(files, func(i, j int) bool { return strings.ToLower(files[i].name) < strings.ToLower(files[j].name) })

	for _, d := range dirs {
		items = append(items, d)
	}
	for _, f := range files {
		items = append(items, f)
	}

	m.browserList.SetItems(items)
	m.browserList.Title = dir
	m.browserList.ResetSelected()
}

func (m *FilePickerModel) refreshStagingList() {
	var items []list.Item
	var paths []string
	for p := range m.selectedFiles {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		items = append(items, m.selectedFiles[p])
	}
	m.stagingList.SetItems(items)
}

func (m *FilePickerModel) Init() tea.Cmd {
	m.readDir(m.currentDir)
	return nil
}

func (m *FilePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height

		m.width = (m.termWidth / 2) + 20
		if m.width < 90 {
			m.width = 90
		}
		if m.width > m.termWidth {
			m.width = m.termWidth
		}
		m.height = msg.Height

		bannerHeight := lipgloss.Height(RenderBanner())
		subtitleHeight := 2 // breadcrumb + gap
		footerHeight := 2

		availableHeight := m.height - bannerHeight - subtitleHeight - footerHeight - 1
		if availableHeight < 10 {
			availableHeight = 10
		}

		searchHeight := 3
		listHeight := availableHeight - searchHeight

		m.compactMode = m.width < 90

		var leftWidth, rightWidth int
		if m.compactMode {
			leftWidth = 0
			rightWidth = m.width - 2
		} else {
			paneWidth := (m.width - 2) / 2
			leftWidth = paneWidth
			rightWidth = m.width - leftWidth - 2
		}

		if !m.compactMode {
			m.browserList.SetSize(leftWidth-4, listHeight-2)
			m.browserList.SetDelegate(browserDelegate{
				width:         leftWidth - 6,
				selectedFiles: m.selectedFiles,
			})
		}

		m.stagingList.SetSize(rightWidth-4, listHeight-8)
		m.stagingList.SetDelegate(stagedDelegate{width: rightWidth - 6})
		m.stagingList.Styles.TitleBar = lipgloss.NewStyle().Width(rightWidth - 4).Align(lipgloss.Center)

		if m.compactMode {
			m.searchInput.Width = rightWidth - 4
		} else {
			m.searchInput.Width = leftWidth + rightWidth + 2 - 4
		}

		if m.compactMode && m.activePane == PaneBrowser {
			m.activePane = PaneSearch
			m.searchInput.Focus()
			m.searchInput.Placeholder = "Search files, or paste path..."
		}
		return m, nil

	case tea.KeyMsg:
		// Switch Panes with Tab
		if msg.String() == "tab" || msg.String() == "shift+tab" {
			if m.compactMode {
				if m.activePane == PaneSearch {
					m.activePane = PaneStaging
				} else {
					m.activePane = PaneSearch
				}
			} else {
				if m.activePane == PaneBrowser {
					m.activePane = PaneStaging
				} else if m.activePane == PaneStaging {
					m.activePane = PaneSearch
				} else {
					m.activePane = PaneBrowser
				}
			}

			if m.activePane == PaneSearch {
				m.searchInput.Focus()
				m.searchInput.Placeholder = "Search files, or paste path..."
			} else {
				m.searchInput.Blur()
				m.searchInput.Placeholder = "[ / to search ]"
			}
			return m, nil
		}

		// Handle Search Pane
		if m.activePane == PaneSearch {
			switch msg.Type {
			case tea.KeyEsc:
				if !m.compactMode {
					m.activePane = PaneBrowser
					m.searchInput.Blur()
				}
				return m, nil
			case tea.KeyEnter:
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
							if m.compactMode {
								m.searchInput.SetValue("")
							}
						} else if m.compactMode && !m.directoryMode {
							if _, exists := m.selectedFiles[path]; !exists {
								item := fileItem{
									name:    filepath.Base(path),
									path:    path,
									isDir:   false,
									size:    info.Size(),
									modTime: info.ModTime(),
								}
								m.selectedFiles[path] = item
								m.refreshStagingList()
								m.searchInput.SetValue("")
							}
						}
					}
				}

				if !m.compactMode {
					m.activePane = PaneBrowser
					m.searchInput.Blur()
				}
				return m, nil
			default:
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				return m, cmd
			}
		}

		// Global keys
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case "/":
			m.activePane = PaneSearch
			m.searchInput.Focus()
			return m, textinput.Blink
		case ".":
			// Toggle hidden files
			m.showHidden = !m.showHidden
			m.readDir(m.currentDir)
			return m, nil
		case "pgdown", "ctrl+down", "ctrl+d":
			if m.activePane == PaneBrowser {
				m.browserList.Paginator.NextPage()
			} else if m.activePane == PaneStaging {
				m.stagingList.Paginator.NextPage()
			}
			return m, nil
		case "pgup", "ctrl+up", "ctrl+u":
			if m.activePane == PaneBrowser {
				m.browserList.Paginator.PrevPage()
			} else if m.activePane == PaneStaging {
				m.stagingList.Paginator.PrevPage()
			}
			return m, nil
		}

		// Pane specific keys
		if m.activePane == PaneStaging {
			switch msg.String() {
			case " ": // Unstage
				if item, ok := m.stagingList.SelectedItem().(fileItem); ok {
					delete(m.selectedFiles, item.path)
					m.refreshStagingList()
					// Update browser delegate so staged markers refresh
					m.browserList.SetDelegate(browserDelegate{
						width:         m.browserList.Width(),
						selectedFiles: m.selectedFiles,
					})
				}
			case "enter":
				if len(m.selectedFiles) > 0 {
					m.quitting = true
					return m, tea.Quit
				}
			}
			var cmd tea.Cmd
			m.stagingList, cmd = m.stagingList.Update(msg)
			return m, cmd
		}

		if m.activePane == PaneBrowser {
			switch msg.String() {
			case " ":
				// Toggle selection
				if item, ok := m.browserList.SelectedItem().(fileItem); ok {
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
					m.refreshStagingList()
					// Update the browser delegate with new selection state
					m.browserList.SetDelegate(browserDelegate{
						width:         m.browserList.Width(),
						selectedFiles: m.selectedFiles,
					})
				}
				return m, nil
			case "enter":
				if item, ok := m.browserList.SelectedItem().(fileItem); ok {
					if item.isDir {
						m.currentDir = item.path
						m.browserList.ResetSelected()
						m.readDir(m.currentDir)
						return m, nil
					} else if !m.directoryMode {
						m.selectedFiles[item.path] = item
						m.quitting = true
						return m, tea.Quit
					}
				}
			case "left", "h":
				if m.currentDir != "/" {
					m.currentDir = filepath.Dir(m.currentDir)
					m.browserList.ResetSelected()
					m.readDir(m.currentDir)
				}
				return m, nil
			case "right", "l":
				if item, ok := m.browserList.SelectedItem().(fileItem); ok && item.isDir && item.name != ".." {
					m.currentDir = item.path
					m.browserList.ResetSelected()
					m.readDir(m.currentDir)
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.browserList, cmd = m.browserList.Update(msg)
			return m, cmd
		}

	}

	return m, nil
}

func (m *FilePickerModel) View() string {
	if m.quitting {
		return ""
	}

	banner := RenderBanner()
	bannerHeight := lipgloss.Height(banner)
	footerHeight := 2

	availableHeight := m.height - bannerHeight - 3 - footerHeight - 1 // 3 = breadcrumb + gap
	if availableHeight < 10 {
		return "Terminal too small"
	}

	searchHeight := 3
	listHeight := availableHeight - searchHeight

	var leftWidth, rightWidth int
	if m.compactMode {
		leftWidth = 0
		rightWidth = m.width - 2
	} else {
		paneWidth := (m.width - 2) / 2
		leftWidth = paneWidth
		rightWidth = m.width - leftWidth - 2
	}

	// ── Breadcrumb ──
	breadcrumb := m.renderBreadcrumb()

	// ── Left Pane (Browser) ──
	var leftPane string
	if !m.compactMode {
		leftPaneStyle := inactiveBorder
		if m.activePane == PaneBrowser {
			leftPaneStyle = activeBorder
		}
		leftContent := lipgloss.NewStyle().Width(leftWidth - 4).Height(listHeight - 2).Render(m.browserList.View())
		leftPane = leftPaneStyle.Render(leftContent)
	}

	// ── Right Pane (Staging + Preview) ──
	rightPaneStyle := inactiveBorder
	if m.activePane == PaneStaging {
		rightPaneStyle = activeBorder
	}

	// Staging title with count + total size
	var totalSize int64
	for _, f := range m.selectedFiles {
		totalSize += f.size
	}
	stagingTitle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).
		Render(fmt.Sprintf("Selected (%d)", len(m.selectedFiles)))
	if len(m.selectedFiles) > 0 {
		stagingTitle += lipgloss.NewStyle().Foreground(ColorSubtext).Faint(true).
			Render(fmt.Sprintf("  %s total", FormatBytes(totalSize)))
	}

	listStr := m.stagingList.View()
	if len(m.selectedFiles) == 0 {
		listStr = lipgloss.NewStyle().
			Width(rightWidth - 6).
			Height(m.stagingList.Height()).
			Align(lipgloss.Center).
			Foreground(ColorSubtext).
			Faint(true).
			Render("No files selected.\n\nPress space in the\nbrowser to add files.")
	}

	// Hover metadata
	hoverText := ""
	if item, ok := m.stagingList.SelectedItem().(fileItem); ok && len(m.selectedFiles) > 0 {
		fileType := "File"
		if item.isDir {
			fileType = "Directory"
		} else if ext := filepath.Ext(item.path); ext != "" {
			fileType = strings.TrimPrefix(ext, ".") + " file"
		}

		hoverText = fmt.Sprintf("Path  %s\nSize  %s\nMod   %s\nType  %s",
			filepath.Base(item.path),
			FormatBytes(item.size),
			item.modTime.Format("Jan 02, 15:04"),
			fileType)
	} else {
		hoverText = "\n\n\n\n"
	}

	hoverBox := lipgloss.NewStyle().
		Width(rightWidth - 4).
		Foreground(ColorSubtext).
		Faint(true).
		Align(lipgloss.Left).
		PaddingLeft(2).
		BorderTop(true).
		BorderForeground(ColorPanel).
		BorderStyle(lipgloss.NormalBorder()).
		Render(hoverText)

	rightContent := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().PaddingLeft(2).PaddingBottom(1).Render(stagingTitle),
		listStr,
		hoverBox,
	)
	rightContent = lipgloss.NewStyle().Width(rightWidth - 4).Height(listHeight - 2).Render(rightContent)
	rightPane := rightPaneStyle.Render(rightContent)

	// ── Combine Panes ──
	var mainContent string
	if m.compactMode {
		mainContent = rightPane
	} else {
		mainContent = lipgloss.JoinHorizontal(lipgloss.Top, leftPane, "  ", rightPane)
	}

	// ── Search Bar ──
	sPaneStyle := searchInactiveStyle
	if m.activePane == PaneSearch {
		sPaneStyle = searchActiveStyle
	}

	errText := ""
	if m.err != nil {
		errText = lipgloss.NewStyle().Foreground(ColorError).Render(" " + m.err.Error())
	}

	var totalTopWidth int
	if m.compactMode {
		totalTopWidth = rightWidth
	} else {
		totalTopWidth = leftWidth + rightWidth + 2
	}

	searchContent := lipgloss.NewStyle().Width(totalTopWidth - 2).Render(m.searchInput.View() + errText)
	searchPane := sPaneStyle.Width(totalTopWidth - 2).Render(searchContent)

	body := lipgloss.JoinVertical(lipgloss.Left, mainContent, searchPane)

	// ── Footer ──
	hiddenLabel := "off"
	if m.showHidden {
		hiddenLabel = "on"
	}
	footer := lipgloss.NewStyle().Foreground(ColorSubtext).Faint(true).Align(lipgloss.Left).
		Render(fmt.Sprintf("tab pane  ·  space select  ·  enter confirm  ·  / search  ·  . hidden(%s)  ·  esc quit", hiddenLabel))

	// ── Assemble ──
	fullPage := lipgloss.JoinVertical(lipgloss.Left, banner, breadcrumb, body, footer)

	if lipgloss.Height(fullPage) > m.height-1 {
		fullPage = lipgloss.NewStyle().MaxHeight(m.height - 1).Render(fullPage)
	}

	return fullPage
}

// renderBreadcrumb creates a clickable-looking path breadcrumb
func (m *FilePickerModel) renderBreadcrumb() string {
	home, _ := os.UserHomeDir()
	dir := m.currentDir

	// Replace home dir with ~
	display := dir
	if home != "" && strings.HasPrefix(dir, home) {
		display = "~" + dir[len(home):]
	}

	parts := strings.Split(display, "/")
	var rendered []string
	for i, p := range parts {
		if p == "" && i == 0 {
			p = "/"
		}
		if p == "" {
			continue
		}
		if i == len(parts)-1 {
			// Current directory — highlighted
			rendered = append(rendered, lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render(p))
		} else {
			rendered = append(rendered, lipgloss.NewStyle().Foreground(ColorSubtext).Render(p))
		}
	}

	sep := lipgloss.NewStyle().Foreground(ColorPanel).Render(" / ")
	breadcrumb := strings.Join(rendered, sep)

	// Item count
	itemCount := len(m.browserList.Items())
	countStr := lipgloss.NewStyle().Foreground(ColorSubtext).Faint(true).
		Render(fmt.Sprintf("  (%d items)", itemCount))

	return lipgloss.NewStyle().PaddingLeft(1).MarginBottom(1).
		Render(breadcrumb + countStr)
}

// RunFilePicker returns active file paths to be bundled/sent.
func RunFilePicker(directoryMode bool, previousPaths []string) ([]string, error) {
	m := NewFilePickerModel(directoryMode, previousPaths)
	tm, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
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
