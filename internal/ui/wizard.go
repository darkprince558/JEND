package ui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// WizardStep tracks which step we're on
type WizardStep int

const (
	WizardStepSource  WizardStep = 0
	WizardStepOptions WizardStep = 1
	WizardStepConfirm WizardStep = 2
)

// WizardResult is what the wizard returns to the caller
type WizardResult struct {
	FilePath    string
	TextContent string
	IsText      bool
	UseS3       bool
	ForceZip    bool
	ForceTar    bool
	Incognito   bool
	NoClipboard bool
	NoHistory   bool
	Cancelled   bool
}

// sourceOption represents an option in step 1
type sourceOption struct {
	label string
	icon  string
	desc  string
}

// wizardHistoryItem stores editor state for undo/redo
type wizardHistoryItem struct {
	text   string
	cursor int
}

// SendWizardModel is the main wizard Bubble Tea model
type SendWizardModel struct {
	step     WizardStep
	cursor   int
	quitting bool
	result   WizardResult

	// Editor history
	history      []wizardHistoryItem
	historyIndex int

	// Step 1: Source type
	sourceOptions []sourceOption
	sourceChoice  int

	// Step 1 sub: text input
	textArea    textarea.Model
	textEditing bool

	// Step 1 sub: file selected
	filePath string
	fileSize int64
	fileName string

	// Step 2: Options
	optCursor     int
	confirmCursor int
	modeChoice    int // 0 = Direct, 1 = S3
	forceZip      bool
	forceTar      bool
	incognito     bool
	noClipboard   bool
	noHistory     bool

	// Terminal size
	width  int
	height int
}

// NewSendWizardModel creates the wizard model
func NewSendWizardModel() SendWizardModel {
	// Configure textarea
	ta := textarea.New()
	ta.Placeholder = "Type or paste your text here..."
	ta.ShowLineNumbers = false
	ta.SetWidth(50)
	ta.SetHeight(6)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Base = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorAccent).
		Padding(0, 1)
	ta.BlurredStyle.Base = ta.FocusedStyle.Base
	ta.CharLimit = 4096

	// Map extra standard text navigation keys to the textarea
	ta.KeyMap.WordForward = key.NewBinding(key.WithKeys("alt+right", "opt+right", "ctrl+right"))
	ta.KeyMap.WordBackward = key.NewBinding(key.WithKeys("alt+left", "opt+left", "ctrl+left"))
	ta.KeyMap.LineStart = key.NewBinding(key.WithKeys("home", "ctrl+a", "ctrl+home", "alt+up", "opt+up"))
	ta.KeyMap.LineEnd = key.NewBinding(key.WithKeys("end", "ctrl+e", "ctrl+end", "alt+down", "opt+down"))

	return SendWizardModel{
		step: WizardStepSource,
		sourceOptions: []sourceOption{
			{label: "Text Snippet", icon: ">", desc: "Type or paste text to send"},
			{label: "File", icon: ">", desc: "Browse and select a single file"},
			{label: "Folder", icon: ">", desc: "Select a directory to send (auto-zipped)"},
		},
		sourceChoice: 0,
		modeChoice:   0,
		textArea:     ta,
		history:      []wizardHistoryItem{{text: "", cursor: 0}}, // Initial state
		historyIndex: 0,
	}
}

func (m SendWizardModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m SendWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Global quit
		// If editing text, Ctrl+C means Copy. So we only globally quit if NOT editing text.
		if msg.Type == tea.KeyCtrlC && !m.textEditing {
			m.quitting = true
			m.result.Cancelled = true
			return m, tea.Quit
		}

		// Text editing mode
		if m.textEditing {
			return m.handleTextInput(msg)
		}

		switch m.step {
		case WizardStepSource:
			return m.updateStepSource(msg)
		case WizardStepOptions:
			return m.updateStepOptions(msg)
		case WizardStepConfirm:
			return m.updateStepConfirm(msg)
		}
	}

	// Forward non-key messages to textarea when editing
	if m.textEditing {
		var cmd tea.Cmd
		m.textArea, cmd = m.textArea.Update(msg)
		return m, cmd
	}

	return m, nil
}

// ── Step 1: Source ──

