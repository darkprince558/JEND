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
	// Sender should exit after transfer (or timeout if we changed logic to loop forever? No, it returns on done)
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

// TestResumeSupport verifies that the sender stays alive for multiple connections
// and allows a receiver to resume.
func TestResumeSupport(t *testing.T) {
	// Setup Large File (1MB is enough to chunk)
	srcFile := "test_data/large_payload.bin"
	size := 1024 * 1024 // 1MB
	content := make([]byte, size)
	for i := range content {
		content[i] = byte(i % 255)
	}
	if err := os.WriteFile(srcFile, content, 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}
	outDir := "output/resume_test"
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
	var code string
	scanner := bufio.NewScanner(senderOut)
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
	t.Logf("Got Code: %s", code)

	// Step 1: Start Receiver, let it run briefly then kill it to simulate failure
	// We can't easily control exactly how many bytes...
	// But we can start it asynchronously and kill it after 100ms.
	receiverCmd1 := exec.Command(binaryPath, "receive", code, "--dir", outDir, "--headless")
	if err := receiverCmd1.Start(); err != nil {
		t.Fatalf("Receiver 1 failed to start: %v", err)
	}

	// Let transfer start - Wait for partial file to ensure we are in data phase
	partialPath := filepath.Join(outDir, "large_payload.bin.partial")
	deadline := time.Now().Add(5 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		if _, err := os.Stat(partialPath); err == nil {
			found = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !found {
		t.Log("Partial file not created in time, checking if transfer completed already?")
		// Might have completed? Check final file
		if _, err := os.Stat(filepath.Join(outDir, "large_payload.bin")); err == nil {
			t.Fatal("Transfer completed too fast! Increase file size or kill sooner.")
		}
		t.Fatal("Partial file not created in time")
	}
	time.Sleep(200 * time.Millisecond) // Allow some data to be written
	receiverCmd1.Process.Kill()
	t.Log("Killed Receiver 1 (Simulation)")

	// Verify partial file details
	info, err := os.Stat(partialPath)
	if err != nil {
		t.Fatalf("Partial file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Log("Warning: Partial file size is 0, maybe too fast or too slow? resuming from 0 is also valid test.")
	} else {
		t.Logf("Partial file size: %d", info.Size())
	}

	// Step 2: Start new Receiver (Resume)
	t.Log("Starting Receiver 2 (Resume)...")
	receiverCmd2 := exec.Command(binaryPath, "receive", code, "--dir", outDir, "--headless")
	out, err := receiverCmd2.CombinedOutput()
	if err != nil {
		t.Fatalf("Receiver 2 failed: %v\nOutput: %s", err, out)
	}

	// Verify Final Content
	destFile := filepath.Join(outDir, "large_payload.bin")
	got, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("Failed to read received file: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("Content mismatch after resume.\nSize Expected: %d\nSize Got: %d", len(content), len(got))
	}
}

func TestSenderCancellation(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "cancel_test.bin")
	outDir := filepath.Join(tmpDir, "output")
	os.MkdirAll(outDir, 0755)

	// Create a large file (so we have time to cancel)
	f, _ := os.Create(srcFile)
	f.Seek(100*1024*1024, 0) // 100MB
	f.Write([]byte{0})
	f.Close()

	// Build Binary (to ensure we test updated main code)
	// We assume go run uses updated code.
	binaryPath := filepath.Join(tmpDir, "jend_test_btn")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "../cmd/jend")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build binary: %v", err)
	}

	// Start Sender
	senderCmd := exec.Command(binaryPath, "send", srcFile, "--headless", "--timeout", "10s")
	var senderStdout bytes.Buffer
	senderCmd.Stdout = &senderStdout

	if err := senderCmd.Start(); err != nil {
		t.Fatalf("Failed to start sender: %v", err)
	}
	defer func() {
		if senderCmd.Process != nil {
			senderCmd.Process.Kill()
		}
	}()

	// Wait for code
	time.Sleep(2 * time.Second)
	output := senderStdout.String()
	marker := "Code: "
	idx := strings.Index(output, marker)
	if idx == -1 {
		t.Fatalf("Sender didn't print code. Output: %s", output)
	}
	code := strings.TrimSpace(output[idx+len(marker):])
	code = strings.Fields(code)[0]
	t.Logf("Got Code: %s", code)

	// Start Receiver
	receiverCmd := exec.Command(binaryPath, "receive", code, "--dir", outDir, "--headless")
	var receiverStdout bytes.Buffer
	receiverCmd.Stdout = &receiverStdout

	if err := receiverCmd.Start(); err != nil {
		t.Fatalf("Failed to start receiver: %v", err)
	}
	defer func() {
		if receiverCmd.Process != nil {
			receiverCmd.Process.Kill()
		}
	}()

	// Wait for transfer to start
	time.Sleep(500 * time.Millisecond)

	// Interrupt Sender
	t.Log("Interrupting Sender...")
	if err := senderCmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("Failed to signal sender: %v", err)
	}

	// Wait for Receiver to finish
	// It should exit. We don't check exit code strictly, but we check stdout.
	receiverCmd.Wait()

	// Check Output for cancellation message
	recvOutput := receiverStdout.String()
	t.Logf("Receiver Output: %s", recvOutput)

	if !strings.Contains(recvOutput, "transfer cancelled by sender") {
		t.Error("Receiver did not report cancellation! Expected 'transfer cancelled by sender'")
	}
}

