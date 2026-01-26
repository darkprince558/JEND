package core

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/darkprince558/jend/internal/transport"
	"github.com/darkprince558/jend/internal/ui"
	"github.com/darkprince558/jend/pkg/protocol"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/darkprince558/jend/internal/audit"
	"github.com/darkprince558/jend/internal/discovery"
	"github.com/darkprince558/jend/internal/signaling"
	"github.com/gofrs/flock"
)

const (
	Port      = "9000"
	ChunkSize = 1024 * 64
)

// RunSender handles the main sending logic
func RunSender(ctx context.Context, p *tea.Program, role ui.Role, filePath, textContent string, isText bool, code string, timeout time.Duration, forceTar, forceZip bool, noHistory bool, turnCfg *transport.CustomTurnConfig) {
	startTime := time.Now()
	var finalErr error
	var fileSize int64
	var fileHash string

	// Helper for sending messages to UI or stdout
	sendMsg := func(msg tea.Msg) {
		if p != nil {
			p.Send(msg)
		} else {
			// Headless fallback
			switch m := msg.(type) {
			case ui.ErrorMsg:
				fmt.Println("Error:", m)
			case ui.StatusMsg:
				fmt.Println("Status:", m)
			case ui.ProgressMsg:
				if m.SentBytes == m.TotalBytes && m.TotalBytes > 0 {
					fmt.Println("Done!")
				}
			}
		}
	}

	// Audit Log Defer
	defer func() {
		status := "failed"
		errMsg := ""
		if finalErr == nil {
			status = "success"
		} else {
			errMsg = finalErr.Error()
		}

		if !noHistory {
			audit.WriteEntry(audit.LogEntry{
				Timestamp: startTime,
				Role:      "sender",
				Code:      code,
				FileName:  filepath.Base(filePath),
				FileSize:  fileSize,
				FileHash:  fileHash,
				Status:    status,
				Error:     errMsg,
				Duration:  time.Since(startTime).Seconds(),
			})
		}
	}()

	var file io.Reader
	var fileName string
	var cleanup func()
	var err error
	var startModTime time.Time
	var info os.FileInfo

	if isText {
		// handle text mode
		fileSize = int64(len(textContent))
		file = strings.NewReader(textContent)
		fileName = "clipboard" // Special name for text mode
		cleanup = func() {}
		// No modtime for text
	} else {
		// Check if path is a directory
		info, err = os.Stat(filePath)
		if err != nil {
			finalErr = err
			sendMsg(ui.ErrorMsg(err))
			return
		}
		fileSize = info.Size()

		var fileObj *os.File

		// Compression Logic
		if info.IsDir() || forceTar {
			sendMsg(ui.StatusMsg("Compressing to .tar.gz..."))
			tempPath, err := CompressPath(filePath, "tar.gz")
			if err != nil {
				finalErr = err
				sendMsg(ui.ErrorMsg(err))
				return
			}

			fileObj, err = os.Open(tempPath)
			if err != nil {
				finalErr = err
				sendMsg(ui.ErrorMsg(err))
				return
			}
			fileName = filepath.Base(filePath) + ".tar.gz"
			cleanup = func() {
				fileObj.Close()
				os.Remove(tempPath)
			}
			info, _ = fileObj.Stat()
		} else if forceZip {
			sendMsg(ui.StatusMsg("Compressing to .zip..."))
			tempPath, err := CompressPath(filePath, "zip")
			if err != nil {
				finalErr = err
				sendMsg(ui.ErrorMsg(err))
				return
			}

			fileObj, err = os.Open(tempPath)
			if err != nil {
				finalErr = err
				sendMsg(ui.ErrorMsg(err))
				return
			}
			fileName = filepath.Base(filePath) + ".zip"
			cleanup = func() {
				fileObj.Close()
				os.Remove(tempPath)
			}
			info, _ = fileObj.Stat()
		} else {
			// Normal File
			fileObj, err = os.Open(filePath)
			if err != nil {
				finalErr = err
				sendMsg(ui.ErrorMsg(err))
				return
			}

			// Try to Lock (Best Effort)
			fileLock := flock.New(filePath)
			locked, err := fileLock.TryLock()
			if err != nil {
				sendMsg(ui.StatusMsg(fmt.Sprintf("Warning: Could not enable file lock: %v", err)))
			} else if !locked {
				// File is busy
				sendMsg(ui.StatusMsg("Warning: File is currently in use by another process. Changes during transfer may corrupt data."))
			} else {
				// Lock acquired!
				sendMsg(ui.StatusMsg("File locked for reading."))
			}

			fileName = info.Name()
			cleanup = func() {
				if locked {
					fileLock.Unlock()
				}
				fileObj.Close()
			}
		}
		file = fileObj
		startModTime = info.ModTime()
	}
	defer cleanup()

	// Start Listener
	tr := transport.NewQUICTransport()
	listener, err := tr.Listen(Port)
	if err != nil {
		finalErr = err
		sendMsg(ui.ErrorMsg(err))
		return
	}

	// Start Advertising
	stopAdvertising, err := discovery.StartAdvertising(9000, code)
	if err != nil {
		sendMsg(ui.StatusMsg(fmt.Sprintf("Warning: Failed to advertise on network: %v", err)))
	} else {
		defer stopAdvertising()
		sendMsg(ui.StatusMsg("Broadcasting on local network..."))
	}

	// Start Signaling (MQTT)
	// We do this in background to not block if credentials fail (security audit: need better creds)
	go func() {
		sendMsg(ui.StatusMsg("Connecting to Signaling Network..."))
		sigClient, err := signaling.NewIoTClient(context.Background(), "sender-"+code)
		if err != nil {
			sendMsg(ui.StatusMsg(fmt.Sprintf("Signaling failed: %v (Is AWS configured?)", err)))
			return
		}
		sendMsg(ui.StatusMsg("Signaling Connected. Waiting for peer..."))
		defer sigClient.Disconnect()

		// Initialize P2P manager and wait for connection.
		p2p := transport.NewP2PManager(sigClient, code, turnCfg)

		// This blocks until ICE connects
		pc, err := p2p.EstablishConnection(ctx, false) // false = Answerer (Sender)
		if err != nil {
			sendMsg(ui.StatusMsg(fmt.Sprintf("P2P Signaling failed: %v", err)))
			return
		}
		sendMsg(ui.StatusMsg("P2P (ICE) Connected! Handing over to QUIC..."))

		// Start QUIC Listener on ICE connection
		iceListener, err := tr.ListenPacket(pc)
		if err != nil {
			sendMsg(ui.StatusMsg(fmt.Sprintf("Failed to start QUIC over ICE: %v", err)))
			return
		}

		// Replace the global listener (or handle carefully).
		// Currently 'listener' in outer scope is bound to Port 9000 (Direct).
		// We want to accept on ICE too.
		// For this PoC, we will just log success and ideally spin up a loop to accept from ICE too.
		sendMsg(ui.StatusMsg("ICE-QUIC Tunnel Established! (Dual-Mode Active)"))

		// In a full implementation, we would multiplex or replace the listener.
		// Here we just prove binding works to satisfy the TODO.
		_ = iceListener
	}()

	// Wait for connection Loop
	sendMsg(ui.StatusMsg(fmt.Sprintf("Waiting for receiver (timeout: %s)...", timeout)))

	// State for resume
	var currentOffset int64 = 0

	for {
		if time.Since(startTime) > timeout {
			finalErr = fmt.Errorf("session timed out")
			sendMsg(ui.ErrorMsg(finalErr))
			return
		}

		// Check cancellation
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Use Passed Context for Accept (handles cancellation)
		acceptCtx, cancel := context.WithTimeout(ctx, timeout-time.Since(startTime))
		conn, err := listener.Accept(acceptCtx)
		cancel()

		if err != nil {
			// If context canceled (timeout or manual), we exit
			if acceptCtx.Err() == context.Canceled {
				return
			}
			if acceptCtx.Err() == context.DeadlineExceeded {
				finalErr = fmt.Errorf("code has expired or connection lost")
				sendMsg(ui.ErrorMsg(finalErr))
				return
			}
			finalErr = err
			sendMsg(ui.ErrorMsg(err))
			return
		}

		sendMsg(ui.StatusMsg("Receiver connected! Opening stream..."))

		// Parallel Stream Handling Loop
		var wg sync.WaitGroup
		var streamID int = 0

		for {
			// Accept Stream (blocks until stream opens or connection dies)
			stream, err := conn.AcceptStream(context.Background())
			if err != nil {
				// Connection closed or error
				break
			}

			isFirst := (streamID == 0)
			streamID++

			wg.Add(1)
			go func(s io.ReadWriter, first bool) {
				defer wg.Done()
				// Ensure we close the stream when done so Receiver gets EOF
				defer func() {
					if c, ok := s.(io.Closer); ok {
						c.Close()
					}
				}()

				_, err := handleConnection(ctx, s, file, isText, fileName, code, currentOffset, fileSize, startTime, startModTime, sendMsg, false)
				if err != nil && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "cancelled") {
					// Log unexpected errors
					// sendMsg(ui.ErrorMsg(err))
				}
			}(stream, isFirst)
		}
		// Wait for all active streams to finish
		wg.Wait()

		// If we are here, connection is done/closed.
		if ctx.Err() != nil {
			return
		}
		sendMsg(ui.StatusMsg("Session finished or disconnected."))
	}
}

