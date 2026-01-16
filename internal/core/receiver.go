package core

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/darkprince558/jend/internal/transport"
	"github.com/darkprince558/jend/internal/ui"
	"github.com/darkprince558/jend/pkg/protocol"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/darkprince558/jend/internal/audit"
	"github.com/darkprince558/jend/internal/discovery"
)

// RunReceiver handles the main receiving logic
func RunReceiver(p *tea.Program, code string, outputDir string, autoUnzip bool, noClipboard bool, noHistory bool) {
	sendMsg := func(msg tea.Msg) {
		if p != nil {
			p.Send(msg)
		} else {
			switch m := msg.(type) {
			case ui.ErrorMsg:
				fmt.Println("Error:", m)
				// os.Exit(1) handled in defer
			case ui.StatusMsg:
				fmt.Println("Status:", m)
			case ui.ProgressMsg:
				if m.TotalBytes > 0 && m.SentBytes == m.TotalBytes {
					fmt.Println("Done!")
				}
			}
		}
	}

	time.Sleep(time.Second * 1) // Fake discovery time

	startTime := time.Now()
	var finalErr error
	var fileHash string
	var fileSize int64
	var exitCode int

	// Audit Log Defer
	defer func() {
		status := "failed"
		errMsg := ""
		if finalErr == nil {
			status = "success"
		} else {
			errMsg = finalErr.Error()
			if p == nil {
				exitCode = 1
			}
		}

		if !noHistory {
			audit.WriteEntry(audit.LogEntry{
				Timestamp: startTime,
				Role:      "receiver",
				Code:      code,
				FileName:  filepath.Base(outputDir), // Rough approximation or update later
				FileSize:  fileSize,
				FileHash:  fileHash,
				Status:    status,
				Error:     errMsg,
				Duration:  time.Since(startTime).Seconds(),
			})
		}

		if p == nil && exitCode != 0 {
			os.Exit(exitCode)
		}
	}()

	sendMsg(ui.StatusMsg("Searching for sender on local network..."))

	// Create a transport early
	tr := transport.NewQUICTransport()

	// Try Discovery
	address := "localhost:" + Port
	foundIP, err := discovery.FindSender(code, 5*time.Second)
	if err == nil {
		sendMsg(ui.StatusMsg(fmt.Sprintf("Found sender at %s!", foundIP)))
		address = foundIP
	} else {
		sendMsg(ui.StatusMsg("Discovery timed out, trying localhost..."))
	}

	// Main Receiver Loop
	// We will attempt to authenticate and resume until complete or fatal error

	retryCount := 0
	maxRetries := 10 // Global retries for connection establishment

	for {
		// Discovery Logic (Simplified: try once then use last known IP or localhost)
		// ... (Existing discovery logic)
		// For loop simplicity, we'll just dial 'address' which was resolved earlier
		// or re-resolve if needed.

		// If address is empty or we want to re-discover:
		// (Ideally move discovery inside loop, but for now stick to address)

		sendMsg(ui.StatusMsg("Dialing " + address + "..."))
		conn, err := tr.Dial(address)
		if err != nil {
			retryCount++
			if retryCount > maxRetries {
				finalErr = err
				sendMsg(ui.ErrorMsg(fmt.Errorf("max retries exceeded: %v", err)))
				return
			}
			sendMsg(ui.StatusMsg(fmt.Sprintf("Connection failed. Retrying in %d seconds...", retryCount)))
			time.Sleep(time.Duration(retryCount) * time.Second)
			continue
		}

		// Reset retry count on successful dial
		retryCount = 0
		sendMsg(ui.StatusMsg("Connected! Opening stream..."))

		stream, err := conn.OpenStreamSync(context.Background())
		if err != nil {
			sendMsg(ui.ErrorMsg(fmt.Errorf("failed to open stream: %v", err)))
			conn.CloseWithError(0, "stream open failed")
			time.Sleep(time.Second)
			continue
		}

		// Handle Session
		done, size, hash, err := handleReceiveSession(stream, code, outputDir, autoUnzip, noClipboard, sendMsg)
		fileSize = size
		fileHash = hash // approximate, might be partial if failed, but better than empty

		if done {
			// Success!
			return
		}

		if err != nil {
			// Check for cancellation
			if strings.Contains(err.Error(), "transfer cancelled by sender") {
				finalErr = err
				sendMsg(ui.ErrorMsg(err))
				return
			}
			sendMsg(ui.StatusMsg(fmt.Sprintf("Transfer interrupted (%v). Retrying...", err)))
			stream.Close()
			// Close connection if not already closed
			// QUIC connection Closing is handled by CloseWithError mostly, but Close() works too.
			// quic-go's Connection interface has CloseWithError, not Close.
			// But our Transport wrapper returns quic.Connection which has CloseWithError.
			conn.CloseWithError(0, "interrupted")
			time.Sleep(time.Second)
			continue
		}
	}
}