func TestTextTransfer(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "output")
	os.MkdirAll(outDir, 0755)

	textContent := "Hello World from JEND Text Mode!"

	// Build Binary
	binaryPath := filepath.Join(tmpDir, "jend_test_text")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "../cmd/jend")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build binary: %v", err)
	}

	// Start Sender with --text
	senderCmd := exec.Command(binaryPath, "send", "--text", textContent, "--headless", "--timeout", "10s")
	var senderStdout bytes.Buffer
	senderCmd.Stdout = &senderStdout

	if err := senderCmd.Start(); err != nil {
		t.Fatalf("Failed to start sender: %v", err)
	}
	defer func() {
		if senderCmd.Process != nil {
			senderCmd.Process.Kill()
		}
	}()

	// Wait for code
	time.Sleep(2 * time.Second)
	output := senderStdout.String()
	marker := "Code: "
	idx := strings.Index(output, marker)
	if idx == -1 {
		t.Fatalf("Sender didn't print code. Output: %s", output)
	}
	code := strings.TrimSpace(output[idx+len(marker):])
	code = strings.Fields(code)[0]
	t.Logf("Got Code: %s", code)

	// Start Receiver
	receiverCmd := exec.Command(binaryPath, "receive", code, "--dir", outDir, "--headless")
	var receiverStdout bytes.Buffer
	receiverCmd.Stdout = &receiverStdout

	if err := receiverCmd.Start(); err != nil {
		t.Fatalf("Failed to start receiver: %v", err)
	}
	defer func() {
		if receiverCmd.Process != nil {
			receiverCmd.Process.Kill()
		}
	}()

	// Wait for completion
	done := make(chan error, 1)
	go func() {
		done <- receiverCmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			// Print partial output on error
			t.Logf("Receiver Output: %s", receiverStdout.String())
			t.Fatalf("Receiver failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Logf("Receiver Output: %s", receiverStdout.String())
		t.Fatal("Receiver timed out")
	}

	// Check Output
	recvOutput := receiverStdout.String()
	t.Logf("Receiver Output: %s", recvOutput)
	if !strings.Contains(recvOutput, "Received Text:") {
		t.Error("Receiver output missing 'Received Text:' header")
	}
	if !strings.Contains(recvOutput, textContent) {
		t.Errorf("Receiver output missing content: %s", textContent)
	}

	// Ensure NO file was created
	files, _ := os.ReadDir(outDir)
	if len(files) > 0 {
		t.Errorf("Receiver created files in text mode! Found: %v", files)
	}
}

