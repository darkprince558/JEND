package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

var logPathOverride string

// SetLogPathOverride sets a custom path for the log file (for testing)
func SetLogPathOverride(path string) {
	logPathOverride = path
}

// GetLogPath returns the path to the history log file
func GetLogPath() (string, error) {
	if logPathOverride != "" {
		return logPathOverride, nil
	}
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
	// Marshaling to JSON
	// data, err := json.Marshal(entry) // Removed, marshaling happens in else block
	// if err != nil {
	// 	return err
	// }

	// Prune if necessary (Keep last 1000)
	// We do this by loading. It's not the most efficient for massive logs but fine for 1000 limit.
	// Prune if necessary (Keep last 1000)
	// We do this by loading. It's not the most efficient for massive logs but fine for 1000 limit.
	entries, err := LoadHistory()

	// Determine if we need to prune/rewrite
	// We strictly limit file to 1000.
	// If existing >= 1000, we must rewrite.
	// If existing < 1000, we can just append (optimization).

	if err == nil && len(entries) >= 1000 {
		// Sort by timestamp is done in LoadHistory (Newest First)
		// We insert current entry at top (assuming it is newest)
		// Actually best to append and re-sort or just prepend since it's likely newest.

		all := append([]LogEntry{entry}, entries...)
		// Resort to be safe if timestamps are messy, but usually unnecessary
		sort.Slice(all, func(i, j int) bool {
			return all[i].Timestamp.After(all[j].Timestamp)
		})

		// Keep top 1000
		keep := all[:1000]

		if err := RewriteHistory(keep); err != nil {
			// Fallback? If rewrite fails, we might lose data or just fail.
			return err
		}
	} else {
		// Just append
		// Open file in append mode
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()

		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}

		if _, err := f.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// RewriteHistory overwrites the log file with the provided entries
func RewriteHistory(entries []LogEntry) error {
	path, err := GetLogPath()
	if err != nil {
		return err
	}

	// Create/Truncate
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Entries are passed in Newest First (from LoadHistory),
	// but typically we append logs Oldest First so that "tail" works naturally?
	// Actually JSONL order doesn't strictly matter if we always load & sort.
	// But appending usually implies chronological order.
	// LoadHistory sorts Newest First. So we should reverse them if we want to restore file order.

	for i := len(entries) - 1; i >= 0; i-- {
		data, err := json.Marshal(entries[i])
		if err != nil {
			continue
		}
		f.Write(append(data, '\n'))
	}
	return nil
}

// ClearHistory deletes the history log file
func ClearHistory() error {
	path, err := GetLogPath()
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// GetEntry finds a specific log entry by ID (prefix match supported)
func GetEntry(id string) (LogEntry, error) {
	entries, err := LoadHistory()
	if err != nil {
		return LogEntry{}, err
	}

	for _, e := range entries {
		if strings.HasPrefix(e.ID, id) {
			return e, nil
		}
	}
	return LogEntry{}, fmt.Errorf("entry not found")
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

func ShowDetail(id string) {
	entry, err := GetEntry(id)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("")
	fmt.Println(headerStyle.Render("TRANSFER DETAILS"))
	fmt.Println("")

	printKV := func(k, v string) {
		fmt.Printf("%s %s\n", lipgloss.NewStyle().Bold(true).Width(15).Foreground(lipgloss.Color("240")).Render(k+":"), v)
	}

	printKV("ID", entry.ID)
	printKV("Date", entry.Timestamp.Format(time.RFC822))
	printKV("Role", strings.ToUpper(entry.Role))
	printKV("Status", entry.Status)
	printKV("File", entry.FileName)
	printKV("Size", formatBytes(entry.FileSize))
	printKV("Code", entry.Code)
	printKV("Duration", fmt.Sprintf("%.2fs", entry.Duration))
	fmt.Println("")

	fmt.Println(lipgloss.NewStyle().Bold(true).Render("Integrity Proof:"))
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF")).Render(entry.FileHash))
	fmt.Println("")

	if entry.Error != "" {
		fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF0000")).Render("Error Log:"))
		fmt.Println(entry.Error)
		fmt.Println("")
	}
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
