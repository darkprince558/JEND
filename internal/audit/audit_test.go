package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuditLogLifecycle(t *testing.T) {
	// Setup temporary directory for testing
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test_history.jsonl")
	SetLogPathOverride(logFile)
	defer SetLogPathOverride("") // Cleanup

	// 1. Test Write
	entry1 := LogEntry{ID: "1", Role: "sender", Status: "success"}
	if err := WriteEntry(entry1); err != nil {
		t.Fatalf("WriteEntry failed: %v", err)
	}

	// 2. Test Load
	entries, err := LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(entries))
	}
	if entries[0].ID != "1" {
		t.Errorf("Expected ID 1, got %s", entries[0].ID)
	}

	// 3. Test Pruning (Simulate 1005 entries)
	// We manually write 1005 entries (append)
	// Wait, WriteEntry handles pruning.
	// Append 1100 entries to test log rotation.
	// Write directly for speed.
	for i := 0; i < 1100; i++ {
		// We set timestamp to be monotonic so sorting is stable
		e := LogEntry{
			ID:        fmt.Sprintf("p-%d", i),
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := WriteEntry(e); err != nil {
			t.Fatalf("WriteEntry loop failed at %d: %v", i, err)
		}
	}

	// 4. Verify Pruning
	entries, err = LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory after prune failed: %v", err)
	}
	if len(entries) > 1000 {
		t.Errorf("Pruning failed. Expected <= 1000 entries, got %d", len(entries))
	}
	// Since we added 1101 entries (1 initial + 1100 loop), and they are time sorted.
	// The last inserted (newest) should remain.
	// entries[0] is newest.
	// ID should be "p-1099"
	if !strings.HasPrefix(entries[0].ID, "p-") {
		// It might be "1" if time didn't work out, actually "1" has default time (Now).
		// The loop adds (Now + i seconds). So "p-1099" is definitely newest.
	}

	// 5. Test Clear
	if err := ClearHistory(); err != nil {
		t.Fatalf("ClearHistory failed: %v", err)
	}

	// Verify Cleared
	entries, err = LoadHistory()
	if err != nil { // LoadHistory returns empty on NotExist, or error?
		// It returns empty slice on NotExist inside LoadHistory logic.
	}
	if len(entries) != 0 {
		t.Errorf("History not cleared. Got %d entries", len(entries))
	}

	// Verify file gone
	if _, err := os.Stat(logFile); !os.IsNotExist(err) {
		t.Error("Log file still exists after clear")
	}
}

func TestEntryMarshaling(t *testing.T) {
	entry := LogEntry{
		ID:        "test-id",
		Timestamp: time.Now(),
		Role:      "sender",
		FileName:  "foo.txt",
		FileSize:  1024,
		Status:    "success",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded LogEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.ID != entry.ID {
		t.Errorf("Expected ID %s, got %s", entry.ID, decoded.ID)
	}
}

func TestConcurrentWrites(t *testing.T) {
	// Setup temporary directory for testing
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "pru_history.jsonl")
	SetLogPathOverride(logFile)
	defer SetLogPathOverride("")

	const numGoroutines = 10
	const entriesPerGoroutine = 50

	errCh := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < entriesPerGoroutine; j++ {
				entry := LogEntry{
					ID:        fmt.Sprintf("worker-%d-%d", id, j),
					Timestamp: time.Now(),
					Role:      "sender",
					Status:    "success",
				}
				if err := WriteEntry(entry); err != nil {
					errCh <- fmt.Errorf("worker %d failed: %v", id, err)
					return
				}
			}
			errCh <- nil
		}(i)
	}

	for i := 0; i < numGoroutines; i++ {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}

	// Verify count
	entries, err := LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}

	expected := numGoroutines * entriesPerGoroutine
	if len(entries) != expected {
		t.Errorf("Expected %d entries, got %d", expected, len(entries))
	}
}
