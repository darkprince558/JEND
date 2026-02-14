package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	Incognito   bool
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
	textInput   string
	textEditing bool

	// Step 1 sub: file selected
	filePath string
	fileSize int64
	fileName string

	// Step 2: Options
	optCursor  int
	modeChoice int // 0 = Direct, 1 = S3
	forceZip   bool
	incognito  bool

	// Terminal size
	width  int
	height int
}

// NewSendWizardModel creates the wizard model
func NewSendWizardModel() SendWizardModel {
	return SendWizardModel{
		step: WizardStepSource,
		sourceOptions: []sourceOption{
			{label: "File or Folder", icon: "📂", desc: "Browse and select a file to send"},
			{label: "Text Snippet", icon: "📝", desc: "Type or paste text to send"},
		},
		sourceChoice: 0,
		modeChoice:   0,
	}
}

func (m SendWizardModel) Init() tea.Cmd {
	return nil
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
			m.textInput = ""
			return m, nil
		}
	}
	return m, nil
}

// ── Text Input Handler ──

func (m SendWizardModel) handleTextInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.textEditing = false
		return m, nil
	case tea.KeyEnter:
		if msg.Alt {
			// Alt+Enter = newline
			m.textInput += "\n"
			return m, nil
		}
		// Submit text
		if strings.TrimSpace(m.textInput) == "" {
			return m, nil // Don't allow empty
		}
		m.result.TextContent = m.textInput
		m.result.IsText = true
		m.textEditing = false
		m.step = WizardStepOptions
		m.cursor = 0
		m.optCursor = 0
		return m, nil
	case tea.KeyBackspace:
		if len(m.textInput) > 0 {
			m.textInput = m.textInput[:len(m.textInput)-1]
		}
	case tea.KeyRunes:
		m.textInput += string(msg.Runes)
	case tea.KeySpace:
		m.textInput += " "
	}
	return m, nil
}

// ── Step 2: Options ──

func (m SendWizardModel) updateStepOptions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Items: [mode: Direct] [mode: S3] [---] [zip toggle] [incognito toggle]
	totalItems := 4 // Direct, S3, ZIP, Incognito

	switch msg.Type {
	case tea.KeyUp, tea.KeyShiftTab:
		if m.optCursor > 0 {
			m.optCursor--
		}
	case tea.KeyDown, tea.KeyTab:
		if m.optCursor < totalItems-1 {
			m.optCursor++
		}
	case tea.KeyEsc:
		// Go back to step 1
		m.step = WizardStepSource
		m.cursor = m.sourceChoice
		m.filePath = ""
		m.fileName = ""
		m.textInput = ""
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
		case 3: // Incognito toggle
			m.incognito = !m.incognito
		}
	case tea.KeyRight:
		// Continue to confirm
		m.result.UseS3 = (m.modeChoice == 1)
		m.result.ForceZip = m.forceZip
		m.result.Incognito = m.incognito
		m.step = WizardStepConfirm
		return m, nil
	}

	// Also allow 'n' for next
	if msg.Type == tea.KeyRunes {
		switch string(msg.Runes) {
		case "n":
			m.result.UseS3 = (m.modeChoice == 1)
			m.result.ForceZip = m.forceZip
			m.result.Incognito = m.incognito
			m.step = WizardStepConfirm
			return m, nil
		}
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
		cursor := "  "
		style := RadioInactiveStyle
		indicator := "○"
		if i == m.cursor {
			cursor = ""
			style = RadioActiveStyle
			indicator = "●"
		}

		line := fmt.Sprintf("%s %s %s  %s", cursor, indicator, opt.icon, opt.label)
		s.WriteString(style.Render(line))
		s.WriteString("\n")

		// Show description for active item
		if i == m.cursor {
			desc := lipgloss.NewStyle().
				Foreground(ColorSubtext).
				Faint(true).
				PaddingLeft(7).
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

	header := WizardHeaderStyle.Render("📝 Type your text")
	s.WriteString(header)
	s.WriteString("\n\n")

	// Text area with cursor
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorAccent).
		Padding(1, 2).
		Width(50)

	displayText := m.textInput + "█"
	if m.textInput == "" {
		displayText = lipgloss.NewStyle().
			Foreground(ColorSubtext).
			Faint(true).
			Render("Start typing...") + "█"
	}

	s.WriteString(boxStyle.Render(displayText))
	s.WriteString("\n\n")

	help := WizardHelpStyle.Render("enter submit · esc back")
	s.WriteString(help)

	return s.String()
}

