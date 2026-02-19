package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// SendWizardModel is the main wizard Bubble Tea model
type SendWizardModel struct {
	step     WizardStep
	cursor   int
	quitting bool
	result   WizardResult

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
	optCursor   int
	modeChoice  int // 0 = Direct, 1 = S3
	forceZip    bool
	forceTar    bool
	incognito   bool
	noClipboard bool
	noHistory   bool

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

	return SendWizardModel{
		step: WizardStepSource,
		sourceOptions: []sourceOption{
			{label: "File or Folder", icon: ">", desc: "Browse and select a file to send"},
			{label: "Text Snippet", icon: ">", desc: "Type or paste text to send"},
		},
		sourceChoice: 0,
		modeChoice:   0,
		textArea:     ta,
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
		switch msg.Type {
		case tea.KeyCtrlC:
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
		if m.cursor == 0 {
			// File picker — we need to exit TUI, launch picker, and come back
			// Use a message to signal this
			return m, tea.Quit // Will be handled by RunSendWizard
		} else if m.cursor == 1 {
			// Text input
			m.textEditing = true
			m.textArea.Reset()
			m.textArea.Focus()
			return m, textarea.Blink
		}
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
		return m, nil
	}

	// Forward to textarea
	var cmd tea.Cmd
	m.textArea, cmd = m.textArea.Update(msg)
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
	case tea.KeyEsc:
		// Go back to step 1
		m.step = WizardStepSource
		m.cursor = m.sourceChoice
		m.filePath = ""
		m.fileName = ""
		m.textArea.Reset()
		m.result.IsText = false
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
				m.noClipboard = true
				m.noHistory = true
			}
		case 5: // No Clipboard toggle
			m.noClipboard = !m.noClipboard
			if !m.noClipboard {
				m.incognito = false
			}
		case 6: // No History toggle
			m.noHistory = !m.noHistory
			if !m.noHistory {
				m.incognito = false
			}
		}
	case tea.KeyTab:
		// Tab advances to confirm step
		m.result.UseS3 = (m.modeChoice == 1)
		m.result.ForceZip = m.forceZip
		m.result.ForceTar = m.forceTar
		m.result.Incognito = m.incognito
		m.result.NoClipboard = m.noClipboard
		m.result.NoHistory = m.noHistory
		m.step = WizardStepConfirm
		return m, nil
	}

	return m, nil
}

// ── Step 3: Confirm ──

func (m SendWizardModel) updateStepConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.step = WizardStepOptions
		m.optCursor = 0
		return m, nil
	case tea.KeyEnter:
		// Ship it!
		m.result.FilePath = m.filePath
		m.quitting = false
		return m, tea.Quit
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

	return ContainerStyle.Render(body)
}

func (m SendWizardModel) viewStepSource() string {
	var s strings.Builder

	header := WizardHeaderStyle.Render("What do you want to send?")
	s.WriteString(header)
	s.WriteString("\n\n")

	for i, opt := range m.sourceOptions {
		style := RadioInactiveStyle
		prefix := "  "
		if i == m.cursor {
			style = RadioActiveStyle
			prefix = "> "
		}

		line := fmt.Sprintf("%s%s  %s", prefix, opt.icon, opt.label)
		s.WriteString(style.Render(line))
		s.WriteString("\n")

		// Show description for active item
		if i == m.cursor {
			desc := lipgloss.NewStyle().
				Foreground(ColorSubtext).
				Faint(true).
				PaddingLeft(6).
				Render(opt.desc)
			s.WriteString(desc)
			s.WriteString("\n")
		}
		s.WriteString("\n")
	}

	help := WizardHelpStyle.Render("↑↓ navigate · enter select · esc quit")
	s.WriteString(help)

	return s.String()
}

func (m SendWizardModel) viewTextInput() string {
	var s strings.Builder

	header := WizardHeaderStyle.Render("Type your text")
	s.WriteString(header)
	s.WriteString("\n\n")

	s.WriteString(m.textArea.View())
	s.WriteString("\n\n")

	help := WizardHelpStyle.Render("tab submit · esc back")
	s.WriteString(help)

	return s.String()
}

