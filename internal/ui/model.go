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
	// Confirmation State
	ConfirmResp chan bool
	ConfirmName string
	ConfirmSize int64
}

func NewModel(role Role, filename string, code string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ColorSecondary)

	// Custom Progress Bar Styles (Neon Gradient)
	pTotal := progress.New(
		progress.WithGradient(string(ColorPrimary), string(ColorSecondary)),
		progress.WithWidth(50),
	)
	pFile := progress.New(
		progress.WithGradient(string(ColorSuccess), string(ColorSecondary)), // Green to Cyan
		progress.WithWidth(50),
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
			lipgloss.JoinVertical(lipgloss.Center,
				ErrorStyle.Render("ERROR"),
				lipgloss.NewStyle().Foreground(ColorError).Padding(1).Render(fmt.Sprintf("%v", m.Err)),
			),
		)
	}

	var content string

	switch m.State {
	case StateStart, StateConnecting:
		// Matrix Style Handshake
		header := MatrixHeaderStyle.Render("JEND SECURE LINK")

		info := ""
		if m.Role == RoleSender {
			info = ViewCode(m.Code)
		} else {
			info = MatrixTextStyle.Render(">> ESTABLISHING SECURE CONNECTION <<\n>> WAITING FOR PEER... <<")
		}

		// Centered Status with Spinner
		statusLine := lipgloss.JoinHorizontal(lipgloss.Center,
			m.Spinner.View(),
			" ",
			StatusStyle.Render(m.Status),
		)

		content = lipgloss.JoinVertical(lipgloss.Center,
			header,
			"\n",
			info,
			"\n",
			statusLine,
		)

	case StateTransferring:
		header := TitleStyle.Render("DATA TRANSFER IN P2P TUNNEL")

		// Telemetry Grid
		telemetry := lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.JoinVertical(lipgloss.Left,
				StatLabelStyle.Render("SPEED"),
				StatValueStyle.Render(m.Speed),
			),
			lipgloss.NewStyle().Width(2).Render(""),
			lipgloss.JoinVertical(lipgloss.Left,
				StatLabelStyle.Render("ETA"),
				StatValueStyle.Render(m.ETA),
			),
			lipgloss.NewStyle().Width(2).Render(""),
			lipgloss.JoinVertical(lipgloss.Left,
				StatLabelStyle.Render("PROTOCOL"),
				StatValueStyle.Render(m.Protocol),
			),
		)

		bars := lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.JoinHorizontal(lipgloss.Bottom, StatLabelStyle.Render("TOTAL"), m.TotalProgress.View()),
			" ", // spacer
			lipgloss.JoinHorizontal(lipgloss.Bottom, StatLabelStyle.Render("FILE "), m.FileProgress.View()),
		)

		content = lipgloss.JoinVertical(lipgloss.Center,
			header,
			"\n",
			telemetry,
			"\n",
			bars,
			"\n",
			StatusStyle.Render(m.Status),
		)

	case StateConfirm:
		header := TitleStyle.Render("INCOMING FILE REQUEST")

		infoBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorSecondary).
			Padding(1, 2).
			Render(
				fmt.Sprintf("Sender wants to send:\n\n%s\n%s",
					lipgloss.NewStyle().Bold(true).Render(m.ConfirmName),
					lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(fmt.Sprintf("%d MB", m.ConfirmSize/1024/1024)),
				),
			)

		prompt := lipgloss.NewStyle().Blink(true).Render("Accept? (y/n)")

		content = lipgloss.JoinVertical(lipgloss.Center,
			header,
			"\n",
			infoBox,
			"\n\n",
			prompt,
		)

	case StateDone:
		// Success state
		header := TitleStyle.Render("TRANSFER COMPLETE")
		check := lipgloss.NewStyle().Foreground(ColorSuccess).SetString("✔").String()
		msg := lipgloss.NewStyle().Foreground(ColorText).Render("All files transmitted successfully.")

		content = lipgloss.JoinVertical(lipgloss.Center,
			header,
			"\n",
			check+" "+msg,
		)
	}

	return ContainerStyle.Render(content)
}
