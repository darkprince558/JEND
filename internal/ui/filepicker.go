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

func (i fileItem) Description() string {
	return "" // Descriptions disabled for compactness
}

func (i fileItem) FilterValue() string { return i.name }

// stagedDelegate is a custom delegate for the staging (right) pane. It renders items
// left-aligned with a > cursor.
type stagedDelegate struct{ width int }

func (d stagedDelegate) Height() int                             { return 1 }
func (d stagedDelegate) Spacing() int                            { return 0 }
func (d stagedDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d stagedDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(fileItem)
	if !ok {
		return
	}

	str := i.Title()

	wFn := lipgloss.NewStyle().Width(d.width).Align(lipgloss.Left).PaddingLeft(2).Render

	if index == m.Index() {
		// Active (space + >> + space = 4 chars wide)
		fmt.Fprint(w, wFn(lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render(" >> "+str)))
	} else {
		// Inactive (3 spaces + > = 4 chars wide)
		fmt.Fprint(w, wFn(lipgloss.NewStyle().Foreground(ColorText).Render("   >"+str)))
	}
}

// FilePickerModel handles the dual-pane file selection
type FilePickerModel struct {
	browserList   list.Model
	stagingList   list.Model
	searchInput   textinput.Model
	currentDir    string
	directoryMode bool

	activePane  activePane
	compactMode bool

	// Multi-select tracking: map of absolute path to fileItem
	selectedFiles map[string]fileItem

	// State
	quitting   bool
	err        error
	width      int
	height     int
	termWidth  int
	termHeight int
}

// Custom styles for the file picker
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

	// Setup Browser List (Left Pane)
	browserDelegate := list.NewDefaultDelegate()
	browserDelegate.ShowDescription = false // Compact spacing
	browserDelegate.SetSpacing(0)
	browserDelegate.Styles.SelectedTitle = browserDelegate.Styles.SelectedTitle.Foreground(ColorAccent).BorderLeftForeground(ColorAccent)

	bl := list.New([]list.Item{}, browserDelegate, 0, 0)
	bl.Title = "Directory: " + abs
	bl.SetShowStatusBar(false)
	bl.SetShowFilter(false) // Custom Spotlight instead
	bl.SetShowHelp(false)
	// Remap default list quit key from 'q' to 'esc'
	bl.KeyMap.Quit.SetKeys("esc", "ctrl+c")

	// Setup Staging List (Right Pane)
	sl := list.New([]list.Item{}, stagedDelegate{width: 20}, 0, 0)
	sl.Title = "Selected to Send"
	sl.Styles.Title = bl.Styles.Title.Copy()
	sl.Styles.TitleBar = sl.Styles.TitleBar.PaddingBottom(1) // Single extra space between title and list items
	sl.SetShowTitle(true)                                    // Bubbles defaults title left, we center the wrapper lower down
	sl.SetShowStatusBar(false)
	sl.SetShowFilter(false)
	sl.SetShowHelp(false)
	sl.KeyMap.Quit.SetKeys("esc", "ctrl+c")

	// Setup Spotlight Search
	ti := textinput.New()
	ti.Placeholder = "[ / to Spotlight Search ]"
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
	m.browserList.Title = "Directory: " + dir
	m.browserList.ResetSelected()
}

