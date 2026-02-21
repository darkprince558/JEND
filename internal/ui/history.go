package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/darkprince558/jend/internal/audit"
)

// HistoryModel is the interactive Bubble Tea model for the history viewer.
type HistoryModel struct {
	entries  []audit.LogEntry // all entries from disk
	filtered []audit.LogEntry // entries after search + filter
	cursor   int              // selected row in filtered list
	offset   int              // scroll offset for visible rows
	width    int
	height   int
	quitting bool

	// Search
	searchMode  bool
	searchInput textinput.Model
	searchQuery string

	// Filter
	roleFilter int // 0=all, 1=sent, 2=received

	// Sort
	sortField int // 0=date, 1=size, 2=duration
	sortAsc   bool

	// Delete confirmation
	confirmDelete bool

	// Deleted entry (for undo feedback)
	deleted bool
}

// NewHistoryModel creates a new interactive history model.
func NewHistoryModel(entries []audit.LogEntry) HistoryModel {
	ti := textinput.New()
	ti.Placeholder = "Search files, codes..."
	ti.CharLimit = 64
	ti.Width = 40

	m := HistoryModel{
		entries:     entries,
		searchInput: ti,
		sortField:   0,
		sortAsc:     false,
	}
	m.applyFilters()
	return m
}

func (m HistoryModel) Init() tea.Cmd {
	return nil
}

func (m HistoryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Search mode input
		if m.searchMode {
			switch msg.Type {
			case tea.KeyEsc:
				m.searchMode = false
				m.searchInput.Blur()
				return m, nil
			case tea.KeyEnter:
				m.searchQuery = m.searchInput.Value()
				m.searchMode = false
				m.searchInput.Blur()
				m.cursor = 0
				m.offset = 0
				m.applyFilters()
				return m, nil
			default:
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				// Live filter as you type
				m.searchQuery = m.searchInput.Value()
				m.cursor = 0
				m.offset = 0
				m.applyFilters()
				return m, cmd
			}
		}

		// Delete confirmation
		if m.confirmDelete {
			switch msg.String() {
			case "y", "Y":
				if len(m.filtered) > 0 {
					entry := m.filtered[m.cursor]
					// Remove from entries
					newEntries := make([]audit.LogEntry, 0, len(m.entries)-1)
					for _, e := range m.entries {
						if e.ID != entry.ID {
							newEntries = append(newEntries, e)
						}
					}
					m.entries = newEntries
					_ = audit.RewriteHistory(m.entries)
					m.applyFilters()
					if m.cursor >= len(m.filtered) && m.cursor > 0 {
						m.cursor--
					}
					m.deleted = true
				}
				m.confirmDelete = false
				return m, nil
			default:
				m.confirmDelete = false
				return m, nil
			}
		}

		m.deleted = false

		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.adjustScroll()
			}
		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m.adjustScroll()
			}
		case "pgup":
			m.cursor -= m.visibleRows()
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.adjustScroll()
		case "pgdown":
			m.cursor += m.visibleRows()
			if m.cursor >= len(m.filtered) {
				m.cursor = len(m.filtered) - 1
			}
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.adjustScroll()
		case "home", "g":
			m.cursor = 0
			m.offset = 0
		case "end", "G":
			m.cursor = len(m.filtered) - 1
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.adjustScroll()
		case "/":
			m.searchMode = true
			m.searchInput.Focus()
			return m, textinput.Blink
		case "esc":
			if m.searchQuery != "" {
				m.searchQuery = ""
				m.searchInput.SetValue("")
				m.cursor = 0
				m.offset = 0
				m.applyFilters()
			}
		case "tab":
			m.roleFilter = (m.roleFilter + 1) % 3
			m.cursor = 0
			m.offset = 0
			m.applyFilters()
		case "s":
			m.sortField = (m.sortField + 1) % 3
			m.applyFilters()
		case "r":
			m.sortAsc = !m.sortAsc
			m.applyFilters()
		case "d":
			if len(m.filtered) > 0 {
				m.confirmDelete = true
			}
		}
	}

	return m, nil
}