func (m SendWizardModel) updateStepSource(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp, tea.KeyShiftTab:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown, tea.KeyTab:
		if m.cursor < len(m.sourceOptions)-1 {
			m.cursor++
		}
	case tea.KeyEsc:
		m.quitting = true
		m.result.Cancelled = true
		return m, tea.Quit
	case tea.KeyEnter:
		m.sourceChoice = m.cursor
		switch m.cursor {
		case 0:
			// Text input (now first option)
			m.textEditing = true
			m.textArea.Reset()
			m.textArea.Focus()
			return m, textarea.Blink
		case 1, 2:
			// File (1) or Folder (2) picker
			return m, tea.Batch(tea.ClearScreen, tea.Quit) // Clear to prevent jitter before launching picker
		}
	case tea.KeyRight:
		// We purposefully do not auto-advance to step 2 here to prevent
		// fast-forwarding bugs if the user wants to re-edit their selection.
		// They must use KeyEnter.
	}
	return m, nil
}

// ── Text Input Handler ──

func (m SendWizardModel) handleTextInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.textEditing = false
		m.textArea.Blur()
		return m, nil
	case tea.KeyTab:
		// Tab to submit and move to next step
		val := strings.TrimSpace(m.textArea.Value())
		if val == "" {
			return m, nil // Don't allow empty
		}
		m.result.TextContent = m.textArea.Value()
		m.result.IsText = true
		m.textEditing = false
		m.textArea.Blur()
		m.step = WizardStepOptions
		m.cursor = 0
		m.optCursor = 0
		m.confirmCursor = 0
		return m, nil
	}

	// Undo
	if msg.Type == tea.KeyCtrlZ {
		if m.historyIndex > 0 {
			m.historyIndex--
			state := m.history[m.historyIndex]
			m.textArea.SetValue(state.text)
			m.textArea.SetCursor(state.cursor)
		}
		return m, nil
	}

	// Redo
	if msg.Type == tea.KeyCtrlY || (msg.Type == tea.KeyCtrlZ && msg.Alt) || msg.String() == "ctrl+shift+z" {
		if m.historyIndex < len(m.history)-1 {
			m.historyIndex++
			state := m.history[m.historyIndex]
			m.textArea.SetValue(state.text)
			m.textArea.SetCursor(state.cursor)
		}
		return m, nil
	}

	// Cut (Ctrl+X)
	if msg.Type == tea.KeyCtrlX {
		val := m.textArea.Value()
		if val != "" {
			_ = clipboard.WriteAll(val)
			m.textArea.Reset()

			// Save to history
			if m.historyIndex < len(m.history)-1 {
				m.history = m.history[:m.historyIndex+1]
			}
			m.history = append(m.history, wizardHistoryItem{text: "", cursor: 0})
			m.historyIndex++
		}
		return m, nil
	}

	// Copy (Ctrl+C)
	if msg.Type == tea.KeyCtrlC {
		val := m.textArea.Value()
		if val != "" {
			_ = clipboard.WriteAll(val)
		}
		return m, nil
	}

	// Paste (Ctrl+V)
	if msg.Type == tea.KeyCtrlV {
		text, err := clipboard.ReadAll()
		if err == nil && text != "" {
			oldVal := m.textArea.Value()
			m.textArea.InsertString(text)
			newVal := m.textArea.Value()

			if newVal != oldVal {
				if m.historyIndex < len(m.history)-1 {
					m.history = m.history[:m.historyIndex+1]
				}
				m.history = append(m.history, wizardHistoryItem{text: newVal, cursor: len(newVal)})
				m.historyIndex++
			}
		}
		return m, nil
	}

	// Capture state before update to compare
	oldVal := m.textArea.Value()

	// Forward to textarea
	var cmd tea.Cmd
	m.textArea, cmd = m.textArea.Update(msg)

	// If changed, push history
	newVal := m.textArea.Value()
	if newVal != oldVal {
		// Truncate redo history
		if m.historyIndex < len(m.history)-1 {
			m.history = m.history[:m.historyIndex+1]
		}
		// Append new state
		// Note: We use len(newVal) for cursor defaults as we can't easily get cursor index
		// from m.textArea.Cursor field in this version without potential reflection or digging.
		// This suffices for basic undo (cursor moves to end).
		m.history = append(m.history, wizardHistoryItem{text: newVal, cursor: len(newVal)})
		m.historyIndex++
	}

	return m, cmd
}

// ── Step 2: Options ──

