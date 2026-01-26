package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	NonceSize  = 12
	TagSize    = 16
	HeaderSize = 4 + NonceSize // Length (4) + Nonce (12)
)

// SecureStream wraps an io.ReadWriter with AES-GCM encryption
type SecureStream struct {
	rw   io.ReadWriter
	aead cipher.AEAD

	// Read buffer state
	readBuf    []byte
	readOffset int
}

// NewSecureStream creates a new authenticated encryption stream
// key must be 32 bytes for AES-256
func NewSecureStream(rw io.ReadWriter, key []byte) (*SecureStream, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &SecureStream{
		rw:   rw,
		aead: gcm,
	}, nil
}

// Write encrypts the data and writes a frame: [Length][Nonce][Ciphertext+Tag]
func (s *SecureStream) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return 0, err
	}

	// Encrypt
	// Seal appends to dst, so we can pass nil
	ciphertext := s.aead.Seal(nil, nonce, p, nil)

	// Prepare Header: Length (uint32) of ciphertext
	frameLen := uint32(len(ciphertext))
	header := make([]byte, 4)
	binary.LittleEndian.PutUint32(header, frameLen)

	// Write Header
	if _, err := s.rw.Write(header); err != nil {
		return 0, err
	}
	// Write Nonce
	if _, err := s.rw.Write(nonce); err != nil {
		return 0, err
	}
	// Write Ciphertext
	if _, err := s.rw.Write(ciphertext); err != nil {
		return 0, err
	}

	return len(p), nil
}

// Read reads encrypted frames and returns plaintext
func (s *SecureStream) Read(p []byte) (n int, err error) {
	// If we have data in the buffer, return it
	if len(s.readBuf) > 0 {
		n = copy(p, s.readBuf[s.readOffset:])
		s.readOffset += n
		if s.readOffset >= len(s.readBuf) {
			s.readBuf = nil
			s.readOffset = 0
		}
		return n, nil
	}

	// Otherwise, read a new frame
	// 1. Read Length
	header := make([]byte, 4)
	if _, err := io.ReadFull(s.rw, header); err != nil {
		return 0, err
	}
	frameLen := binary.LittleEndian.Uint32(header)

	if frameLen > 10*1024*1024 { // Sanity check: 10MB max frame
		return 0, fmt.Errorf("oversized frame: %d", frameLen)
	}

	// 2. Read Nonce
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(s.rw, nonce); err != nil {
		return 0, err
	}

	// 3. Read Ciphertext
	ciphertext := make([]byte, frameLen)
	if _, err := io.ReadFull(s.rw, ciphertext); err != nil {
		return 0, err
	}

	// 4. Decrypt
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return 0, fmt.Errorf("decryption failed: %v", err)
	}

	// 5. Copy to p
	s.readBuf = plaintext
	s.readOffset = 0

	n = copy(p, s.readBuf)
	s.readOffset += n
	if s.readOffset >= len(s.readBuf) {
		s.readBuf = nil
		s.readOffset = 0
	}

	return n, nil
}
