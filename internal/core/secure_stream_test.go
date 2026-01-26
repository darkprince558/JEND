package core

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
)

func TestSecureStream(t *testing.T) {
	// 1. Generate a random key
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatal(err)
	}

	// 2. Create a pipe to simulate a network connection
	// internal buffer acts as the wire
	var wire bytes.Buffer

	// 3. Create Writer
	writer, err := NewSecureStream(&wire, key)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	// 4. Create Reader
	// Note: We use the same 'wire' buffer. In reality, this would be two ends of a net.Conn
	reader, err := NewSecureStream(&wire, key)
	if err != nil {
		t.Fatalf("Failed to create reader: %v", err)
	}

	// 5. Write Data (Small Chunk)
	message := []byte("Hello, Encrypted World!")
	n, err := writer.Write(message)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(message) {
		t.Errorf("Short write: %d vs %d", n, len(message))
	}

	// 6. Read Data
	readBuf := make([]byte, 1024)
	n, err = reader.Read(readBuf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if !bytes.Equal(readBuf[:n], message) {
		t.Errorf("Decrypted message mismatch.\nGot: %s\nWant: %s", readBuf[:n], message)
	}

	// 7. Test Large Data (Multiple Frames)
	largeMsg := make([]byte, 20000) // Larger than potential frame limits if we had them, testing buffer logic
	rand.Read(largeMsg)

	// Write in one go
	_, err = writer.Write(largeMsg)
	if err != nil {
		t.Fatal(err)
	}

	// Read in loop
	received := make([]byte, 0, len(largeMsg))
	tmp := make([]byte, 1024)
	for len(received) < len(largeMsg) {
		n, err := reader.Read(tmp)
		if err != nil {
			t.Fatal(err)
		}
		received = append(received, tmp[:n]...)
	}

	if !bytes.Equal(received, largeMsg) {
		t.Error("Large message mismatch")
	}
}