func (m *FilePickerModel) refreshStagingList() {
	var items []list.Item
	// To preserve ordering predictability, sort by path
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

		// Shrink the file picker to roughly half the terminal width
		m.width = m.termWidth / 2
		if m.width < 90 {
			m.width = 90 // Sane lower bounds so compact mode triggers or it remains legible
		}
		if m.width > m.termWidth {
			m.width = m.termWidth // Can't exceed screen
		}
		m.height = msg.Height

		bannerHeight := lipgloss.Height(RenderBanner())
		subtitleHeight := lipgloss.Height(SectionHeaderStyle.Render(">> File picker <<"))
		footerHeight := lipgloss.Height("tab switch pane") // Safely calculate exact requested footer height

		// Determine the vertical room we have for the lists
		// Subtract an extra 1 for the prompt/status bar of the terminal itself
		availableHeight := m.height - bannerHeight - subtitleHeight - footerHeight - 1
		if availableHeight < 10 {
			availableHeight = 10
		}

		searchHeight := 3
		listHeight := availableHeight - searchHeight

		m.compactMode = m.width < 90

		// The file picker takes exactly half the screen plus 20 characters (10 per pane)
		m.width = (m.termWidth / 2) + 20
		if m.width < 90 {
			m.width = 90 // Sane lower bounds so compact mode triggers or it remains legible
		}
		if m.width > m.termWidth {
			m.width = m.termWidth // Can't exceed screen
		}

		m.compactMode = m.width < 90

		var leftWidth, rightWidth int
		if m.compactMode {
			leftWidth = 0
			rightWidth = m.width - 2
		} else {
			// Make File Selection (left) and Selected (right) panes exactly equal width
			paneWidth := (m.width - 2) / 2
			leftWidth = paneWidth
			rightWidth = m.width - leftWidth - 2
		}

		if !m.compactMode {
			m.browserList.SetSize(leftWidth-4, listHeight-2)
		}

		// The overall height of the content block in the right pane should be exactly (listHeight - 2)
		// We subtract 6 for the hover box (4 lines text + 2 padding/borders), so stagingList height gets (listHeight - 8)
		m.stagingList.SetSize(rightWidth-4, listHeight-8)

		// Update custom delegate width
		m.stagingList.SetDelegate(stagedDelegate{width: rightWidth - 6})

		// The title sits inside the list, if we want it strictly centered within the right pane:
		m.stagingList.Styles.TitleBar = lipgloss.NewStyle().Width(rightWidth - 4).Align(lipgloss.Center)

		// Search bar width MUST match exactly (leftPane total + rightPane total)
		if m.compactMode {
			m.searchInput.Width = rightWidth - 4
		} else {
			m.searchInput.Width = leftWidth + rightWidth + 2 - 4
		}

		// If we shrunk into compact mode while focus was on Browser, shift focus to Search
		if m.compactMode && m.activePane == PaneBrowser {
			m.activePane = PaneSearch
			m.searchInput.Focus()
			m.searchInput.Placeholder = "Search files, or paste path to navigate..."
		}
		return m, nil

	case tea.KeyMsg:
		// Switch Panes with Tab
		if msg.String() == "tab" || msg.String() == "shift+tab" {
			if m.compactMode {
				// Only toggle between Search and Staging
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

			// Setup visual cues
			if m.activePane == PaneSearch {
				m.searchInput.Focus()
				m.searchInput.Placeholder = "Search files, or paste path to navigate..."
			} else {
				m.searchInput.Blur()
				m.searchInput.Placeholder = "[ / to Spotlight Search ]"
				if m.activePane == PaneStaging {
					// We only want to focus staging if there are items, otherwise bounce
					// But we will allow it to visually show the border jump even if empty.
				}
			}
			m.browserList.SetShowTitle(m.activePane == PaneBrowser)
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
						} else {
							// If it's a file, and we are in compact mode, add it directly!
							if m.compactMode && !m.directoryMode {
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
				} else {
					if !m.compactMode {
						m.browserList.SetShowFilter(true)
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

		// Handle Global / Quitting keys
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case "/":
			m.activePane = PaneSearch
			m.searchInput.Focus()
			return m, textinput.Blink
		// Pass page down/up to active list regardless of pane
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
			case " ": // Unstage toggle
				if item, ok := m.stagingList.SelectedItem().(fileItem); ok {
					delete(m.selectedFiles, item.path)
					m.refreshStagingList()
				}
			case "enter": // Submit regardless of pane
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
				// Toggle Selection
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
				}
				return m, nil
			case "enter":
				if item, ok := m.browserList.SelectedItem().(fileItem); ok {
					if m.directoryMode {
						if item.isDir && item.name != ".." {
							m.selectedFiles[item.path] = item
							m.quitting = true
							return m, tea.Quit
						}
					} else {
						if item.isDir {
							// Navigate into directory
							m.currentDir = item.path
							m.browserList.ResetSelected()
							m.readDir(m.currentDir)
							return m, nil
						} else {
							// If we hit enter on a file, and staging is empty, send just this file
							// If someone already staged items via spacebar, just stage this and submit.
							m.selectedFiles[item.path] = item
							m.quitting = true
							return m, tea.Quit
						}
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
	subtitle := SectionHeaderStyle.Render(">> File picker <<")

	bannerHeight := lipgloss.Height(banner)
	subtitleHeight := lipgloss.Height(subtitle)
	footerHeight := 2

	availableHeight := m.height - bannerHeight - subtitleHeight - footerHeight - 1
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
		// Sync with Update logic
		paneWidth := (m.width - 2) / 2
		leftWidth = paneWidth
		rightWidth = m.width - leftWidth - 2
	}

	// Render Left Pane
	var leftPane string
	if !m.compactMode {
		leftPaneStyle := inactiveBorder
		if m.activePane == PaneBrowser {
			leftPaneStyle = activeBorder
		}
		leftContent := lipgloss.NewStyle().Width(leftWidth - 4).Height(listHeight - 2).Render(m.browserList.View())
		leftPane = leftPaneStyle.Render(leftContent)
	}

	// Render Right Pane
	rightPaneStyle := inactiveBorder
	if m.activePane == PaneStaging {
		rightPaneStyle = activeBorder
	}

	listStr := m.stagingList.View()
	if len(m.selectedFiles) == 0 {
		// Override empty list output to look a bit friendlier
		listStr = lipgloss.NewStyle().
			Width(rightWidth - 6).
			Height(m.stagingList.Height()).
			Align(lipgloss.Center).
			Foreground(ColorSubtext).
			Faint(true).
			Render("No files mapped.\nPress Space in the\nbrowser to pin files.")
	}

	// Hover View Metadata Box
	hoverText := ""
	if item, ok := m.stagingList.SelectedItem().(fileItem); ok && len(m.selectedFiles) > 0 {
		fileType := "File"
		if item.isDir {
			fileType = "Directory"
		} else if ext := filepath.Ext(item.path); ext != "" {
			fileType = strings.TrimPrefix(ext, ".") + " file"
		}

		hoverText = fmt.Sprintf("Path: %s\nSize: %s\nMod: %s\nType: %s",
			filepath.Base(item.path),
			FormatBytes(item.size),
			item.modTime.Format("Jan 02, 15:04"),
			fileType)
	} else {
		hoverText = "\n\n\n\n" // Empty reserves 4 lines
	}

	hoverBox := lipgloss.NewStyle().
		Width(rightWidth - 4).
		Foreground(ColorSubtext).
		Faint(true).
		Align(lipgloss.Left).
		PaddingLeft(2).
		BorderTop(true).
		BorderForeground(ColorSubtext).
		BorderStyle(lipgloss.NormalBorder()).
		Render(hoverText)

	rightContent := lipgloss.JoinVertical(lipgloss.Center, listStr, hoverBox)
	// Force block height so borders don't jump
	rightContent = lipgloss.NewStyle().Width(rightWidth - 4).Height(listHeight - 2).Render(rightContent)
	rightPane := rightPaneStyle.Render(rightContent)

	// Combine Panes
	var mainContent string
	if m.compactMode {
		mainContent = rightPane
	} else {
		mainContent = lipgloss.JoinHorizontal(lipgloss.Top, leftPane, "  ", rightPane)
	}

	// Search Pane
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

	searchContent := lipgloss.NewStyle().Width(totalTopWidth - 2).Render(m.searchInput.View() + errText) // -2 for borders
	searchPane := sPaneStyle.Width(totalTopWidth - 2).Render(searchContent)

	body := lipgloss.JoinVertical(lipgloss.Left, mainContent, searchPane)

	// Footer Instructions
	footer := lipgloss.NewStyle().Foreground(ColorSubtext).Faint(true).Align(lipgloss.Left).Render("tab switch pane  ·  space toggle item  ·  enter confirm  ·  / search")

	// Pre-join everything exactly
	fullPage := lipgloss.JoinVertical(lipgloss.Left, banner, subtitle, body, footer)

	// Force the full page to never exceed the requested terminal m.height, truncating any 1-off lines that cause scrolls
	if lipgloss.Height(fullPage) > m.height-1 {
		fullPage = lipgloss.NewStyle().MaxHeight(m.height - 1).Render(fullPage)
	}

	// Return top-left aligned, do not center
	return fullPage
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