func (m *HistoryModel) adjustScroll() {
	visible := m.visibleRows()
	if visible <= 0 {
		visible = 10
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
}

func (m HistoryModel) visibleRows() int {
	// Header(2) + stats(3) + search(2) + detail(10) + help(2) + margins(4)
	overhead := 23
	rows := m.height - overhead
	if rows < 3 {
		rows = 3
	}
	return rows
}

func (m HistoryModel) View() string {
	if m.quitting {
		return ""
	}

	w := m.width
	if w == 0 {
		w = 100
	}
	h := m.height
	if h == 0 {
		h = 30
	}

	var s strings.Builder

	// ── Header ──
	headerText := lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true).
		Render("TRANSFER HISTORY")

	countText := lipgloss.NewStyle().
		Foreground(ColorSubtext).
		Render(fmt.Sprintf("%d transfers", len(m.filtered)))

	header := lipgloss.JoinHorizontal(lipgloss.Top,
		headerText,
		lipgloss.NewStyle().Width(3).Render(""),
		countText,
	)
	s.WriteString(header)
	s.WriteString("\n")

	// ── Stats Bar ──
	s.WriteString(m.renderStats())
	s.WriteString("\n")

	// ── Filter + Search Bar ──
	filterLabels := []string{"All", "Sent", "Received"}
	var filterParts []string
	for i, label := range filterLabels {
		if i == m.roleFilter {
			filterParts = append(filterParts, lipgloss.NewStyle().
				Foreground(ColorAccent).Bold(true).Render("["+label+"]"))
		} else {
			filterParts = append(filterParts, lipgloss.NewStyle().
				Foreground(ColorSubtext).Render(" "+label+" "))
		}
	}
	filterBar := strings.Join(filterParts, " ")

	sortLabels := []string{"Date", "Size", "Duration"}
	sortDir := "v"
	if m.sortAsc {
		sortDir = "^"
	}
	sortInfo := lipgloss.NewStyle().Foreground(ColorSubtext).
		Render(fmt.Sprintf("Sort: %s %s", sortLabels[m.sortField], sortDir))

	s.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
		filterBar,
		lipgloss.NewStyle().Width(4).Render(""),
		sortInfo,
	))
	s.WriteString("\n")

	if m.searchMode {
		s.WriteString(lipgloss.NewStyle().Foreground(ColorAccent).Render("/") + " " + m.searchInput.View())
		s.WriteString("\n")
	} else if m.searchQuery != "" {
		s.WriteString(lipgloss.NewStyle().Foreground(ColorSubtext).Faint(true).
			Render(fmt.Sprintf("Searching: \"%s\"  (esc to clear)", m.searchQuery)))
		s.WriteString("\n")
	} else {
		s.WriteString("\n")
	}

	// ── Divider ──
	divider := lipgloss.NewStyle().Foreground(ColorPanel).
		Render(strings.Repeat("─", min(w-4, 90)))
	s.WriteString(divider)
	s.WriteString("\n")

	// ── List ──
	if len(m.filtered) == 0 {
		empty := lipgloss.NewStyle().Foreground(ColorSubtext).Faint(true).
			Render("  No transfers found.")
		s.WriteString(empty)
		s.WriteString("\n\n")
	} else {
		visible := m.visibleRows()
		end := m.offset + visible
		if end > len(m.filtered) {
			end = len(m.filtered)
		}

		for i := m.offset; i < end; i++ {
			e := m.filtered[i]
			selected := i == m.cursor

			s.WriteString(m.renderRow(e, selected))
			s.WriteString("\n")
		}

		// Scroll indicator
		if len(m.filtered) > visible {
			pct := 0
			if len(m.filtered) > 1 {
				pct = m.cursor * 100 / (len(m.filtered) - 1)
			}
			scrollHint := lipgloss.NewStyle().Foreground(ColorSubtext).Faint(true).
				Render(fmt.Sprintf("  %d/%d (%d%%)", m.cursor+1, len(m.filtered), pct))
			s.WriteString(scrollHint)
			s.WriteString("\n")
		}
	}

	// ── Detail Panel ──
	s.WriteString(divider)
	s.WriteString("\n")
	if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
		s.WriteString(m.renderDetail(m.filtered[m.cursor]))
	}

	// ── Delete confirmation ──
	if m.confirmDelete {
		s.WriteString("\n")
		s.WriteString(lipgloss.NewStyle().Foreground(ColorError).Bold(true).
			Render("  Delete this entry? (y/n)"))
		s.WriteString("\n")
	}

	// ── Deleted feedback ──
	if m.deleted {
		s.WriteString(lipgloss.NewStyle().Foreground(ColorSecondary).
			Render("  Entry removed."))
		s.WriteString("\n")
	}

	// ── Help ──
	s.WriteString("\n")
	helpStyle := lipgloss.NewStyle().Foreground(ColorSubtext).Faint(true)
	s.WriteString(helpStyle.Render("  j/k scroll  tab filter  / search  s sort  r reverse  d delete  q quit"))
	s.WriteString("\n")

	return lipgloss.NewStyle().Padding(1, 2).Render(s.String())
}

