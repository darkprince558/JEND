package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type State int

const (
	StateStart State = iota
	StateConnecting
	StateTransferring
	StateDone
	StateError
	StateConfirm
)

type Role int

const (
	RoleSender Role = iota
	RoleReceiver
)

// Messages
type StatusMsg string
type DetailedErrorMsg struct {
	Err   error
	Level ErrorLevel
}

func (e DetailedErrorMsg) Error() string {
	return e.Err.Error()
}

type ErrorMsg error
type TextReceivedMsg struct {
	Content     string
	ClipboardOk bool
}
type RequestApprovalMsg struct {
	Name string
	Size int64
	Resp chan bool
}

type ProgressMsg struct {
	SentBytes  int64
	TotalBytes int64
	Speed      float64       // bytes per second
	ETA        time.Duration // estimated time remaining
	Protocol   string        // "Direct [LAN]" or similar
}

// Error Levels
type ErrorLevel int

const (
	LevelInfo ErrorLevel = iota
	LevelWarning
	LevelError
	LevelFatal
)

type Model struct {
	Role          Role
	State         State
	Filename      string
	Code          string
	Address       string
	Spinner       spinner.Model
	TotalProgress progress.Model
	FileProgress  progress.Model
	Speed         string
	ETA           string
	Protocol      string
	Status        string
	Err           error
	ErrLevel      ErrorLevel
	Exit          bool
	SentBytes     int64
	TotalBytes    int64
	// Text receive
	TextContent  string
	ReceivedFile string
	ClipboardOk  bool
	TextViewport viewport.Model
	// Confirmation State
	ConfirmResp chan bool
	ConfirmName string
	ConfirmSize int64
	// Terminal size
	Width  int
	Height int
}

func NewModel(role Role, filename string, code string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ColorAccent)

	// Custom Progress Bar Styles (Minimal Gradient)
	pTotal := progress.New(
		progress.WithGradient(string(ColorPrimary), string(ColorAccent)),
		progress.WithWidth(60),
	)
	pFile := progress.New(
		progress.WithGradient(string(ColorSecondary), string(ColorAccent)), // Green to Cyan
		progress.WithWidth(60),
	)

	return Model{
		Role:          role,
		State:         StateStart,
		Filename:      filename,
		Code:          code,
		Spinner:       s,
		TotalProgress: pTotal,
		FileProgress:  pFile,
		Speed:         "0 MB/s",
		ETA:           "Calculating...",
		Protocol:      "Initializing...",
	}
}

