package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/charmbracelet/lipgloss"
	petname "github.com/dustinkirkland/golang-petname"
)

// LogEntry represents a single transfer event
type LogEntry struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Role      string    `json:"role"` // "sender" or "receiver"
	FileName  string    `json:"file_name"`
	FileSize  int64     `json:"file_size"`
	FileHash  string    `json:"file_hash"`
	Code      string    `json:"code"`
	Status    string    `json:"status"` // "success" or "failed"
	Error     string    `json:"error,omitempty"`
	Duration  float64   `json:"duration_seconds"`
}

// GetLogPath returns the path to the history log file
func GetLogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".jend")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.jsonl"), nil
}

// WriteEntry appends a log entry to the history file
func WriteEntry(entry LogEntry) error {
	path, err := GetLogPath()
	if err != nil {
		return err
	}

	// Ensure ID is set
	if entry.ID == "" {
		entry.ID = petname.Generate(2, "-") // Simple ID
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	// Marshaling to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	// Open file in append mode
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

// LoadHistory reads all log entries from the history file
func LoadHistory() ([]LogEntry, error) {
	path, err := GetLogPath()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []LogEntry{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []LogEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // Skip malformed lines
		}
		entries = append(entries, entry)
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	return entries, scanner.Err()
}

// --- Display Logic ---

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	rowStyle = lipgloss.NewStyle().
			Padding(0, 1)

	statusSuccessStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("SUCCESS")
	statusFailStr    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("FAILED")
)

func ShowHistory() {
	entries, err := LoadHistory()
	if err != nil {
		fmt.Printf("Error loading history: %v\n", err)
		return
	}

	if len(entries) == 0 {
		fmt.Println("No transfer history found.")
		return
	}

	// Define Columns
	// DATE | ROLE | FILE | SIZE | TIME | STATUS | HASH

	fmt.Println("")
	fmt.Printf("%s %s %s %s %s %s %s\n",
		headerStyle.Width(20).Render("DATE"),
		headerStyle.Width(10).Render("ROLE"),
		headerStyle.Width(25).Render("FILE"),
		headerStyle.Width(10).Render("SIZE"),
		headerStyle.Width(8).Render("TIME"),
		headerStyle.Width(10).Render("STATUS"),
		headerStyle.Width(10).Render("HASH"),
	)
	fmt.Println("")

	for _, e := range entries {
		ts := e.Timestamp.Format("2006-01-02 15:04")
		role := e.Role
		file := e.FileName
		if len(file) > 23 {
			file = file[:20] + "..."
		}
		size := formatBytes(e.FileSize)
		duration := fmt.Sprintf("%.1fs", e.Duration)
		status := statusSuccessStr
		if e.Status != "success" {
			status = statusFailStr
		}
		hash := ""
		if len(e.FileHash) > 8 {
			hash = e.FileHash[:8] + "..."
		}

		// Color coding for role
		roleStr := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(role)
		if role == "sender" {
			roleStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Render("SENDER")
		} else {
			roleStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF")).Render("RECEIVER")
		}

		fmt.Printf("%s %s %s %s %s %s %s\n",
			rowStyle.Width(20).Render(ts),
			rowStyle.Width(10).Render(roleStr),
			rowStyle.Width(25).Render(file),
			rowStyle.Width(10).Render(size),
			rowStyle.Width(8).Render(duration),
			rowStyle.Width(10).Render(status),
			rowStyle.Width(10).Render(hash),
		)
	}
	fmt.Println("")
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