func (m SendWizardModel) updateStepOptions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	totalItems := 7 // Direct, S3, ZIP, TAR, Incognito, No Clipboard, No History

	switch msg.Type {
	case tea.KeyUp:
		if m.optCursor > 0 {
			m.optCursor--
		}
	case tea.KeyDown:
		if m.optCursor < totalItems-1 {
			m.optCursor++
		}
	case tea.KeyEsc, tea.KeyLeft:
		// Go back to step 1 (Non-destructive)
		m.step = WizardStepSource
		// Clear IsText so clicking Text Snippet again doesn't fast-forward
		m.result.IsText = false
		m.cursor = m.sourceChoice
		return m, nil
	case tea.KeyEnter, tea.KeySpace:
		switch m.optCursor {
		case 0: // Direct
			m.modeChoice = 0
		case 1: // S3
			m.modeChoice = 1
		case 2: // ZIP toggle
			m.forceZip = !m.forceZip
			if m.forceZip {
				m.forceTar = false
			}
		case 3: // TAR toggle
			m.forceTar = !m.forceTar
			if m.forceTar {
				m.forceZip = false
			}
		case 4: // Incognito toggle
			m.incognito = !m.incognito
			if m.incognito {
				m.noClipboard = false
				m.noHistory = false
			}
		case 5: // No Clipboard toggle
			m.noClipboard = !m.noClipboard
			if m.noClipboard {
				m.incognito = false
			}
			if m.noClipboard && m.noHistory {
				m.incognito = true
				m.noClipboard = false
				m.noHistory = false
			}
		case 6: // No History toggle
			m.noHistory = !m.noHistory
			if m.noHistory {
				m.incognito = false
			}
			if m.noClipboard && m.noHistory {
				m.incognito = true
				m.noClipboard = false
				m.noHistory = false
			}
		}
	case tea.KeyTab, tea.KeyRight:
		// Tab advances to confirm step
		m.result.UseS3 = (m.modeChoice == 1)
		m.result.ForceZip = m.forceZip
		m.result.ForceTar = m.forceTar
		m.result.Incognito = m.incognito
		m.result.NoClipboard = m.noClipboard || m.incognito
		m.result.NoHistory = m.noHistory || m.incognito
		m.step = WizardStepConfirm
		return m, nil
	}

	return m, nil
}

// ── Step 3: Confirm ──

func (m SendWizardModel) updateStepConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if m.confirmCursor > 0 {
			m.confirmCursor--
		}
	case tea.KeyDown:
		items := m.getConfirmItems()
		if m.confirmCursor < len(items)-1 {
			m.confirmCursor++
		}
	case tea.KeyEsc, tea.KeyLeft:
		m.step = WizardStepOptions
		m.optCursor = 0
		return m, nil
	case tea.KeyEnter:
		items := m.getConfirmItems()
		// Only trigger completion if the "Start" button is selected
		if m.confirmCursor == len(items)-1 {
			m.result.FilePath = m.filePath
			m.quitting = false
			return m, tea.Quit
		}
	}
	return m, nil
}

// ── View ──

func (m SendWizardModel) View() string {
	if m.quitting {
		return ""
	}

	var content string
	switch m.step {
	case WizardStepSource:
		if m.textEditing {
			content = m.viewTextInput()
		} else {
			content = m.viewStepSource()
		}
	case WizardStepOptions:
		content = m.viewStepOptions()
	case WizardStepConfirm:
		content = m.viewStepConfirm()
	}

	// Step indicator
	steps := m.viewStepDots()

	// Wrap everything
	body := lipgloss.JoinVertical(lipgloss.Left,
		content,
		"",
		steps,
	)

	// Add some padding to the whole body (left margin)
	body = lipgloss.NewStyle().PaddingLeft(4).Render(body)

	// Pin banner to top, content below
	// Use Place with Top alignment to prevent vertical bouncing
	fullView := lipgloss.JoinVertical(lipgloss.Left,
		RenderBanner(),
		body,
	)

	return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, fullView)
}

func (m SendWizardModel) viewStepSource() string {
	var s strings.Builder

	header := WizardHeaderStyle.Render(">> What do you want to send? <<")
	s.WriteString(header)
	s.WriteString("\n")

	// Calculate max length for alignment
	maxLen := 0
	for _, opt := range m.sourceOptions {
		// prefix/icon (2) + spacing (1) + label len
		l := 3 + len(opt.label)
		if l > maxLen {
			maxLen = l
		}
	}

	for i, opt := range m.sourceOptions {
		style := lipgloss.NewStyle().Foreground(ColorText).Width(maxLen)
		prefix := " "
		icon := ">"
		if i == m.cursor {
			style = RadioActiveStyle.Width(maxLen)
			prefix = ">"
		}

		labelRaw := fmt.Sprintf("%s%s %s", prefix, icon, opt.label)
		line := style.Render(labelRaw)

		// Inline description if active
		if i == m.cursor {
			desc := lipgloss.NewStyle().
				Foreground(ColorSubtext).
				Faint(true).
				PaddingLeft(2).
				Render(opt.desc)
			line = lipgloss.JoinHorizontal(lipgloss.Left, line, desc)
		}

		s.WriteString(line)
		s.WriteString("\n")
	}

	help := WizardHelpStyle.Render("arrows navigate · enter select · esc quit")
	s.WriteString(help)

	return s.String()
}

