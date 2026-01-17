package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	petname "github.com/dustinkirkland/golang-petname"
	"github.com/gofrs/flock"
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

// getLockPath returns the path to the lock file
func getLockPath() (string, error) {
	logPath, err := GetLogPath()
	if err != nil {
		return "", err
	}
	return logPath + ".lock", nil
}

// withLock executes the given function with an exclusive file lock
func withLock(action func() error) error {
	lockPath, err := getLockPath()
	if err != nil {
		return err
	}

	fileLock := flock.New(lockPath)

	// Try to lock with a timeout to avoid indefinite hanging
	// 5 seconds should be plenty for even a slow rewrite
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	locked, err := fileLock.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("timed out waiting for history lock")
	}
	defer fileLock.Unlock()

	return action()
}

// withReadLock executes the given function with a shared read lock
func withReadLock(action func() error) error {
	lockPath, err := getLockPath()
	if err != nil {
		return err
	}

	fileLock := flock.New(lockPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	locked, err := fileLock.TryRLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to acquire read lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("timed out waiting for history read lock")
	}
	defer fileLock.Unlock()

	return action()
}

// WriteEntry appends a log entry to the history file
func WriteEntry(entry LogEntry) error {
	return withLock(func() error {
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

		// Prune if necessary (Keep last 1000)
		entries, err := loadHistoryInternal(path)

		// If log is large, prune
		if err == nil && len(entries) >= 1000 {
			all := append([]LogEntry{entry}, entries...)
			// Re-sort
			sort.Slice(all, func(i, j int) bool {
				return all[i].Timestamp.After(all[j].Timestamp)
			})

			// Keep top 1000
			keep := all[:1000]
			return rewriteHistoryInternal(path, keep)
		}

		// Otherwise, just append
		return appendEntryInternal(path, entry)
	})
}

// RewriteHistory overwrites the log file with the provided entries
func RewriteHistory(entries []LogEntry) error {
	return withLock(func() error {
		path, err := GetLogPath()
		if err != nil {
			return err
		}
		return rewriteHistoryInternal(path, entries)
	})
}

// ClearHistory deletes the history log file
func ClearHistory() error {
	return withLock(func() error {
		path, err := GetLogPath()
		if err != nil {
			return err
		}
		return os.Remove(path)
	})
}

// GetEntry finds a specific log entry by ID (prefix match supported)
func GetEntry(id string) (LogEntry, error) {
	var found LogEntry
	err := withReadLock(func() error {
		path, err := GetLogPath()
		if err != nil {
			return err
		}
		entries, err := loadHistoryInternal(path)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if strings.HasPrefix(e.ID, id) {
				found = e
				return nil
			}
		}
		return fmt.Errorf("entry not found")
	})
	return found, err
}

// LoadHistory reads all log entries from the history file
func LoadHistory() ([]LogEntry, error) {
	var entries []LogEntry
	err := withReadLock(func() error {
		path, err := GetLogPath()
		if err != nil {
			return err
		}

		var loadErr error
		entries, loadErr = loadHistoryInternal(path)
		return loadErr
	})
	return entries, err
}

// Internal helpers (NO LOCKING)

func loadHistoryInternal(path string) ([]LogEntry, error) {
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

func rewriteHistoryInternal(path string, entries []LogEntry) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Reverse to write oldest first (if desired for append log style)
	// But JSONL doesn't strictly require order.
	for i := len(entries) - 1; i >= 0; i-- {
		data, err := json.Marshal(entries[i])
		if err != nil {
			continue
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func appendEntryInternal(path string, entry LogEntry) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	_, err = f.Write(append(data, '\n'))
	return err
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