func (m SendWizardModel) viewStepOptions() string {
	var s strings.Builder

	// File info header
	if m.result.IsText {
		header := WizardHeaderStyle.Render("Sending text snippet")
		s.WriteString(header)
	} else {
		sizeStr := FormatBytes(m.fileSize)
		header := WizardHeaderStyle.Render(fmt.Sprintf("%s (%s)", m.fileName, sizeStr))
		s.WriteString(header)
	}

	s.WriteString("\n\n")

	sectionStyle := lipgloss.NewStyle().
		Foreground(ColorText).
		Bold(true).
		MarginBottom(1)

	descStyle := lipgloss.NewStyle().
		Foreground(ColorSubtext).
		Faint(true).
		PaddingLeft(8)

	// -- Transfer Mode --
	s.WriteString(sectionStyle.Render("Transfer Mode"))
	s.WriteString("\n")

	s.WriteString(m.renderRadio("Direct (P2P)", "", m.optCursor == 0, m.modeChoice == 0))
	if m.optCursor == 0 {
		s.WriteString(descStyle.Render("Real-time transfer, both devices must be online"))
		s.WriteString("\n")
	}

	s.WriteString(m.renderRadio("Cloud (S3)", "", m.optCursor == 1, m.modeChoice == 1))
	if m.optCursor == 1 {
		s.WriteString(descStyle.Render("Upload to cloud, receiver downloads later (max 200MB)"))
		s.WriteString("\n")
	}

	s.WriteString("\n")

	// -- Compression --
	s.WriteString(sectionStyle.Render("Compression"))
	s.WriteString("\n")

	s.WriteString(m.renderToggle("Compress as ZIP", m.optCursor == 2, m.forceZip))
	if m.optCursor == 2 {
		s.WriteString(descStyle.Render("Bundle into a .zip archive before sending"))
		s.WriteString("\n")
	}

	s.WriteString(m.renderToggle("Compress as TAR", m.optCursor == 3, m.forceTar))
	if m.optCursor == 3 {
		s.WriteString(descStyle.Render("Bundle into a .tar.gz archive before sending"))
		s.WriteString("\n")
	}

	s.WriteString("\n")

	// -- Privacy --
	s.WriteString(sectionStyle.Render("Privacy"))
	s.WriteString("\n")

	s.WriteString(m.renderToggle("Incognito", m.optCursor == 4, m.incognito))
	if m.optCursor == 4 {
		s.WriteString(descStyle.Render("Disables clipboard copy and transfer history"))
		s.WriteString("\n")
	}

	s.WriteString(m.renderToggle("No clipboard", m.optCursor == 5, m.noClipboard))
	if m.optCursor == 5 {
		s.WriteString(descStyle.Render("Don't copy the transfer code to clipboard"))
		s.WriteString("\n")
	}

	s.WriteString(m.renderToggle("No history", m.optCursor == 6, m.noHistory))
	if m.optCursor == 6 {
		s.WriteString(descStyle.Render("Skip logging this transfer to audit history"))
		s.WriteString("\n")
	}

	s.WriteString("\n")
	help := WizardHelpStyle.Render("up/down navigate  |  enter toggle  |  tab continue  |  esc back")
	s.WriteString(help)

	return s.String()
}