// ── Rendering Helpers ──

func (m HistoryModel) renderStats() string {
	totalTransfers := len(m.entries)
	var totalBytes int64
	var totalDuration float64
	successCount := 0
	sentCount := 0
	recvCount := 0

	for _, e := range m.entries {
		totalBytes += e.FileSize
		totalDuration += e.Duration
		if e.Status == "success" {
			successCount++
		}
		if e.Role == "sender" {
			sentCount++
		} else {
			recvCount++
		}
	}

	successRate := 0
	if totalTransfers > 0 {
		successRate = successCount * 100 / totalTransfers
	}

	labelStyle := lipgloss.NewStyle().Foreground(ColorSubtext).Faint(true)
	valueStyle := lipgloss.NewStyle().Foreground(ColorText).Bold(true)

	parts := []string{
		labelStyle.Render("Total ") + valueStyle.Render(fmt.Sprintf("%d", totalTransfers)),
		labelStyle.Render("Sent ") + valueStyle.Foreground(lipgloss.Color("#FFA500")).Render(fmt.Sprintf("%d", sentCount)),
		labelStyle.Render("Received ") + valueStyle.Foreground(ColorAccent).Render(fmt.Sprintf("%d", recvCount)),
		labelStyle.Render("Data ") + valueStyle.Render(FormatBytes(totalBytes)),
		labelStyle.Render("Success ") + valueStyle.Foreground(ColorSecondary).Render(fmt.Sprintf("%d%%", successRate)),
	}

	return lipgloss.NewStyle().PaddingLeft(1).Render(strings.Join(parts, "    "))
}

func (m HistoryModel) renderRow(e audit.LogEntry, selected bool) string {
	// Role indicator
	var roleIcon string
	if e.Role == "sender" {
		roleIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Render("SEND")
	} else {
		roleIcon = lipgloss.NewStyle().Foreground(ColorAccent).Render("RECV")
	}

	// Status
	var statusDot string
	if e.Status == "success" {
		statusDot = lipgloss.NewStyle().Foreground(ColorSecondary).Render("*")
	} else {
		statusDot = lipgloss.NewStyle().Foreground(ColorError).Render("x")
	}

	// File name (truncated)
	name := e.FileName
	if len(name) > 28 {
		name = name[:25] + "..."
	}

	// Duration
	dur := fmt.Sprintf("%.1fs", e.Duration)

	// Time
	ts := m.formatRelativeTime(e.Timestamp)

	// Size
	size := FormatBytes(e.FileSize)

	row := fmt.Sprintf("  %s %s  %-28s  %8s  %6s  %s",
		statusDot, roleIcon, name, size, dur, ts)

	if selected {
		return lipgloss.NewStyle().
			Background(ColorPanel).
			Foreground(ColorText).
			Bold(true).
			Render(row)
	}
	return lipgloss.NewStyle().Foreground(ColorSubtext).Render(row)
}