// handleReceiveSession encapsulates the logic for a single resume attempt
func handleReceiveSession(
	stream io.ReadWriter,
	code string,
	outputDir string,
	autoUnzip bool,
	noClipboard bool,
	sendMsg func(tea.Msg),
) (bool, int64, string, error) {
	var fileSize int64
	var fileHash string

	// PAKE Authentication
	sendMsg(ui.StatusMsg("Authenticating..."))
	if err := PerformPAKE(stream, code, 1); err != nil {
		return false, 0, "", fmt.Errorf("authentication failed: %v", err)
	}
	sendMsg(ui.StatusMsg("Authenticated! Waiting for handshake..."))

	// Read Handshake
	pType, length, err := protocol.DecodeHeader(stream)
	if err != nil || pType != protocol.TypeHandshake {
		return false, 0, "", fmt.Errorf("invalid handshake")
	}

	metaBytes := make([]byte, length)
	if _, err := io.ReadFull(stream, metaBytes); err != nil {
		return false, 0, "", err
	}

	var meta struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		Code string `json:"code"`
		Hash string `json:"hash"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return false, 0, "", err
	}
	fileSize = meta.Size
	// Update audit log filename if we have it now
	// Ideally we could update the struct in the defer, referencing variables.
	// Since we use closure variables, setting fileName var (if we had one) would work.
	// But we initialized log entry in defer. I'll add a fileName var in the scope.

	// Handle Text Mode
	if meta.Type == "text" {
		// Just check size warnings
		sendMsg(ui.StatusMsg("Receiving text snippet..."))

		// Read all data
		// Limit size for safety (e.g. 1MB for text)
		limit := int64(1 * 1024 * 1024)
		if meta.Size > limit {
			return false, meta.Size, "", fmt.Errorf("text content too large (>1MB)")
		}

		// We can't use ReadFull directly if it's chunked via TypeData...
		// Wait, the protocol sends TypeData chunks. We reusing the loop?
		// Reusing the loop logic is better than rewriting it.
		// But the loop writes to mw (MultiWriter -> file + hasher).
		// We can point mw to a bytes.Buffer instead of a file.
	}

	// Send Ack
	// Check for existing partial file to resume
	safeName := filepath.Base(meta.Name)
	if safeName == "." || safeName == "/" {
		safeName = "received_file"
	}

	// Strategy: Always download to .partial
	// Resume checks .partial file
	// On success, strip .partial and handle collisions
	partialPath := filepath.Join(outputDir, safeName+".partial")
	var offset int64 = 0

	if meta.Type != "text" {
		if info, err := os.Stat(partialPath); err == nil {
			if info.Size() < meta.Size && info.Size() > 0 {
				offset = info.Size()
				sendMsg(ui.StatusMsg(fmt.Sprintf("Partial download found. Resuming from %d bytes...", offset)))
			}
		}
	}

	if err := protocol.EncodeHeader(stream, protocol.TypeAck, 8); err != nil {
		return false, fileSize, "", err
	}
	if err := binary.Write(stream, binary.LittleEndian, offset); err != nil {
		return false, fileSize, "", err
	}

	sendMsg(ui.StatusMsg("Receiving " + safeName))

	// Ensure output directory exists (optional, but good practice)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return false, fileSize, "", fmt.Errorf("failed to create output dir: %w", err)
		}
	}

	var outFile io.WriteCloser
	var textBuf *bytes.Buffer

	if meta.Type == "text" {
		textBuf = new(bytes.Buffer)
		// wrapper to satisfy WriteCloser
		outFile = &nopCloser{textBuf}
	} else {
		var f *os.File
		if offset > 0 {
			// Resume: Open in Append mode
			f, err = os.OpenFile(partialPath, os.O_WRONLY|os.O_APPEND, 0644)
		} else {
			// New: Create/Truncate
			f, err = os.Create(partialPath)
		}
		if err != nil {
			return false, fileSize, "", err
		}
		outFile = f
	}
	defer outFile.Close()

	// Receive Loop
	buf := make([]byte, ChunkSize)
	var totalRecv int64 = offset
	startTime := time.Now()

	hasher := sha256.New()

	// If resuming, we must hash the existing part first so the final hash matches the full file
	if offset > 0 {
		existingFile, err := os.Open(partialPath)
		if err != nil {
			return false, fileSize, "", err
		}
		if _, err := io.CopyN(hasher, existingFile, offset); err != nil {
			existingFile.Close()
			return false, fileSize, "", err
		}
		existingFile.Close()
	}

	mw := io.MultiWriter(outFile, hasher)

	for {
		pType, length, err := protocol.DecodeHeader(stream)
		if err != nil {
			if err == io.EOF {
				break
			}
			// If we received all data but connection dropped (e.g. sender closed improperly or timed out), treat as success
			if totalRecv == meta.Size {
				break
			}
			return false, fileSize, "", err
		}

		if pType == protocol.TypeCancel {
			return false, fileSize, "", fmt.Errorf("transfer cancelled by sender")
		}

		if pType == protocol.TypeData {
			// Reallocate if buf too small
			if uint32(len(buf)) < length {
				buf = make([]byte, length)
			}
			if _, err := io.ReadFull(stream, buf[:length]); err != nil {
				return false, fileSize, "", err
			}
			mw.Write(buf[:length])
			totalRecv += int64(length)

			// Calculate Telemetry
			elapsed := time.Since(startTime).Seconds()
			var speed float64
			var eta time.Duration
			if elapsed > 0 {
				speed = float64(totalRecv) / elapsed
				if speed > 0 {
					eta = time.Duration(float64(meta.Size-totalRecv)/speed) * time.Second
				}
			}

			sendMsg(ui.ProgressMsg{
				SentBytes:  totalRecv,
				TotalBytes: meta.Size,
				Speed:      speed,
				ETA:        eta,
				Protocol:   "QUIC (Direct)",
			})
		}
	}

	// Close stream using type assertion if needed, or rely on connection close.
	// io.ReadWriter doesn't have Close.
	if c, ok := stream.(io.Closer); ok {
		c.Close()
	}
	sendMsg(ui.ProgressMsg{
		SentBytes:  meta.Size,
		TotalBytes: meta.Size,
		Speed:      0,
		ETA:        0,
		Protocol:   "Done",
	})

	// Close explicitly to allow rename
	outFile.Close()

	// Verify Checksum
	finalPath := filepath.Join(outputDir, safeName)
	if meta.Hash != "" {
		recvHash := fmt.Sprintf("%x", hasher.Sum(nil))
		if recvHash == meta.Hash {
			sendMsg(ui.StatusMsg("Integrity Check: PASSED"))

			if meta.Type == "text" {
				content := textBuf.String()
				fmt.Printf("\nReceived Text:\n%s\n", content)
				if !noClipboard {
					if err := clipboard.WriteAll(content); err == nil {
						sendMsg(ui.StatusMsg("Text copied to clipboard!"))
					} else {
						sendMsg(ui.StatusMsg("Failed to copy to clipboard"))
					}
				} else {
					sendMsg(ui.StatusMsg("Clipboard copy skipped (--no-clipboard)"))
				}
				return true, fileSize, meta.Hash, nil
			}

			// Safe Move Logic
			counter := 0
			// Find a non-colliding name
			for {
				if _, err := os.Stat(finalPath); os.IsNotExist(err) {
					break
				}
				counter++
				ext := filepath.Ext(safeName)
				nameBox := strings.TrimSuffix(safeName, ext)
				finalPath = filepath.Join(outputDir, fmt.Sprintf("%s (%d)%s", nameBox, counter, ext))
			}

			if err := os.Rename(partialPath, finalPath); err != nil {
				return false, fileSize, "", fmt.Errorf("failed to save final file: %v", err)
			}
			fileHash = meta.Hash // Set hash for audit log only on success
			sendMsg(ui.StatusMsg("Saved to: " + filepath.Base(finalPath)))

		} else {
			return false, fileSize, "", fmt.Errorf("Integrity Check: FAILED (Expected %s, Got %s).", meta.Hash, recvHash)
		}
	} else {
		if meta.Type == "text" {
			content := textBuf.String()
			fmt.Printf("\nReceived Text:\n%s\n", content)
			if !noClipboard {
				clipboard.WriteAll(content)
			}
			return true, fileSize, "", nil
		}

		// No hash provided, just move it (risky but consistent with old logic)
		os.Rename(partialPath, finalPath)
		sendMsg(ui.StatusMsg("Integrity Check: SKIPPED (No hash provided)"))
	}

	time.Sleep(time.Second)

	// Auto-Unzip Logic
	if autoUnzip {
		ext := filepath.Ext(safeName)
		if strings.HasSuffix(safeName, ".tar.gz") {
			sendMsg(ui.StatusMsg("Unzipping .tar.gz archive..."))
			// Re-open the file
			f, err := os.Open(finalPath)
			if err != nil {
				return true, fileSize, fileHash, err // Return true because transfer succeeded, unzip failed
			}
			defer f.Close()

			gzr, err := gzip.NewReader(f)
			if err != nil {
				return true, fileSize, fileHash, err
			}
			defer gzr.Close()

			tr := tar.NewReader(gzr)

			for {
				header, err := tr.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					return true, fileSize, fileHash, err
				}

				// Zip Slip Protection
				target := filepath.Join(outputDir, header.Name)
				if !strings.HasPrefix(target, filepath.Clean(outputDir)+string(os.PathSeparator)) {
					// log.Println("zip slip attempt detected")
					continue
				}

				if header.Typeflag == tar.TypeDir {
					if err := os.MkdirAll(target, 0755); err != nil {
						return true, fileSize, fileHash, err
					}
				} else if header.Typeflag == tar.TypeReg {
					f, err := os.Create(target)
					if err != nil {
						return true, fileSize, fileHash, err
					}
					if _, err := io.Copy(f, tr); err != nil {
						f.Close()
						return true, fileSize, fileHash, err
					}
					f.Close()
				}
			}
			sendMsg(ui.StatusMsg("Extracted successfully!"))

		} else if ext == ".zip" {
			sendMsg(ui.StatusMsg("Unzipping .zip archive..."))

			// zip.OpenReader requires random access, safe since we have the file on disk
			zr, err := zip.OpenReader(finalPath)
			if err != nil {
				return true, fileSize, fileHash, err
			}
			defer zr.Close()

			for _, f := range zr.File {
				fpath := filepath.Join(outputDir, f.Name)

				// Check for Zip Slip
				if !strings.HasPrefix(fpath, filepath.Clean(outputDir)+string(os.PathSeparator)) {
					continue
				}

				if f.FileInfo().IsDir() {
					os.MkdirAll(fpath, os.ModePerm)
					continue
				}

				if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
					return true, fileSize, fileHash, err
				}

				outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
				if err != nil {
					return true, fileSize, fileHash, err
				}

				rc, err := f.Open()
				if err != nil {
					outFile.Close()
					return true, fileSize, fileHash, err
				}

				_, err = io.Copy(outFile, rc)
				outFile.Close()
				rc.Close()
				if err != nil {
					return true, fileSize, fileHash, err
				}
			}
		}
	}
	return true, fileSize, fileHash, nil
}

func PerformPAKE(stream io.ReadWriter, password string, role int) error {
	// Custom Robust Mutual Authentication (replacing crashing schollz/pake)
	// Role 0 = Sender (Verifier), Role 1 = Receiver (Prover/Client)
	// Uses HMAC-SHA256 with Salt and Session Nonce.

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

	// 2. Derive Session Key K = SHA256(Password + Salt)
	// In production, use Argon2 or Scrypt. Here using SHA256 for simplicity/speed in prototype.
	keyHash := sha256.Sum256(append([]byte(password), salt...))
	K := keyHash[:]

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
	// Import crypto/hmac needed?
	// Or use simple SHA256 for now? Receiver.go imports sha256.
	// We need HMAC. import "crypto/hmac"
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// verifySessionKey removed as it is integrated above
// Ensure crypto/hmac and crypto/rand are imported

type nopCloser struct {
	io.Writer
}

func (n *nopCloser) Close() error {
	return nil
}