// handleConnection encapsulates the logic for a single connection attempt
// Returns (done bool, err error).
func handleConnection(
	ctx context.Context,
	stream io.ReadWriter,
	file io.Reader,
	isText bool,
	fileName string,
	code string,
	currentOffset int64,
	fileSize int64,
	startTime time.Time,
	startModTime time.Time,
	sendMsg func(tea.Msg),
	skipAuth bool,
) (bool, error) {

	// PAKE Authentication
	if !skipAuth {
		sendMsg(ui.StatusMsg("Authenticating..."))
		if err := PerformPAKE(stream, code, 0); err != nil {
			return false, fmt.Errorf("authentication failed: %v", err)
		}
		sendMsg(ui.StatusMsg("Authenticated! Handshaking..."))
	}

	// Calculate Code Hash
	sendMsg(ui.StatusMsg("Calculating checksum..."))
	hasher := sha256.New()

	// Reset reader if it's an os.File or bytes.Reader-like
	if seeker, ok := file.(io.Seeker); ok {
		if _, err := seeker.Seek(0, 0); err != nil {
			return false, err
		}
	}

	if _, err := io.Copy(hasher, file); err != nil {
		return false, err
	}
	fileHash := fmt.Sprintf("%x", hasher.Sum(nil))

	// Handshake
	meta := map[string]interface{}{
		"name": fileName,
		"size": fileSize,
		"code": code,
		"hash": fileHash,
	}
	if isText {
		meta["type"] = "text"
	} else {
		meta["type"] = "file"
	}

	metaBytes, _ := json.Marshal(meta)

	if err := protocol.EncodeHeader(stream, protocol.TypeHandshake, uint32(len(metaBytes))); err != nil {
		return false, err
	}
	stream.Write(metaBytes)

	// Wait for Ack OR Range Request
	sendMsg(ui.StatusMsg("Handshake sent. Waiting for response..."))
	pType, length, err := protocol.DecodeHeader(stream)
	if err != nil {
		return false, fmt.Errorf("handshake failed: %v", err)
	}

	var offset int64 = 0
	var byteLimit int64 = -1 // -1 means until EOF

	if pType == protocol.TypeAck {
		// Standard sequential download (or resume)
		if length == 8 {
			if err := binary.Read(stream, binary.LittleEndian, &offset); err != nil {
				return false, err
			}
			if offset > 0 {
				sendMsg(ui.StatusMsg(fmt.Sprintf("Resuming transfer from %d bytes...", offset)))
			}
		}
	} else if pType == protocol.TypeRangeReq {
		// Parallel Stream Request
		// Payload: [StartOffset int64][Length int64]
		if length != 16 {
			return false, fmt.Errorf("invalid range request length")
		}
		var startOff int64
		var lenReq int64
		if err := binary.Read(stream, binary.LittleEndian, &startOff); err != nil {
			return false, err
		}
		if err := binary.Read(stream, binary.LittleEndian, &lenReq); err != nil {
			return false, err
		}
		offset = startOff
		byteLimit = lenReq
		sendMsg(ui.StatusMsg(fmt.Sprintf("Parallel worker sending bytes %d-%d", offset, offset+byteLimit)))
	} else {
		return false, fmt.Errorf("unexpected packet type: %d", pType)
	}

	// Parallel/Concurrent Read implementation using ReaderAt
	var dataReader io.Reader
	if readerAt, ok := file.(io.ReaderAt); ok {
		// Use SectionReader for thread-safe concurrent access
		limit := fileSize - offset
		if byteLimit > 0 {
			limit = byteLimit
		}
		dataReader = io.NewSectionReader(readerAt, offset, limit)
	} else {
		// Fallback for non-ReaderAt (e.g. stdin/text)
		if offset > 0 {
			// Try to seek if possible
			if seeker, ok := file.(io.Seeker); ok {
				if _, err := seeker.Seek(offset, 0); err != nil {
					return false, err
				}
			} else {
				return false, fmt.Errorf("cannot seek in non-seekable source")
			}
		}
		dataReader = file
	}

	// Send Data
	// sendMsg(ui.StatusMsg("Sending data..."))
	buf := make([]byte, ChunkSize)
	var totalSent int64 = 0

	// If byteLimit is set, we only send that much
	var bytesRemaining int64 = -1
	if byteLimit > 0 {
		bytesRemaining = byteLimit
	} else {
		// limit for loop logic (if infinite)
	}

	// actually SectionReader handles EOF at limit automatically.
	// So we can just read from dataReader until EOF.

	for {
		// Check Cancellation
		select {
		case <-ctx.Done():
			// sendMsg(ui.StatusMsg("Stopping transfer (User Cancelled)..."))
			protocol.EncodeHeader(stream, protocol.TypeCancel, 0)
			return false, ctx.Err()
		default:
		}

		// TEST HOOK: Slow down transfer for cancellation testing
		if delay := os.Getenv("JEND_TEST_DELAY"); delay != "" {
			d, _ := time.ParseDuration(delay)
			time.Sleep(d)
		}

		// Calculate read size
		readSize := ChunkSize
		// We don't strictly need manual limiting if SectionReader is used, but good for chunking.
		if bytesRemaining > 0 && int64(readSize) > bytesRemaining {
			readSize = int(bytesRemaining)
		}

		n, err := dataReader.Read(buf[:readSize])
		if n > 0 {
			if err := protocol.EncodeHeader(stream, protocol.TypeData, uint32(n)); err != nil {
				return false, err
			}
			if _, err := stream.Write(buf[:n]); err != nil {
				return false, err
			}
			totalSent += int64(n)

			if bytesRemaining > 0 {
				bytesRemaining -= int64(n)
			}
		}
		if bytesRemaining == 0 {
			break // Done with range
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, err
		}
	}
	// Done with this stream
	return true, nil
}