func (m HistoryModel) renderDetail(e audit.LogEntry) string {
	var s strings.Builder

	labelStyle := lipgloss.NewStyle().Foreground(ColorSubtext).Width(12).Align(lipgloss.Right).PaddingRight(2)
	valueStyle := lipgloss.NewStyle().Foreground(ColorText).Bold(true)
	hashStyle := lipgloss.NewStyle().Foreground(ColorAccent).Faint(true)

	s.WriteString("  ")
	s.WriteString(lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("Details"))
	s.WriteString("\n")

	// Row 1: File + Code
	s.WriteString("  ")
	s.WriteString(labelStyle.Render("File"))
	s.WriteString(valueStyle.Render(e.FileName))
	s.WriteString("\n")

	s.WriteString("  ")
	s.WriteString(labelStyle.Render("Code"))
	s.WriteString(lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render(e.Code))
	s.WriteString("\n")

	// Row 2: Date + Duration + Size
	s.WriteString("  ")
	s.WriteString(labelStyle.Render("Date"))
	s.WriteString(valueStyle.Render(e.Timestamp.Format("2006-01-02 15:04:05")))
	s.WriteString("    ")
	s.WriteString(lipgloss.NewStyle().Foreground(ColorSubtext).Render("Duration "))
	s.WriteString(valueStyle.Render(fmt.Sprintf("%.2fs", e.Duration)))
	s.WriteString("    ")
	s.WriteString(lipgloss.NewStyle().Foreground(ColorSubtext).Render("Size "))
	s.WriteString(valueStyle.Render(FormatBytes(e.FileSize)))
	s.WriteString("\n")

	// Row 3: SHA-256
	s.WriteString("  ")
	s.WriteString(labelStyle.Render("SHA-256"))
	if e.FileHash != "" {
		s.WriteString(hashStyle.Render(e.FileHash))
	} else {
		s.WriteString(hashStyle.Render("n/a"))
	}
	s.WriteString("\n")

	// Error (if any)
	if e.Error != "" {
		s.WriteString("  ")
		s.WriteString(labelStyle.Render("Error"))
		s.WriteString(lipgloss.NewStyle().Foreground(ColorError).Render(e.Error))
		s.WriteString("\n")
	}

	return s.String()
}

func (m HistoryModel) formatRelativeTime(t time.Time) string {
	d := time.Since(t)

	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", hours)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%dd ago", days)
	case d < 30*24*time.Hour:
		weeks := int(d.Hours() / 24 / 7)
		if weeks == 1 {
			return "1w ago"
		}
		return fmt.Sprintf("%dw ago", weeks)
	default:
		return t.Format("Jan 2")
	}
}

// ── Filtering & Sorting ──

func (m *HistoryModel) applyFilters() {
	// Filter by role
	var roleFiltered []audit.LogEntry
	for _, e := range m.entries {
		switch m.roleFilter {
		case 1:
			if e.Role != "sender" {
				continue
			}
		case 2:
			if e.Role != "receiver" {
				continue
			}
		}
		roleFiltered = append(roleFiltered, e)
	}

	// Filter by search query
	if m.searchQuery != "" {
		q := strings.ToLower(m.searchQuery)
		var searched []audit.LogEntry
		for _, e := range roleFiltered {
			if strings.Contains(strings.ToLower(e.FileName), q) ||
				strings.Contains(strings.ToLower(e.Code), q) ||
				strings.Contains(strings.ToLower(e.ID), q) ||
				strings.Contains(strings.ToLower(e.Role), q) ||
				strings.Contains(strings.ToLower(e.Status), q) {
				searched = append(searched, e)
			}
		}
		roleFiltered = searched
	}

	// Sort
	switch m.sortField {
	case 0: // Date
		if m.sortAsc {
			sortEntries(roleFiltered, func(a, b audit.LogEntry) bool { return a.Timestamp.Before(b.Timestamp) })
		} else {
			sortEntries(roleFiltered, func(a, b audit.LogEntry) bool { return a.Timestamp.After(b.Timestamp) })
		}
	case 1: // Size
		if m.sortAsc {
			sortEntries(roleFiltered, func(a, b audit.LogEntry) bool { return a.FileSize < b.FileSize })
		} else {
			sortEntries(roleFiltered, func(a, b audit.LogEntry) bool { return a.FileSize > b.FileSize })
		}
	case 2: // Duration
		if m.sortAsc {
			sortEntries(roleFiltered, func(a, b audit.LogEntry) bool { return a.Duration < b.Duration })
		} else {
			sortEntries(roleFiltered, func(a, b audit.LogEntry) bool { return a.Duration > b.Duration })
		}
	}

	m.filtered = roleFiltered
}

func sortEntries(entries []audit.LogEntry, less func(a, b audit.LogEntry) bool) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && less(entries[j], entries[j-1]); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

// RunHistoryViewer starts the interactive history TUI.
func RunHistoryViewer() error {
	entries, err := audit.LoadHistory()
	if err != nil {
		return err
	}

	model := NewHistoryModel(entries)
	p := tea.NewProgram(model, tea.WithAltScreen())

	_, err = p.Run()
	return err
}