func (m SendWizardModel) viewStepOptions() string {
	var s strings.Builder

	// File info header
	if m.result.IsText {
		header := WizardHeaderStyle.Render("📝 Sending text snippet")
		s.WriteString(header)
	} else {
		sizeStr := FormatBytes(m.fileSize)
		header := WizardHeaderStyle.Render(fmt.Sprintf("📂 %s (%s)", m.fileName, sizeStr))
		s.WriteString(header)
	}

	s.WriteString("\n\n")

	// Section: Transfer Mode
	sectionStyle := lipgloss.NewStyle().
		Foreground(ColorText).
		Bold(true).
		MarginBottom(1)
	s.WriteString(sectionStyle.Render("Transfer Mode"))
	s.WriteString("\n")

	// Direct option
	directRadio := m.renderRadio("Direct (P2P)", "Fastest · real-time · both online", m.optCursor == 0, m.modeChoice == 0)
	s.WriteString(directRadio)

	// S3 option
	s3Radio := m.renderRadio("Cloud (S3)", "Async · pick up later · ≤200MB", m.optCursor == 1, m.modeChoice == 1)
	s.WriteString(s3Radio)

	s.WriteString("\n")

	// Section: Options
	s.WriteString(sectionStyle.Render("Options"))
	s.WriteString("\n")

	// ZIP Toggle
	zipToggle := m.renderToggle("Compress as ZIP", m.optCursor == 2, m.forceZip)
	s.WriteString(zipToggle)

	// Incognito Toggle
	incognitoToggle := m.renderToggle("Incognito mode", m.optCursor == 3, m.incognito)
	s.WriteString(incognitoToggle)

	s.WriteString("\n")
	help := WizardHelpStyle.Render("↑↓ navigate · space toggle · → or n next · esc back")
	s.WriteString(help)

	return s.String()
}

func (m SendWizardModel) viewStepConfirm() string {
	var s strings.Builder

	header := WizardHeaderStyle.Render("✅ Ready to send!")
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

	// Options
	var opts []string
	if m.result.ForceZip {
		opts = append(opts, "ZIP")
	}
	if m.result.Incognito {
		opts = append(opts, "Incognito")
	}
	if len(opts) == 0 {
		opts = append(opts, "None")
	}
	rows = append(rows, m.confirmRow("Options", strings.Join(opts, ", ")))

	cardContent := strings.Join(rows, "\n")
	card := ConfirmCardStyle.Render(cardContent)
	s.WriteString(card)
	s.WriteString("\n")

	// Big start button
	btnStyle := lipgloss.NewStyle().
		Foreground(ColorBg).
		Background(ColorSecondary).
		Bold(true).
		Padding(0, 4).
		Align(lipgloss.Center)

	s.WriteString(btnStyle.Render("  Press Enter to Start  "))
	s.WriteString("\n\n")

	help := WizardHelpStyle.Render("enter start · esc back")
	s.WriteString(help)

	return s.String()
}

// ── Helpers ──

func (m SendWizardModel) renderRadio(label, desc string, focused, selected bool) string {
	var s strings.Builder

	indicator := "○"
	style := RadioInactiveStyle
	cursor := "  "
	if selected {
		indicator = "●"
	}
	if focused {
		style = RadioActiveStyle
		cursor = ""
	}

	s.WriteString(style.Render(fmt.Sprintf("%s %s  %s", cursor, indicator, label)))
	s.WriteString("\n")

	if focused {
		descStyle := lipgloss.NewStyle().
			Foreground(ColorSubtext).
			Faint(true).
			PaddingLeft(7)
		s.WriteString(descStyle.Render(desc))
		s.WriteString("\n")
	}

	return s.String()
}

func (m SendWizardModel) renderToggle(label string, focused, on bool) string {
	cursor := "  "
	style := ToggleOffStyle
	toggle := "☐"
	if on {
		toggle = "☑"
		style = ToggleOnStyle
	}
	if focused {
		style = RadioActiveStyle
		cursor = ""
	}

	return style.Render(fmt.Sprintf("%s %s  %s", cursor, toggle, label)) + "\n"
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