func TestNoClipboard(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "output")
	os.MkdirAll(outDir, 0755)

	textContent := "Sensitive Data - Do Not Copy"

	// Build Binary
	binaryPath := filepath.Join(tmpDir, "jend_test_noclip")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "../cmd/jend")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build binary: %v", err)
	}

	// Start Sender
	senderCmd := exec.Command(binaryPath, "send", "--text", textContent, "--headless", "--timeout", "10s")
	var senderStdout bytes.Buffer
	senderCmd.Stdout = &senderStdout

	if err := senderCmd.Start(); err != nil {
		t.Fatalf("Failed to start sender: %v", err)
	}
	defer func() {
		if senderCmd.Process != nil {
			senderCmd.Process.Kill()
		}
	}()

	// Wait for code
	time.Sleep(2 * time.Second)
	output := senderStdout.String()
	marker := "Code: "
	idx := strings.Index(output, marker)
	if idx == -1 {
		t.Fatalf("Sender didn't print code. Output: %s", output)
	}
	code := strings.TrimSpace(output[idx+len(marker):])
	code = strings.Fields(code)[0]
	t.Logf("Got Code: %s", code)

	// Start Receiver WITH --no-clipboard
	receiverCmd := exec.Command(binaryPath, "receive", code, "--dir", outDir, "--headless", "--no-clipboard")
	var receiverStdout bytes.Buffer
	receiverCmd.Stdout = &receiverStdout

	if err := receiverCmd.Start(); err != nil {
		t.Fatalf("Failed to start receiver: %v", err)
	}

	// Wait for completion
	done := make(chan error, 1)
	go func() {
		done <- receiverCmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Receiver failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Receiver timed out")
	}

	// Check Output
	recvOutput := receiverStdout.String()
	t.Logf("Receiver Output: %s", recvOutput)

	if !strings.Contains(recvOutput, "Received Text:") {
		t.Error("Receiver output missing 'Received Text:' header")
	}
	if !strings.Contains(recvOutput, textContent) {
		t.Errorf("Receiver output missing content: %s", textContent)
	}
	if strings.Contains(recvOutput, "Text copied to clipboard!") {
		t.Error("Receiver copied to clipboard despite --no-clipboard flag!")
	}
	if !strings.Contains(recvOutput, "Clipboard copy skipped (--no-clipboard)") {
		t.Error("Receiver output missing skip message")
	}
}

func TestNoHistory(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "output")
	os.MkdirAll(outDir, 0755)

	textContent := "Ghost Transfer"

	// Build Binary
	binaryPath := filepath.Join(tmpDir, "jend_test_nohist")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "../cmd/jend")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build binary: %v", err)
	}

	// 1. Clear History explicitly to ensure clean slate
	// Note: This clears the ACTUAL history of the user running the test if we don't mock the audit file location.
	// However, `internal/audit` writes to `~/.jend/audit.json` or similar.
	// For testing, we should probably redirect the audit file or just check the count before/after if we can't redirect.
	// Ideally `audit` package should allow overriding the path.
	// HACK: We will check the output of `jend history` command.

	// Get initial history count
	histCmd := exec.Command(binaryPath, "history")
	var histOut bytes.Buffer
	histCmd.Stdout = &histOut
	if err := histCmd.Run(); err != nil {
		t.Fatalf("Failed to run history: %v", err)
	}
	initialLines := strings.Count(histOut.String(), "\n")

	// 2. Perform Transfer with --no-history
	// Start Sender
	senderCmd := exec.Command(binaryPath, "send", "--text", textContent, "--headless", "--timeout", "10s", "--no-history")
	var senderStdout bytes.Buffer
	senderCmd.Stdout = &senderStdout

	if err := senderCmd.Start(); err != nil {
		t.Fatalf("Failed to start sender: %v", err)
	}
	defer func() {
		if senderCmd.Process != nil {
			senderCmd.Process.Kill()
		}
	}()

	time.Sleep(2 * time.Second)
	output := senderStdout.String()
	marker := "Code: "
	idx := strings.Index(output, marker)
	if idx == -1 {
		t.Fatalf("Sender didn't print code. Output: %s", output)
	}
	code := strings.TrimSpace(output[idx+len(marker):])
	code = strings.Fields(code)[0]
	t.Logf("Got Code: %s", code)

	// Start Receiver WITH --no-history
	receiverCmd := exec.Command(binaryPath, "receive", code, "--dir", outDir, "--headless", "--no-history")
	if err := receiverCmd.Start(); err != nil {
		t.Fatalf("Failed to start receiver: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- receiverCmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Receiver failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Receiver timed out")
	}

	// 3. Check History again
	histCmd2 := exec.Command(binaryPath, "history")
	var histOut2 bytes.Buffer
	histCmd2.Stdout = &histOut2
	if err := histCmd2.Run(); err != nil {
		t.Fatalf("Failed to run history again: %v", err)
	}
	finalLines := strings.Count(histOut2.String(), "\n")

	if finalLines != initialLines {
		t.Errorf("History changed! Initial lines: %d, Final lines: %d. Diff: \n%s", initialLines, finalLines, histOut2.String())
	}
}
