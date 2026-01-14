package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkprince558/jend/internal/audit"
)

// Binary path relative to this test file
const binaryPath = "../../bin/jend"

func TestMain(m *testing.M) {
	// 1. Build the binary
	cmd := exec.Command("go", "build", "-o", binaryPath, "../cmd/jend")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build binary: %v\n%s\n", err, out)
		os.Exit(1)
	}

	// 2. Setup Test Environment if needed
	os.MkdirAll("test_data", 0755)

	// 3. Run Tests
	code := m.Run()

	// 4. Cleanup
	os.RemoveAll("test_data")
	os.RemoveAll("output")
	os.Remove(binaryPath)

	os.Exit(code)
}

func TestFileTransfer(t *testing.T) {
	// Setup
	srcFile := "test_data/payload.txt"
	content := []byte("Hello, JEND World! This is a robust test.")
	if err := os.WriteFile(srcFile, content, 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}
	outDir := "output/file_test"
	os.RemoveAll(outDir)

	// Start Sender
	senderCmd := exec.Command(binaryPath, "send", srcFile, "--headless")
	senderOut, err := senderCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to get sender stdout: %v", err)
	}
	if err := senderCmd.Start(); err != nil {
		t.Fatalf("Failed to start sender: %v", err)
	}
	defer func() {
		if senderCmd.Process != nil {
			senderCmd.Process.Kill()
		}
	}()

	// Parse Code
	codeCh := make(chan string)
	go func() {
		scanner := bufio.NewScanner(senderOut)
		for scanner.Scan() {
			line := scanner.Text()
			// Log for debugging
			t.Logf("[Sender] %s", line)
			if strings.HasPrefix(line, "Code: ") {
				codeCh <- strings.TrimPrefix(line, "Code: ")
				return
			}
		}
	}()

	var code string
	select {
	case c := <-codeCh:
		code = c
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for code generation")
	}

	// Start Receiver
	receiverCmd := exec.Command(binaryPath, "receive", code, "--dir", outDir, "--headless")
	if out, err := receiverCmd.CombinedOutput(); err != nil {
		t.Fatalf("Receiver failed: %v\nOutput: %s", err, out)
	}

	// Wait for Sender to finish
	if err := senderCmd.Wait(); err != nil {
		t.Fatalf("Sender exited with error: %v", err)
	}

	// Verify Content
	destFile := filepath.Join(outDir, "payload.txt")
	got, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("Failed to read received file: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("Content mismatch.\nExpected: %s\nGot: %s", content, got)
	}
}

func TestAuditLog(t *testing.T) {
	// This test depends on the side effect of TestFileTransfer or runs its own small transfer
	// Run a small transfer
	// ... (Use a helper function in real robust code)
	srcFile := "test_data/audit_payload.txt"
	os.WriteFile(srcFile, []byte("Audit"), 0644)
	outDir := "output/audit_test"

	// Sender
	senderCmd := exec.Command(binaryPath, "send", srcFile, "--headless")
	senderOut, _ := senderCmd.StdoutPipe()
	senderCmd.Start()
	defer senderCmd.Process.Kill()

	scanner := bufio.NewScanner(senderOut)
	var code string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Code: ") {
			code = strings.TrimPrefix(line, "Code: ")
			break
		}
	}

	if code == "" {
		t.Fatal("Failed to get code")
	}

	// Receiver
	exec.Command(binaryPath, "receive", code, "--dir", outDir, "--headless").Run()
	senderCmd.Wait()

	// Verify Log
	historyPath, _ := audit.GetLogPath()
	data, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("Failed to read log file at %s: %v", historyPath, err)
	}

	// Look for our specific file in the logs (JSONL parsing)
	found := false
	lines := bytes.Split(data, []byte("\n"))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var entry audit.LogEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.FileName == "audit_payload.txt" && entry.Status == "success" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Audit log entry for 'audit_payload.txt' not found or not successful")
	}
}
