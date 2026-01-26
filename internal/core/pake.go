package core

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"io"

	"github.com/darkprince558/jend/pkg/protocol"
	"golang.org/x/crypto/argon2"
)

// PAKE Constants
const (
	ArgonTime    = 3
	ArgonMemory  = 64 * 1024 // 64 MB
	ArgonThreads = 4
	ArgonKeyLen  = 32
)

// PerformPAKE executes a custom Mutual Authentication protocol using Argon2id + HMAC-SHA256
// and a challenge-response mechanism.
// It establishes that both parties share the same correct code/password without revealing it.
// role: 0 for Sender (Verifier), 1 for Receiver (Prover).
func PerformPAKE(stream io.ReadWriter, password string, role int) error {

	// Step 0: Sync Stream (Receiver speaks first to trigger AcceptStream on Server)
	if role == 1 { // Receiver
		if err := protocol.EncodeHeader(stream, protocol.TypePAKE, 0); err != nil {
			return err
		}
	} else { // Sender
		// Sender waits for Hello
		pType, _, err := protocol.DecodeHeader(stream)
		if err != nil {
			return err
		}
		if pType != protocol.TypePAKE {
			return fmt.Errorf("expected PAKE hello")
		}
	}

	// 1. Salt Exchange (Sender generates Salt)
	var salt []byte
	if role == 0 { // Sender
		salt = make([]byte, 16)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return err
		}
		// Send Salt
		if err := protocol.EncodeHeader(stream, protocol.TypePAKE, uint32(len(salt))); err != nil {
			return err
		}
		if _, err := stream.Write(salt); err != nil {
			return err
		}
	} else { // Receiver
		// Read Salt
		pType, length, err := protocol.DecodeHeader(stream)
		if err != nil {
			return err
		}
		if pType != protocol.TypePAKE {
			return fmt.Errorf("expected salt")
		}
		salt = make([]byte, length)
		if _, err := io.ReadFull(stream, salt); err != nil {
			return err
		}
	}

	// 2. Derive Session Key K = Argon2id(Password, Salt, ...)
	// Upgraded from SHA256 to Argon2id for brute-force resistance.
	K := argon2.IDKey([]byte(password), salt, ArgonTime, ArgonMemory, ArgonThreads, ArgonKeyLen)

	// 3. Mutual Challenge-Response
	// Sender generates Random Nonce N
	var nonce []byte
	if role == 0 { // Sender
		nonce = make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return err
		}
		// Send Nonce
		if err := protocol.EncodeHeader(stream, protocol.TypePAKE, uint32(len(nonce))); err != nil {
			return err
		}
		if _, err := stream.Write(nonce); err != nil {
			return err
		}
	} else { // Receiver
		// Read Nonce
		pType, length, err := protocol.DecodeHeader(stream)
		if err != nil {
			return err
		}
		if pType != protocol.TypePAKE {
			return fmt.Errorf("expected nonce")
		}
		nonce = make([]byte, length)
		if _, err := io.ReadFull(stream, nonce); err != nil {
			return err
		}
	}

	// 4. Receiver Authenticates First (sends HMAC(K, "client" + Nonce))
	clientTag := computeHMAC(K, append([]byte("client"), nonce...))

	if role == 1 { // Receiver sends proof
		if err := protocol.EncodeHeader(stream, protocol.TypePAKE, uint32(len(clientTag))); err != nil {
			return err
		}
		if _, err := stream.Write(clientTag); err != nil {
			return err
		}
	} else { // Sender verifies proof
		pType, length, err := protocol.DecodeHeader(stream)
		if err != nil {
			return err
		}
		if pType != protocol.TypePAKE {
			return fmt.Errorf("expected client proof")
		}
		gotTag := make([]byte, length)
		if _, err := io.ReadFull(stream, gotTag); err != nil {
			return err
		}
		if subtle.ConstantTimeCompare(gotTag, clientTag) != 1 {
			return fmt.Errorf("authentication failed: wrong password")
		}
	}

	// 5. Sender Authenticates (sends HMAC(K, "server" + Nonce))
	serverTag := computeHMAC(K, append([]byte("server"), nonce...))

	if role == 0 { // Sender sends proof
		if err := protocol.EncodeHeader(stream, protocol.TypePAKE, uint32(len(serverTag))); err != nil {
			return err
		}
		if _, err := stream.Write(serverTag); err != nil {
			return err
		}
	} else { // Receiver verifies proof
		pType, length, err := protocol.DecodeHeader(stream)
		if err != nil {
			return err
		}
		if pType != protocol.TypePAKE {
			return fmt.Errorf("expected server proof")
		}
		gotTag := make([]byte, length)
		if _, err := io.ReadFull(stream, gotTag); err != nil {
			return err
		}
		if subtle.ConstantTimeCompare(gotTag, serverTag) != 1 {
			return fmt.Errorf("server authentication failed")
		}
	}

	return nil
}

func computeHMAC(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
