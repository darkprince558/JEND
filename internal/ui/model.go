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
)

type Role int

const (
	RoleSender Role = iota
	RoleReceiver
)

// Messages
type StatusMsg string
type ErrorMsg error
type ProgressMsg struct {
	SentBytes  int64
	TotalBytes int64
	Speed      float64       // bytes per second
	ETA        time.Duration // estimated time remaining
	Protocol   string        // "Direct [LAN]" or similar
}

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
	Exit          bool
}

func NewModel(role Role, filename string, code string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ColorSecondary)

	// Custom Progress Bar Styles
	pTotal := progress.New(
		progress.WithGradient(string(ColorPrimary), string(ColorSecondary)),
		progress.WithWidth(40),
	)
	pFile := progress.New(
		progress.WithGradient("#00FF00", "#00FFFF"), // Different color for file
		progress.WithWidth(40),
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
		if msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc {
			m.Exit = true
			return m, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		return m, cmd

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

		// Update Telemetry
		m.Speed = fmt.Sprintf("%.2f MB/s", msg.Speed/1024/1024)
		m.ETA = msg.ETA.Round(time.Second).String()
		m.Protocol = msg.Protocol

		return m, tea.Batch(cmdTotal, cmdFile)

	case ErrorMsg:
		m.State = StateError
		m.Err = msg
		return m, tea.Quit
	}

	return m, nil
}

func (m Model) View() string {
	if m.Err != nil {
		return ContainerStyle.Render(
			lipgloss.JoinVertical(lipgloss.Left,
				ErrorStyle.Render("Error Occurred"),
				fmt.Sprintf("%v", m.Err),
			),
		)
	}

	var content string

	switch m.State {
	case StateStart, StateConnecting:
		// Matrix Style Handshake
		header := MatrixHeaderStyle.Render("JEND")

		info := ""
		if m.Role == RoleSender {
			info = ViewCode(m.Code)
		} else {
			info = MatrixTextStyle.Render(">> TERMINAL ACTIVE <<\n>> INITIALIZING... <<")
		}

		status := MatrixTextStyle.Render(fmt.Sprintf(">> %s", m.Status))

		content = lipgloss.JoinVertical(lipgloss.Center, header, info, m.Spinner.View(), status)

	case StateTransferring:
		header := TitleStyle.Render("Transfer In Progress")

		// Telemetry Grid
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
				StatValueStyle.Render(m.Protocol), // e.g. "QUIC [LAN]"
			),
		)

		bars := lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.JoinHorizontal(lipgloss.Bottom, StatLabelStyle.Render("Total Session"), m.TotalProgress.View()),
			" ", // spacer
			lipgloss.JoinHorizontal(lipgloss.Bottom, StatLabelStyle.Render("Current File "), m.FileProgress.View()),
		)

		content = lipgloss.JoinVertical(lipgloss.Center, header, telemetry, " ", bars)

	case StateDone:
		content = TitleStyle.Render("Transfer Complete!")
	}

	return ContainerStyle.Render(content)
}
