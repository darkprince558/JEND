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
	senderCmd.Stderr = os.Stderr
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
			fmt.Printf("[Sender] %s\n", line)
			if strings.HasPrefix(line, "Code: ") {
				select {
				case codeCh <- strings.TrimPrefix(line, "Code: "):
				default:
				}
				// Don't return, keep logging
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
	receiverCmd.Stdout = os.Stdout
	receiverCmd.Stderr = os.Stderr
	if err := receiverCmd.Start(); err != nil {
		t.Fatalf("Failed to start receiver: %v", err)
	}

	if err := receiverCmd.Wait(); err != nil {
		t.Fatalf("Receiver failed: %v", err)
	}

	// Wait for Sender to finish
	// Sender loops indefinitely now (Resume Support), so we must kill it.
	if err := senderCmd.Process.Signal(os.Interrupt); err != nil {
		senderCmd.Process.Kill()
	}
	// Wait for it to exit after signal
	senderCmd.Wait()

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

// TestLargeFileTransfer verifies parallel streaming (triggers > 100MB logic)
func TestLargeFileTransfer(t *testing.T) {
	// Create large 150MB file
	largeFileName := "large_test.bin"
	f, err := os.Create(largeFileName)
	if err != nil {
		t.Fatal(err)
	}
	// Write dummy data (fast)
	// 150MB = 150 * 1024 * 1024
	size := int64(150 * 1024 * 1024)
	if err := f.Truncate(size); err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(largeFileName)
	defer os.RemoveAll("received_large")

	// Same setup as TestFileTransfer
	// ... reusing sender/receiver logic but with large file
	// Since the code is largely copy-paste, I'll invoke helper if possible
	// or just run the sender/receiver commands.

	// Start Sender
	senderCmd := exec.Command(binaryPath, "send", largeFileName, "--headless", "--no-history", "--no-clipboard")
	// Pipes...
	senderReader, err := senderCmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	senderCmd.Stderr = os.Stderr // debug

	if err := senderCmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if senderCmd.Process != nil {
			senderCmd.Process.Kill()
		}
	}()

	// Scan for Code
	var code string
	scanner := bufio.NewScanner(senderReader)
	for scanner.Scan() {
		line := scanner.Text()
		t.Logf("[Sender] %s", line)
		if strings.Contains(line, "Code:") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				code = strings.TrimSpace(parts[1])
				break
			}
		}
	}
	if code == "" {
		t.Fatal("Failed to get code from sender")
	}

	// Start Receiver
	time.Sleep(2 * time.Second) // Let sender init
	recvCmd := exec.Command(binaryPath, "receive", code, "--headless", "--no-history", "--no-clipboard", "--dir", "received_large")

	// Pipe output to test stdout for live debugging
	recvCmd.Stdout = os.Stdout
	recvCmd.Stderr = os.Stderr

	if err := recvCmd.Run(); err != nil {
		t.Fatalf("Receiver failed: %v", err)
	}

	// Verify File
	info, err := os.Stat(filepath.Join("received_large", largeFileName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != size {
		t.Fatalf("Size mismatch. Want %d, Got %d", size, info.Size())
	}

	// Kill Sender (it loops)
	if senderCmd.Process != nil {
		senderCmd.Process.Signal(os.Interrupt)
		senderCmd.Wait()
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

	// Kill Sender (it loops)
	if senderCmd.Process != nil {
		senderCmd.Process.Signal(os.Interrupt)
		senderCmd.Wait()
	}

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
	// Setup Large File (50MB to ensure we can interrupt)
	srcFile := "test_data/large_payload.bin"
	size := 50 * 1024 * 1024 // 50MB
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
	receiverCmd1.Stdout = os.Stdout
	receiverCmd1.Stderr = os.Stderr
	if err := receiverCmd1.Start(); err != nil {
		t.Fatalf("Receiver 1 failed to start: %v", err)
	}

	// Let transfer start - Wait for partial file to ensure we are in data phase
	partialPath := filepath.Join(outDir, "large_payload.bin.partial")
	deadline := time.Now().Add(15 * time.Second)
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

	// Create a smaller file (10MB is enough with delay)
	f, _ := os.Create(srcFile)
	f.Seek(10*1024*1024, 0) // 10MB
	f.Write([]byte{0})
	f.Close()

	// Build Binary (to ensure we test updated main code)
	// We assume go run uses updated code.
	binaryPath := filepath.Join(tmpDir, "jend_test_btn")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "../cmd/jend")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build binary: %v", err)
	}

	// Start Sender with Delay Env Var
	senderCmd := exec.Command(binaryPath, "send", srcFile, "--headless", "--timeout", "30s")
	senderCmd.Env = append(os.Environ(), "JEND_TEST_DELAY=100ms")
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

	// Wait for Handshake to complete (Checksum done)
	// We scan sender output for "Handshake sent"
	readyToCancel := false
	deadline := time.Now().Add(20 * time.Second) // Allow time for hashing 5GB
	for time.Now().Before(deadline) {
		if strings.Contains(senderStdout.String(), "Handshake sent") {
			readyToCancel = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !readyToCancel {
		t.Logf("Sender Output: %s", senderStdout.String())
		t.Fatal("Timeout waiting for sender to finish hashing/handshake")
	}

	// Give it a tiny moment to start data phase
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

func TestBinaryFileTransfer(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "output")
	os.MkdirAll(outDir, 0755)

	// Create a binary file (mixture of ranges)
	srcFile := filepath.Join(tmpDir, "binary.dat")
	size := 10 * 1024 * 1024 // 10MB
	content := make([]byte, size)
	// Fill with random-ish data (predictable for replay)
	for i := range content {
		content[i] = byte((i * 37) % 256)
	}
	if err := os.WriteFile(srcFile, content, 0644); err != nil {
		t.Fatalf("Failed to create binary file: %v", err)
	}

	// Build Binary
	binaryPath := filepath.Join(tmpDir, "jend_test_bin")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "../cmd/jend")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build binary: %v", err)
	}

	// Start Sender
	senderCmd := exec.Command(binaryPath, "send", srcFile, "--headless", "--timeout", "30s")
	var senderStdout bytes.Buffer
	senderCmd.Stdout = &senderStdout

	if err := senderCmd.Start(); err != nil {
		t.Fatalf("Failed to start sender: %v", err)
	}

	// Ensure we kill the sender eventually
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
	if out, err := receiverCmd.CombinedOutput(); err != nil {
		t.Fatalf("Receiver failed: %v\nOutput: %s", err, out)
	}

	// Explicitly kill sender now that receiver is done (since sender loops)
	senderCmd.Process.Signal(os.Interrupt)
	senderCmd.Wait()

	// Verify Content
	destFile := filepath.Join(outDir, "binary.dat")
	got, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("Failed to read received file: %v", err)
	}

	if !bytes.Equal(got, content) {
		t.Fatal("Binary content mismatch! Transfer corrupted.")
	}
}

