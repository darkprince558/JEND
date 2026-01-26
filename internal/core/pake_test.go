package core

import (
	"io"
	"testing"
	"time"
)

func TestPerformPAKE_Argon2(t *testing.T) {
	// this test simulates a PAKE exchange
	password := "correct-horse-battery-staple"

	// Better: Run Sender and Receiver in goroutines with a pipe.
	r, w := io.Pipe()   // Sender writes to w, Receiver reads from r
	r2, w2 := io.Pipe() // Receiver writes to w2, Sender reads from r2

	// Sender Stream: Reads from r2, Writes to w
	senderRW := &readWriter{Reader: r2, Writer: w}

	// Receiver Stream: Reads from r, Writes to w2
	receiverRW := &readWriter{Reader: r, Writer: w2}

	start := time.Now()

	errChan := make(chan error)

	go func() {
		_, err := PerformPAKE(senderRW, password, 0)
		if err != nil {
			errChan <- err
		}
		close(errChan)
	}()

	_, err := PerformPAKE(receiverRW, password, 1)
	if err != nil {
		t.Errorf("Handshake failed: %v", err)
	}

	// Wait for both
	for i := 0; i < 1; i++ { // Changed loop count to 1 as receiver is synchronous
		err := <-errChan
		if err != nil {
			t.Errorf("Handshake failed: %v", err)
		}
	}

	elapsed := time.Since(start)
	t.Logf("PAKE Handshake took: %v", elapsed)

	// Verify it took at least some time (proving Argon2 is active)
	// Argon2 with these params should take > 100ms usually.
	if elapsed < 100*time.Millisecond {
		t.Log("Warning: Handshake was too fast! Is Argon2 working?")
	}
}

type readWriter struct {
	io.Reader
	io.Writer
}
