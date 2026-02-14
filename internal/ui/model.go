package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
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
	// Confirmation State
	ConfirmResp chan bool
	ConfirmName string
	ConfirmSize int64
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
	case tea.KeyMsg:
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

	case StatusMsg:
		m.Status = string(msg)
		if m.State == StateStart {
			m.State = StateConnecting
		}

	case ProgressMsg:
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
	if m.State == StateError {
		header := ErrorStyle.Render("ERROR")
		msgColor := ColorError

		if m.ErrLevel == LevelWarning {
			header = WarningStyle.Render("WARNING")
			msgColor = ColorWarning
		} else if m.ErrLevel == LevelInfo {
			header = InfoStyle.Render("INFO")
			msgColor = ColorSecondary
		}

		content := lipgloss.JoinVertical(lipgloss.Center,
			header,
			lipgloss.NewStyle().Foreground(msgColor).Padding(1).Render(fmt.Sprintf("%v", m.Err)),
		)

		if m.ErrLevel != LevelFatal {
			content = lipgloss.JoinVertical(lipgloss.Center,
				content,
				lipgloss.NewStyle().Faint(true).Foreground(ColorSubtext).Render("Press Enter to continue..."),
			)
		}

		return ContainerStyle.Render(content)
	}

	var content string

	switch m.State {
	case StateStart, StateConnecting:
		// Clean Header
		header := TitleStyle.Render("JEND")

		info := ""
		if m.Role == RoleSender {
			info = ViewCode(m.Code)
		} else {
			info = SubTextStyle.Render("waiting for connection...")
		}

		// Centered Status with Spinner
		statusLine := lipgloss.JoinHorizontal(lipgloss.Center,
			m.Spinner.View(),
			" ",
			StatusStyle.Render(m.Status),
		)

		content = lipgloss.JoinVertical(lipgloss.Center,
			header,
			"\n\n",
			info,
			"\n\n",
			statusLine,
		)

	case StateTransferring:
		header := TitleStyle.Render("TRANSFERRING")

		// Telemetry Grid - Minimal
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
				StatLabelStyle.Render("PROTOCOL"),
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
			header,
			"\n\n",
			telemetry,
			"\n\n",
			bars,
			"\n\n",
			StatusStyle.Render(m.Status),
		)

	case StateConfirm:
		header := TitleStyle.Render("INCOMING FILE")

		// Borderless Info Box
		infoBox := lipgloss.NewStyle().
			Padding(1, 0).
			Render(
				fmt.Sprintf("Sender wants to send:\n\n%s\n%s",
					lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(m.ConfirmName),
					lipgloss.NewStyle().Foreground(ColorSubtext).Render(fmt.Sprintf("%d MB", m.ConfirmSize/1024/1024)),
				),
			)

		prompt := lipgloss.NewStyle().Foreground(ColorAccent).Blink(true).Render("Accept? (y/n)")

		content = lipgloss.JoinVertical(lipgloss.Center,
			header,
			"\n",
			infoBox,
			"\n\n",
			prompt,
		)

	case StateDone:
		// Success state
		header := TitleStyle.Render("COMPLETE")
		check := lipgloss.NewStyle().Foreground(ColorSuccess).SetString("✔").String()
		msg := lipgloss.NewStyle().Foreground(ColorText).Render("All files transmitted successfully.")

		content = lipgloss.JoinVertical(lipgloss.Center,
			header,
			"\n\n",
			check+" "+msg,
		)
	}

	return ContainerStyle.Render(content)
}