func TestMP4Transfer(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "output")
	os.MkdirAll(outDir, 0755)

	// Download a real MP4 file
	// Using a reliable sample URL (Big Buck Bunny, ~1-2MB)
	url := "https://test-videos.co.uk/vids/bigbuckbunny/mp4/h264/720/Big_Buck_Bunny_720_10s_1MB.mp4"
	srcFile := filepath.Join(tmpDir, "sample.mp4")

	t.Logf("Downloading sample MP4 from %s...", url)
	// We use http.Get to download
	// Note: We need 'net/http' and 'io'
	// Since I cannot add imports easily without analyzing the file, I will rely on existing imports or add them if missing.
	// But 'net/http' is likely not imported in e2e_test.go. I should check imports first or use curl via exec.
	// Using curl is safer and cleaner here given imports management cost.

	err := exec.Command("curl", "-L", "-o", srcFile, url).Run()
	if err != nil {
		t.Skipf("Failed to download sample MP4 (internet required): %v", err)
	}

	// Read content for verification
	content, err := os.ReadFile(srcFile)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}
	t.Logf("Downloaded %d bytes", len(content))

	// Build Binary
	binaryPath := filepath.Join(tmpDir, "jend_test_mp4")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "../cmd/jend")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build binary: %v", err)
	}

	// Start Sender
	senderCmd := exec.Command(binaryPath, "send", srcFile, "--headless", "--timeout", "60s")
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
	time.Sleep(3 * time.Second) // Give a bit more time for file hashing if needed
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
	if out, err := receiverCmd.CombinedOutput(); err != nil {
		t.Fatalf("Receiver failed: %v\nOutput: %s", err, out)
	}

	// Kill Sender
	senderCmd.Process.Signal(os.Interrupt)
	senderCmd.Wait()

	// Verify Content
	destFile := filepath.Join(outDir, "sample.mp4")
	got, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("Failed to read received file: %v", err)
	}

	if !bytes.Equal(got, content) {
		t.Fatal("MP4 content mismatch! Transfer corrupted.")
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
