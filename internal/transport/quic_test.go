package transport

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestQUICConnection(t *testing.T) {
	tr := NewQUICTransport()
	port := "9999"

	// Start Listener
	listener, err := tr.Listen(port)
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	done := make(chan struct{})

	// Accept Loop
	go func() {
		defer close(done)
		conn, err := listener.Accept(context.Background())
		if err != nil {
			t.Errorf("Accept error: %v", err)
			return
		}
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			t.Errorf("AcceptStream error: %v", err)
			return
		}
		buf := make([]byte, 5)
		if _, err := io.ReadFull(stream, buf); err != nil {
			t.Errorf("ReadFull error: %v", err)
			return
		}
		if string(buf) != "HELLO" {
			t.Errorf("Expected HELLO, got %s", buf)
		}
		stream.Close()
	}()

	// Dial
	conn, err := tr.Dial("localhost:" + port)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}

	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		t.Fatalf("OpenStreamSync error: %v", err)
	}

	if _, err := stream.Write([]byte("HELLO")); err != nil {
		t.Fatalf("Write error: %v", err)
	}
	stream.Close()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out")
	}
}