func (m SendWizardModel) viewStepConfirm() string {
	var s strings.Builder

	header := WizardHeaderStyle.Render("Ready to send")
	s.WriteString(header)
	s.WriteString("\n")

	// Build card content
	var rows []string

	if m.result.IsText {
		preview := m.result.TextContent
		if len(preview) > 40 {
			preview = preview[:40] + "..."
		}
		rows = append(rows, m.confirmRow("Source", fmt.Sprintf("Text: \"%s\"", preview)))
	} else {
		rows = append(rows, m.confirmRow("File", m.fileName))
		rows = append(rows, m.confirmRow("Size", FormatBytes(m.fileSize)))
	}

	mode := "Direct (P2P)"
	if m.result.UseS3 {
		mode = "Cloud (S3)"
	}
	rows = append(rows, m.confirmRow("Mode", mode))

	if m.result.ForceZip {
		rows = append(rows, m.confirmRow("Compress", "ZIP"))
	} else if m.result.ForceTar {
		rows = append(rows, m.confirmRow("Compress", "TAR"))
	}

	// Privacy flags
	var privacy []string
	if m.result.Incognito {
		privacy = append(privacy, "Incognito")
	} else {
		if m.result.NoClipboard {
			privacy = append(privacy, "No clipboard")
		}
		if m.result.NoHistory {
			privacy = append(privacy, "No history")
		}
	}
	if len(privacy) > 0 {
		rows = append(rows, m.confirmRow("Privacy", strings.Join(privacy, ", ")))
	}

	cardContent := strings.Join(rows, "\n")
	card := ConfirmCardStyle.Render(cardContent)
	s.WriteString(card)
	s.WriteString("\n")

	// Start button
	btnStyle := lipgloss.NewStyle().
		Foreground(ColorBg).
		Background(ColorSecondary).
		Bold(true).
		Padding(0, 4).
		Align(lipgloss.Center)

	s.WriteString(btnStyle.Render("  Press Enter to Start  "))
	s.WriteString("\n\n")

	help := WizardHelpStyle.Render("enter start  |  esc back")
	s.WriteString(help)

	return s.String()
}

// ── Helpers ──

func (m SendWizardModel) renderRadio(label, desc string, focused, selected bool) string {
	var s strings.Builder

	indicator := "( )"
	style := RadioInactiveStyle
	prefix := "  "
	if selected {
		indicator = "(*)"
	}
	if focused {
		style = RadioActiveStyle
		prefix = "> "
	}

	s.WriteString(style.Render(fmt.Sprintf("%s%s  %s", prefix, indicator, label)))
	s.WriteString("\n")

	if focused {
		descStyle := lipgloss.NewStyle().
			Foreground(ColorSubtext).
			Faint(true).
			PaddingLeft(8)
		s.WriteString(descStyle.Render(desc))
		s.WriteString("\n")
	}

	return s.String()
}

func (m SendWizardModel) renderToggle(label string, focused, on bool) string {
	prefix := "  "
	style := ToggleOffStyle
	toggle := "[ ]"
	if on {
		toggle = "[x]"
		style = ToggleOnStyle
	}
	if focused {
		style = RadioActiveStyle
		prefix = "> "
	}

	return style.Render(fmt.Sprintf("%s%s  %s", prefix, toggle, label)) + "\n"
}

func (m SendWizardModel) confirmRow(label, value string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top,
		ConfirmLabelStyle.Render(label),
		ConfirmValueStyle.Render(value),
	)
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

	// If source choice is "File" and no file was selected yet,
	// we need to launch the file picker outside of alt screen
	if result.sourceChoice == 0 && !result.result.IsText {
		selected, err := RunFilePicker()
		if err != nil {
			return nil, err
		}
		if selected == "" {
			return &WizardResult{Cancelled: true}, nil
		}
		result.filePath = selected
		result.fileName = filepath.Base(selected)

		// Get file size
		fi, err := os.Stat(selected)
		if err == nil {
			result.fileSize = fi.Size()
		}

		// Now re-launch wizard at step 2
		result.step = WizardStepOptions
		result.cursor = 0
		result.optCursor = 0
		result.result.FilePath = selected

		tm2, err := tea.NewProgram(result, tea.WithAltScreen()).Run()
		if err != nil {
			return nil, err
		}
		result = tm2.(SendWizardModel)
	}

	if result.result.Cancelled || result.quitting {
		return &WizardResult{Cancelled: true}, nil
	}

	// Final result
	r := &result.result
	r.FilePath = result.filePath
	return r, nil
}
