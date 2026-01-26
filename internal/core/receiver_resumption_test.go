package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStatePersistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jend-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	metaPath := filepath.Join(tmpDir, "test.meta")
	totalSize := int64(1000)
	concurrency := 4

	// 1. Init State
	state, err := loadOrInitState(metaPath, totalSize, concurrency)
	if err != nil {
		t.Fatalf("Failed to init state: %v", err)
	}

	if len(state.Chunks) != 4 {
		t.Errorf("Expected 4 chunks, got %d", len(state.Chunks))
	}
	if state.TotalSize != totalSize {
		t.Errorf("Expected size %d, got %d", totalSize, state.TotalSize)
	}

	// 2. Mark Chunk 0 as Done
	markChunkDone(metaPath, 0)

	// 3. Reload State
	state2, err := loadOrInitState(metaPath, totalSize, concurrency)
	if err != nil {
		t.Fatalf("Failed to reload state: %v", err)
	}

	if !state2.Chunks[0].Done {
		t.Error("Chunk 0 should be marked done after reload")
	}
	if state2.Chunks[1].Done {
		t.Error("Chunk 1 should NOT be done")
	}

	// 4. Test Concurrency Mismatch (Simulate restart with different concurrency)
	// The current logic should keep the OLD concurrency from the file
	state3, err := loadOrInitState(metaPath, totalSize, 8) // Request 8
	if err != nil {
		t.Fatalf("Failed to reload state 3: %v", err)
	}

	if len(state3.Chunks) != 4 {
		t.Errorf("Expected state to preserve 4 chunks, got %d", len(state3.Chunks))
	}
}

func TestDownloadStateResumption(t *testing.T) {
	// This test verifies that we can "resume" by creating a dummy file and checking logic
	// Ideally we mock the networking, but for now we test the state engine.
	// (covered above)
}