func (m SendWizardModel) viewTextInput() string {
	var s strings.Builder

	header := WizardHeaderStyle.Render(">> Type your text <<")
	s.WriteString(header)
	s.WriteString("\n")

	s.WriteString(m.textArea.View())
	s.WriteString("\n")

	// Stats & Hint
	val := m.textArea.Value()
	line := m.textArea.Line() + 1
	stats := fmt.Sprintf("Ln %d   %d chars", line, len(val))
	statsStyle := lipgloss.NewStyle().Foreground(ColorSubtext).Faint(true)

	hint := "(up ↑ down ↓)"
	hintStyle := lipgloss.NewStyle().Foreground(ColorSubtext).Faint(true)

	// Combine stats (left) and hint (right) - width approx 50
	// textArea width is 50 content + 2 padding + 2 border = 54 total visual width
	footer := lipgloss.JoinHorizontal(lipgloss.Top,
		statsStyle.Render(stats),
		lipgloss.NewStyle().Width(54-lipgloss.Width(stats)).Align(lipgloss.Right).Render(hintStyle.Render(hint)),
	)

	s.WriteString(footer)
	s.WriteString("\n")

	help := WizardHelpStyle.Render("tab submit · esc back")
	s.WriteString(help)

	return s.String()
}

func (m SendWizardModel) viewStepOptions() string {
	var s strings.Builder

	// File info header
	if m.result.IsText {
		header := WizardHeaderStyle.Render(">> Sending text snippet <<")
		s.WriteString(header)
	} else {
		sizeStr := FormatBytes(m.fileSize)
		header := WizardHeaderStyle.Render(fmt.Sprintf(">> %s (%s) <<", m.fileName, sizeStr))
		s.WriteString(header)
	}

	s.WriteString("\n")

	sectionStyle := lipgloss.NewStyle().
		Foreground(ColorText).
		Bold(true).
		Align(lipgloss.Left)

	// -- Transfer Mode --
	s.WriteString(sectionStyle.Render("Transfer Mode"))
	s.WriteString("\n")

	s.WriteString(m.renderRadio("Direct (P2P)", "Fastest · real-time · both online", m.optCursor == 0, m.modeChoice == 0))
	s.WriteString(m.renderRadio("Cloud (S3)  ", "Async · pick up later · ≤200MB", m.optCursor == 1, m.modeChoice == 1))

	// -- Compression --
	s.WriteString(sectionStyle.Render("Compression"))
	s.WriteString("\n")

	s.WriteString(m.renderToggle("Compress as ZIP", "Bundle into .zip archive", m.optCursor == 2, m.forceZip))
	s.WriteString(m.renderToggle("Compress as TAR", "Bundle into .tar.gz archive", m.optCursor == 3, m.forceTar))

	// -- Privacy --
	s.WriteString(sectionStyle.Render("Privacy"))
	s.WriteString("\n")

	s.WriteString(m.renderToggle("Incognito   ", "No clipboard, no history", m.optCursor == 4, m.incognito))
	s.WriteString(m.renderToggle("No clipboard", "Don't copy code", m.optCursor == 5, m.noClipboard))
	s.WriteString(m.renderToggle("No history  ", "Skip audit log", m.optCursor == 6, m.noHistory))

	help := WizardHelpStyle.Render("arrows navigate · enter toggle · esc back")
	s.WriteString(help)

	return s.String()
}

