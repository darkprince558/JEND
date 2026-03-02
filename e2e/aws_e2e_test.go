package e2e

import (
"bufio"
"bytes"
"fmt"
"os"
"os/exec"
"path/filepath"
"strings"
"testing"
"time"
)

// Binary path relative to this test file
const binPath = "../../bin/jend"

func TestAWSS3Transfer(t *testing.T) {
	// Build the binary
	cmd := exec.Command("go", "build", "-o", binPath, "../cmd/jend")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build binary: %v\n%s\n", err, out)
	}

	// Setup
	srcFile := "test_data/aws_payload.txt"
	os.MkdirAll("test_data", 0755)
	content := []byte("Hello, JEND World via AWS S3! This is a robust test.")
	if err := os.WriteFile(srcFile, content, 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}
	outDir := "output/aws_s3_test"
	os.RemoveAll(outDir)
	os.MkdirAll(outDir, 0755)

	// Start Sender in S3 mode
	senderCmd := exec.Command(binPath, "send", srcFile, "--headless", "--s3")
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
			fmt.Printf("[Sender] %s\n", line)
			if strings.HasPrefix(line, "Code: ") {
				select {
				case codeCh <- strings.TrimPrefix(line, "Code: "):
				default:
				}
			}
		}
	}()

	var code string
	select {
	case c := <-codeCh:
		code = c
	case <-time.After(15 * time.Second):
		t.Fatal("Timeout waiting for code generation")
	}

	// Give S3 a moment to register fully
	time.Sleep(3 * time.Second)

	// Start Receiver
	receiverCmd := exec.Command(binPath, "receive", code, "--dir", outDir, "--headless")
	receiverCmd.Stdout = os.Stdout
	receiverCmd.Stderr = os.Stderr
	if err := receiverCmd.Start(); err != nil {
		t.Fatalf("Failed to start receiver: %v", err)
	}

	if err := receiverCmd.Wait(); err != nil {
		t.Fatalf("Receiver failed: %v", err)
	}

	// Wait up to 10 seconds for sender to gracefully exit
    senderWaitCh := make(chan error)
    go func() {
        senderWaitCh <- senderCmd.Wait()
    }()

    select {
    case err := <-senderWaitCh:
        if err != nil {
            t.Logf("Sender exited with error: %v (Usually due to context cancellation - OK)", err)
        } else {
            t.Log("Sender gracefully exited!")
        }
    case <-time.After(20 * time.Second):
        t.Fatal("FAILED: Sender did not exit gracefully after successful transfer via S3")
    }

	// Verify Content
	destFile := filepath.Join(outDir, "aws_payload.txt")
	got, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("Failed to read received file: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("Content mismatch.\nExpected: %s\nGot: %s", content, got)
	}
}