func (m Model) Init() tea.Cmd {
	return m.Spinner.Tick
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// If in Done state with text, forward scroll keys to viewport
		if m.State == StateDone && m.TextContent != "" {
			switch msg.String() {
			case "q", "esc", "ctrl+c":
				m.Exit = true
				return m, tea.Quit
			case "c":
				// Copy text to clipboard
				if err := clipboard.WriteAll(m.TextContent); err == nil {
					m.ClipboardOk = true
				}
				return m, nil
			default:
				var cmd tea.Cmd
				m.TextViewport, cmd = m.TextViewport.Update(msg)
				return m, cmd
			}
		}

		// If showing a non-fatal error or done state, allow exit or dismissal
		if m.State == StateError && m.ErrLevel != LevelFatal {
			if msg.String() == "esc" || msg.String() == "enter" {
				m.State = StateStart // Return to start or previous state? For now, start/connecting
				if m.Role == RoleReceiver || m.Role == RoleSender {
					m.State = StateConnecting
				}
				m.Err = nil
				return m, nil
			}
		}

		if m.State == StateConfirm {
			if msg.String() == "y" || msg.String() == "Y" {
				m.ConfirmResp <- true
				m.State = StateTransferring // Optimistic
				return m, nil
			} else if msg.String() == "n" || msg.String() == "N" {
				m.ConfirmResp <- false
				m.Exit = true
				return m, tea.Quit
			}
		}

		if msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc {
			if m.State == StateConfirm {
				m.ConfirmResp <- false // Safety cancel
			}
			m.Exit = true
			return m, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		return m, cmd

	case RequestApprovalMsg:
		m.State = StateConfirm
		m.ConfirmName = msg.Name
		m.ConfirmSize = msg.Size
		m.ConfirmResp = msg.Resp
		return m, nil

	case progress.FrameMsg:
		// Update both bars (animations)
		newTotal, cmdTotal := m.TotalProgress.Update(msg)
		newFile, cmdFile := m.FileProgress.Update(msg)
		m.TotalProgress = newTotal.(progress.Model)
		m.FileProgress = newFile.(progress.Model)
		return m, tea.Batch(cmdTotal, cmdFile)

	case TextReceivedMsg:
		m.TextContent = msg.Content
		m.ClipboardOk = msg.ClipboardOk
		m.State = StateDone // Show text UI immediately

		// Format content with clickable links
		formattedContent := FormatTextWithLinks(msg.Content)

		// Initialize viewport for text display
		vpWidth := 56
		vpHeight := 8
		if m.Width > 0 {
			vpWidth = m.Width - 24
			if vpWidth > 60 {
				vpWidth = 60
			}
			if vpWidth < 30 {
				vpWidth = 30
			}
		}
		if m.Height > 0 {
			vpHeight = m.Height / 3
			if vpHeight > 12 {
				vpHeight = 12
			}
			if vpHeight < 4 {
				vpHeight = 4
			}
		}
		m.TextViewport = viewport.New(vpWidth, vpHeight)
		m.TextViewport.SetContent(formattedContent)
		return m, nil // Don't quit — let user read the text

	case StatusMsg:
		m.Status = string(msg)
		if m.State == StateStart {
			m.State = StateConnecting
		}
		// Track received filename from status messages
		if strings.HasPrefix(string(msg), "Saved to: ") {
			m.ReceivedFile = strings.TrimPrefix(string(msg), "Saved to: ")
		}

	case ProgressMsg:
		// If we already showed text, ignore progress (user quits manually)
		if m.TextContent != "" {
			return m, nil
		}

		m.State = StateTransferring
		ratio := float64(msg.SentBytes) / float64(msg.TotalBytes)

		if ratio >= 1.0 {
			m.State = StateDone
			return m, tea.Quit
		}

		cmdTotal := m.TotalProgress.SetPercent(ratio)
		cmdFile := m.FileProgress.SetPercent(ratio) // Same for single file

		// Track bytes for display
		m.SentBytes = msg.SentBytes
		m.TotalBytes = msg.TotalBytes

		// Update Telemetry
		m.Speed = fmt.Sprintf("%.2f MB/s", msg.Speed/1024/1024)
		m.ETA = msg.ETA.Round(time.Second).String()
		m.Protocol = msg.Protocol

		return m, tea.Batch(cmdTotal, cmdFile)

	case DetailedErrorMsg:
		m.Err = msg.Err
		m.ErrLevel = msg.Level
		m.State = StateError
		if m.ErrLevel == LevelFatal {
			return m, tea.Quit
		}
		// For non-fatal, we stay in the loop to show the error
		return m, nil

	case ErrorMsg:
		// Legacy support: Treat as Fatal
		m.Err = msg
		m.ErrLevel = LevelFatal
		m.State = StateError
		return m, tea.Quit
	}

	return m, nil
}