func (m SendWizardModel) viewStepConfirm() string {
	var s strings.Builder

	header := WizardHeaderStyle.Render(">> Ready to send <<")
	s.WriteString(header)
	s.WriteString("\n")

	items := m.getConfirmItems()

	// Calculate max label length for consistency
	maxLabel := 0
	for _, it := range items {
		if len(it.label) > maxLabel {
			maxLabel = len(it.label)
		}
	}

	for i, item := range items {
		// Base style
		style := lipgloss.NewStyle().Foreground(ColorText).Width(maxLabel)
		prefix := " "
		icon := ">"

		// Button special formatting
		if item.isButton {
			btnStyle := lipgloss.NewStyle().
				Foreground(ColorBg).
				Background(ColorSecondary).
				Bold(true).
				Padding(0, 2)

			if i == m.confirmCursor {
				// Highlighted button
				btnStyle = btnStyle.Background(ColorAccent).Foreground(ColorBg)
			}

			// No icon or selection prefix for button
			line := "   " + btnStyle.Render(item.label)
			s.WriteString("\n" + line + "\n")
			continue
		}

		// Regular item
		if i == m.confirmCursor {
			style = RadioActiveStyle.Width(maxLabel)
			prefix = ">" // Becomes ">>"
		}

		// Label (prefix + icon + space)
		line := prefix + icon + " " + style.Render(item.label)

		// Description (Value)
		if i == m.confirmCursor {
			descStyle := lipgloss.NewStyle().
				Foreground(ColorSubtext).
				Faint(true).
				PaddingLeft(2)
			line = lipgloss.JoinHorizontal(lipgloss.Left, line, descStyle.Render(item.desc))
		} else {
			// Show the value slightly dimmed
			valStyle := lipgloss.NewStyle().
				Foreground(ColorSubtext).
				PaddingLeft(2)
			line = lipgloss.JoinHorizontal(lipgloss.Left, line, valStyle.Render(item.value))
		}

		s.WriteString(line)
		s.WriteString("\n")
	}

	s.WriteString("\n")
	help := WizardHelpStyle.Render("arrows navigate · enter start · esc back")
	s.WriteString(help)

	return s.String()
}

type confirmItem struct {
	label    string
	value    string
	desc     string
	isButton bool
}

func (m SendWizardModel) getConfirmItems() []confirmItem {
	items := []confirmItem{}

	// Source
	srcVal := m.fileName
	srcDesc := fmt.Sprintf("%s (%s)", m.filePath, FormatBytes(m.fileSize))
	if m.result.IsText {
		srcVal = "Text Snippet"
		preview := strings.ReplaceAll(m.result.TextContent, "\n", " ")
		if len(preview) > 40 {
			preview = preview[:40] + "..."
		}
		srcDesc = fmt.Sprintf("\"%s\"", preview)
	}
	items = append(items, confirmItem{label: "Source", value: srcVal, desc: srcDesc})

	// Mode
	modeVal := "Direct (P2P)"
	modeDesc := "Fastest · real-time · both online"
	if m.result.UseS3 {
		modeVal = "Cloud (S3)"
		modeDesc = "Async · pick up later · ≤200MB"
	}
	items = append(items, confirmItem{label: "Mode", value: modeVal, desc: modeDesc})

	// Options
	if m.result.ForceZip {
		items = append(items, confirmItem{label: "Compression", value: "ZIP", desc: "Bundle into .zip archive"})
	} else if m.result.ForceTar {
		items = append(items, confirmItem{label: "Compression", value: "TAR", desc: "Bundle into .tar.gz archive"})
	}

	if m.result.Incognito {
		items = append(items, confirmItem{label: "Privacy", value: "Incognito", desc: "No clipboard, no history"})
	} else {
		if m.result.NoClipboard {
			items = append(items, confirmItem{label: "Clipboard", value: "Disabled", desc: "Don't copy code"})
		}
		if m.result.NoHistory {
			items = append(items, confirmItem{label: "History", value: "Disabled", desc: "Skip audit log"})
		}
	}

	// Start Button
	items = append(items, confirmItem{label: "Enter to Start", isButton: true})

	return items

}

// ── Helpers ──

func (m SendWizardModel) renderRadio(label, desc string, focused, selected bool) string {
	indicator := "( )"
	style := RadioInactiveStyle
	prefix := " "
	icon := ">"

	if selected {
		indicator = "(*)"
		style = ToggleOnStyle // Green
	}
	if focused {
		style = RadioActiveStyle
		prefix = ">" // Becomes ">>"
	}

	labelRaw := fmt.Sprintf("%s%s %s %s", prefix, icon, indicator, label)
	line := style.Render(labelRaw)

	if focused && desc != "" {
		descStyle := lipgloss.NewStyle().
			Foreground(ColorSubtext).
			Faint(true).
			PaddingLeft(2)
		line = lipgloss.JoinHorizontal(lipgloss.Left, line, descStyle.Render(desc))
	}

	return line + "\n"
}