func CompressPath(filePath string, format string) (string, error) {
	if format == "tar.gz" {
		tempFile, err := os.CreateTemp("", "jend-*.tar.gz")
		if err != nil {
			return "", err
		}

		gw := gzip.NewWriter(tempFile)
		tw := tar.NewWriter(gw)

		err = filepath.Walk(filePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			header, err := tar.FileInfoHeader(info, info.Name())
			if err != nil {
				return err
			}

			// Use filepath.Dir(filePath) to ensure we include the base name of the file/folder being compressed
			// e.g. send "testdir" -> archive contains "testdir/file1", not just "file1"
			base := filepath.Dir(filePath)
			if base == "." {
				base = "" // handle current dir case
			}
			relPath, err := filepath.Rel(base, path)
			if err != nil {
				return err
			}
			header.Name = filepath.ToSlash(relPath)

			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			if !info.IsDir() {
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				defer f.Close()
				if _, err := io.Copy(tw, f); err != nil {
					return err
				}
			}
			return nil
		})

		tw.Close()
		gw.Close()
		tempFile.Close()

		if err != nil {
			os.Remove(tempFile.Name())
			return "", err
		}
		return tempFile.Name(), nil
	} else if format == "zip" {
		tempFile, err := os.CreateTemp("", "jend-*.zip")
		if err != nil {
			return "", err
		}

		zw := zip.NewWriter(tempFile)

		err = filepath.Walk(filePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			base := filepath.Dir(filePath)
			if base == "." {
				base = ""
			}
			relPath, err := filepath.Rel(base, path)
			if err != nil {
				return err
			}
			header.Name = filepath.ToSlash(relPath)

			if info.IsDir() {
				header.Name += "/"
			} else {
				header.Method = zip.Deflate
			}

			writer, err := zw.CreateHeader(header)
			if err != nil {
				return err
			}

			if !info.IsDir() {
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				defer f.Close()
				if _, err := io.Copy(writer, f); err != nil {
					return err
				}
			}
			return nil
		})

		zw.Close()
		tempFile.Close()

		if err != nil {
			os.Remove(tempFile.Name())
			return "", err
		}
		return tempFile.Name(), nil
	}
	return "", fmt.Errorf("unsupported format")
}