func (m Model) View() string {
	// Helper for consistent vertical centering
	width := m.Width
	height := m.Height
	if width == 0 {
		width = 80
	}
	if height == 0 {
		height = 24
	}

	hintStyle := lipgloss.NewStyle().
		Foreground(ColorSubtext).
		Faint(true).
		Align(lipgloss.Center)

	if m.State == StateError {
		banner := RenderBanner()

		headerText := "ERROR"
		headerStyle := ErrorStyle
		msgColor := ColorError

		if m.ErrLevel == LevelWarning {
			headerText = "WARNING"
			headerStyle = WarningStyle
			msgColor = ColorWarning
		} else if m.ErrLevel == LevelInfo {
			headerText = "INFO"
			headerStyle = InfoStyle
			msgColor = ColorSecondary
		}

		header := headerStyle.Copy().Padding(1, 4).Render(headerText)

		content := lipgloss.JoinVertical(lipgloss.Center,
			banner,
			"",
			header,
			"",
			lipgloss.NewStyle().Foreground(msgColor).Width(60).Align(lipgloss.Center).Render(fmt.Sprintf("%v", m.Err)),
		)

		if m.ErrLevel != LevelFatal {
			content = lipgloss.JoinVertical(lipgloss.Center,
				content,
				"",
				hintStyle.Render("enter continue  ·  esc quit"),
			)
		}

		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
	}

	var content string

	switch m.State {
	case StateStart, StateConnecting:
		header := RenderBannerWithTagline()

		info := ""
		if m.Role == RoleSender {
			info = ViewCode(m.Code)
		} else {
			info = SubTextStyle.Render("waiting for connection...")
		}

		statusLine := lipgloss.JoinHorizontal(lipgloss.Center,
			m.Spinner.View(),
			" ",
			StatusStyle.Render(m.Status),
		)

		content = lipgloss.JoinVertical(lipgloss.Center,
			header,
			"",
			info,
			"",
			statusLine,
		)

	case StateTransferring:
		banner := RenderBanner()
		header := SectionHeaderStyle.Render("TRANSFERRING")

		telemetry := lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.JoinVertical(lipgloss.Left,
				StatLabelStyle.Render("SPEED"),
				StatValueStyle.Render(m.Speed),
			),
			lipgloss.NewStyle().Width(4).Render(""),
			lipgloss.JoinVertical(lipgloss.Left,
				StatLabelStyle.Render("ETA"),
				StatValueStyle.Render(m.ETA),
			),
			lipgloss.NewStyle().Width(4).Render(""),
			lipgloss.JoinVertical(lipgloss.Left,
				StatLabelStyle.Render("VIA"),
				StatValueStyle.Render(m.Protocol),
			),
		)

		bars := lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.JoinHorizontal(lipgloss.Bottom, StatLabelStyle.Render("TOTAL"), m.TotalProgress.View()),
			lipgloss.NewStyle().Foreground(ColorSubtext).PaddingLeft(11).Render(
				FormatBytes(m.SentBytes)+" / "+FormatBytes(m.TotalBytes),
			),
		)

		content = lipgloss.JoinVertical(lipgloss.Center,
			banner,
			"",
			header,
			"",
			telemetry,
			"",
			bars,
			"",
			StatusStyle.Render(m.Status),
		)

	case StateConfirm:
		banner := RenderBanner()
		header := SectionHeaderStyle.Render("INCOMING FILE")

		nameRow := lipgloss.JoinHorizontal(lipgloss.Top,
			ConfirmLabelStyle.Render("Name"),
			ConfirmValueStyle.Render(m.ConfirmName),
		)
		sizeRow := lipgloss.JoinHorizontal(lipgloss.Top,
			ConfirmLabelStyle.Render("Size"),
			ConfirmValueStyle.Render(FormatBytes(m.ConfirmSize)),
		)
		infoBox := ConfirmCardStyle.Render(
			lipgloss.JoinVertical(lipgloss.Left, nameRow, sizeRow),
		)

		prompt := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render("Accept? (y/n)")

		content = lipgloss.JoinVertical(lipgloss.Center,
			banner,
			"",
			header,
			"",
			infoBox,
			"",
			prompt,
		)

	case StateDone:
		if m.TextContent != "" {
			banner := RenderBanner()

			successLine := lipgloss.NewStyle().
				Foreground(ColorSuccess).
				Bold(true).
				Align(lipgloss.Center).
				Render("Text Received")

			vpBorder := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPrimary).
				Padding(0, 1).
				Foreground(ColorText)

			scrollHint := ""
			if m.TextViewport.TotalLineCount() > m.TextViewport.VisibleLineCount() {
				pct := int(m.TextViewport.ScrollPercent() * 100)
				scrollHint = hintStyle.Render(fmt.Sprintf("scroll %d%%", pct))
			}

			clipLine := ""
			if m.ClipboardOk {
				clipLine = lipgloss.NewStyle().Foreground(ColorSecondary).Align(lipgloss.Center).Render("Copied to clipboard")
			}

			content = lipgloss.JoinVertical(lipgloss.Center,
				banner,
				"",
				successLine,
				"",
				vpBorder.Render(m.TextViewport.View()),
				scrollHint,
				"",
				clipLine,
				"",
				hintStyle.Render("c copy  ·  q quit"),
			)
		} else {
			banner := RenderBanner()
			header := SectionHeaderStyle.Render("COMPLETE")

			var details string
			if m.ReceivedFile != "" {
				details = lipgloss.JoinVertical(lipgloss.Center,
					lipgloss.NewStyle().Foreground(ColorText).Bold(true).Render("Transfer complete"),
					"",
					lipgloss.NewStyle().Foreground(ColorSubtext).Render("Saved as: ")+lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render(m.ReceivedFile),
					"",
					lipgloss.NewStyle().Foreground(ColorSubtext).Render(FormatBytes(m.TotalBytes)+" received"),
				)
			} else if m.Role == RoleSender {
				details = lipgloss.NewStyle().Foreground(ColorText).Bold(true).Render("File sent successfully")
			} else {
				details = lipgloss.NewStyle().Foreground(ColorText).Bold(true).Render("All files transmitted successfully")
			}

			content = lipgloss.JoinVertical(lipgloss.Center,
				banner,
				"",
				header,
				"",
				details,
			)
		}
	}

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}