func (m SendWizardModel) renderToggle(label, desc string, focused, on bool) string {
	prefix := " "
	icon := ">"
	style := ToggleOffStyle
	toggle := "[ ]"
	if on {
		toggle = "[x]"
		style = ToggleOnStyle
	}
	if focused {
		style = RadioActiveStyle
		prefix = ">" // Becomes ">>"
	}

	labelRaw := fmt.Sprintf("%s%s %s %s", prefix, icon, toggle, label)
	line := style.Render(labelRaw)

	if focused && desc != "" {
		descStyle := lipgloss.NewStyle().
			Foreground(ColorSubtext).
			Faint(true).
			PaddingLeft(2)
		line = lipgloss.JoinHorizontal(lipgloss.Left, line, descStyle.Render(desc))
	}

	return line + "\n"
}

func (m SendWizardModel) viewStepDots() string {
	steps := []string{"Source", "Options", "Confirm"}
	var dots []string

	for i, name := range steps {
		dot := "○"
		style := StepDotInactive
		if WizardStep(i) == m.step {
			dot = "●"
			style = StepDotActive
		} else if WizardStep(i) < m.step {
			dot = "●"
			style = StepDotInactive
		}
		dots = append(dots, style.Render(fmt.Sprintf("%s %s", dot, name)))
	}

	return lipgloss.JoinHorizontal(lipgloss.Center, strings.Join(dots, "   "))
}

// ── Public API ──

// RunSendWizard launches the full wizard and returns the user's choices.
func RunSendWizard() (*WizardResult, error) {
	m := NewSendWizardModel()

	// Run step 1
	tm, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return nil, err
	}

	result := tm.(SendWizardModel)

	if result.result.Cancelled {
		return &WizardResult{Cancelled: true}, nil
	}

	// If source choice is "File" (1) or "Folder" (2) and no file was selected yet,
	// we need to launch the file picker outside of alt screen
	if (result.sourceChoice == 1 || result.sourceChoice == 2) && !result.result.IsText {
		isDirMode := (result.sourceChoice == 2)
		var previous []string // Track selections across back/forth

		for {
			selected, err := RunFilePicker(isDirMode, previous)
			if err != nil {
				return nil, err
			}
			if len(selected) == 0 {
				return &WizardResult{Cancelled: true}, nil
			}

			// Save for potential back-nav
			previous = selected

			if len(selected) == 1 {
				result.filePath = selected[0]
				result.fileName = filepath.Base(selected[0])
				fi, err := os.Stat(selected[0])
				if err == nil {
					result.fileSize = fi.Size()
				}
			} else {
				// Bundle multiple files into a temp directory
				tmpDir, err := os.MkdirTemp("", "jend-bundle-*")
				if err != nil {
					return nil, err
				}
				var totalSize int64
				for _, p := range selected {
					fi, err := os.Stat(p)
					if err != nil {
						continue
					}
					totalSize += fi.Size()
					dest := filepath.Join(tmpDir, filepath.Base(p))
					if fi.IsDir() {
						_ = copyDir(p, dest)
					} else {
						_ = copyFile(p, dest)
					}
				}
				result.filePath = tmpDir
				result.fileName = fmt.Sprintf("Multiple files (%d items)", len(selected))
				result.fileSize = totalSize

				// Force zip compression for multiple files
				result.forceZip = true
			}

			// Now re-launch wizard at step 2
			result.step = WizardStepOptions
			result.cursor = 0
			result.optCursor = 0
			result.confirmCursor = 0
			result.result.FilePath = result.filePath

			tm2, err := tea.NewProgram(result, tea.WithAltScreen()).Run()
			if err != nil {
				return nil, err
			}

			updatedResult := tm2.(SendWizardModel)

			if updatedResult.quitting || updatedResult.result.Cancelled {
				return &WizardResult{Cancelled: true}, nil
			}

			// If they navigated back to the source step, loop again
			if updatedResult.step == WizardStepSource {
				continue
			}

			// Otherwise, they confirmed correctly, break and return
			result = updatedResult
			break
		}
	}

	if result.result.Cancelled || result.quitting {
		return &WizardResult{Cancelled: true}, nil
	}

	// Final result
	r := &result.result
	r.FilePath = result.filePath
	return r, nil
}

// copyFile is a helper utility for multi-file bundling
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// copyDir is a helper utility for multi-file bundling
func copyDir(src string, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, info.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			err = copyDir(srcPath, dstPath)
		} else {
			err = copyFile(srcPath, dstPath)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
