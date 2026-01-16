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
	"time"

	"github.com/darkprince558/jend/internal/transport"
	"github.com/darkprince558/jend/internal/ui"
	"github.com/darkprince558/jend/pkg/protocol"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/darkprince558/jend/internal/audit"
	"github.com/darkprince558/jend/internal/discovery"
	"github.com/gofrs/flock"
)

const (
	Port      = "9000"
	ChunkSize = 1024 * 64
)

// RunSender handles the main sending logic
func RunSender(ctx context.Context, p *tea.Program, role ui.Role, filePath, textContent string, isText bool, code string, timeout time.Duration, forceTar, forceZip bool, noHistory bool) {
	startTime := time.Now()
	var finalErr error
	var fileSize int64
	var fileHash string

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
				// Lock failed (permission/system error), just warn
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
	// Note: We need to pass the transport or create it. For now creating new.
	tr := transport.NewQUICTransport()
	listener, err := tr.Listen(Port)
	if err != nil {
		finalErr = err
		sendMsg(ui.ErrorMsg(err))
		return
	}

	// Start Advertising
	stopAdvertising, err := discovery.StartAdvertising(9000, code) // TODO: Dynamic Port
	if err != nil {
		sendMsg(ui.StatusMsg(fmt.Sprintf("Warning: Failed to advertise on network: %v", err)))
	} else {
		defer stopAdvertising()
		sendMsg(ui.StatusMsg("Broadcasting on local network..."))
	}

	// Wait for connection Loop
	sendMsg(ui.StatusMsg(fmt.Sprintf("Waiting for receiver (timeout: %s)...", timeout)))

	// State for resume
	var currentOffset int64 = 0

	for {
		// Calculate remaining time
		// If we are resumed, we probably want to extend or reset timeout?
		// For now, strict total timeout.

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
				// Manual Cancellation
				return
			}
			if acceptCtx.Err() == context.DeadlineExceeded {
				// Only error if we haven't finished finding someone at least once?
				// Or fail completely if no successful transfer.

				// If we are in the middle of a transfer (some bytes sent), maybe let it slide?
				// But we rely on listener.Accept() returning.

				finalErr = fmt.Errorf("code has expired or connection lost")
				sendMsg(ui.ErrorMsg(finalErr))
				return // Timeout
			}
			finalErr = err
			sendMsg(ui.ErrorMsg(err))
			return
		}

		sendMsg(ui.StatusMsg("Receiver connected! Opening stream..."))

		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			sendMsg(ui.StatusMsg(fmt.Sprintf("Connection failed: %v. Waiting for retry...", err)))
			conn.CloseWithError(0, "stream failed")
			continue
		}

		// Handle this connection in a sub-function or block
		done, err := handleConnection(ctx, stream, file, isText, fileName, code, currentOffset, fileSize, startTime, startModTime, sendMsg)
		if done {
			// Transfer Complete
			return
		}

		if err != nil {
			if errors.Is(err, context.Canceled) {
				sendMsg(ui.StatusMsg("Closing connection (cancelled)..."))
				conn.CloseWithError(0, "transfer cancelled by sender")
				stream.Close()
				return
			}
			sendMsg(ui.StatusMsg(fmt.Sprintf("Connection lost (%v). Waiting for resume...", err)))
			conn.CloseWithError(0, "interrupted")
			stream.Close() // Ensure cleanup
			// Continue loop -> Accept new connection
		}
	}
}

// handleConnection encapsulates the logic for a single connection attempt
// Returns (done bool, err error). If done is true, the outer loop should exit (success).
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
) (bool, error) {

	// PAKE Authentication
	// We need to export performPAKE or move it here. Moving it to common would be best, but for now copying logic/making helper.
	// Assume PerformPAKE is available in core.
	sendMsg(ui.StatusMsg("Authenticating..."))
	if err := PerformPAKE(stream, code, 0); err != nil {
		return false, fmt.Errorf("authentication failed: %v", err)
	}
	sendMsg(ui.StatusMsg("Authenticated! Handshaking..."))

	// Calculate Code Hash
	// Re-calculating hash every time is expensive for large files.
	// Optimization: Calculate once outside logic.
	// For now, let's keep it but ideally we pass it in.
	// Actually we can't easily pass it in without refactoring outer scope heavily.
	// But since file is locked, we can cache it?
	// Let's do it simple: Calculate again. (Performance Hit on Resume, but safe)

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

	// Wait for Ack
	sendMsg(ui.StatusMsg("Handshake sent. Waiting for ACK..."))
	pType, length, err := protocol.DecodeHeader(stream)
	if err != nil || pType != protocol.TypeAck {
		return false, fmt.Errorf("handshake failed: %v", err)
	}

	// Read Offset (Resume logic)
	var offset int64 = 0
	if length == 8 {
		if err := binary.Read(stream, binary.LittleEndian, &offset); err != nil {
			return false, err
		}
		if offset > 0 {
			sendMsg(ui.StatusMsg(fmt.Sprintf("Resuming transfer from %d bytes...", offset)))
		}
	} else if length > 0 {
		// Consume unknown payload
		io.CopyN(io.Discard, stream, int64(length))
	}

	if seeker, ok := file.(io.Seeker); ok {
		if _, err := seeker.Seek(offset, 0); err != nil {
			return false, err
		}
	} else if offset > 0 {
		return false, fmt.Errorf("resume not supported for this source")
	} else {
		// offset == 0 and not seekable.
		// If we hashed it, we consumed it. If we can't seek back, we can't send it.
		// However, maybe we should error if not seekable AND we did hash?
		// RunSender ensures file is os.File (seekable) or strings.Reader (seekable).
		// So this branch might be unreachable for current inputs, but good to be safe.
		// If we are here, we probably fail read later if hashed.
	}

	// Send Data
	sendMsg(ui.StatusMsg("Sending data..."))
	buf := make([]byte, ChunkSize)
	var totalSent int64 = offset

	for {
		// Check Cancellation
		select {
		case <-ctx.Done():
			sendMsg(ui.StatusMsg("Stopping transfer (User Cancelled)..."))
			protocol.EncodeHeader(stream, protocol.TypeCancel, 0)
			return false, ctx.Err()
		default:
		}

		n, err := file.Read(buf)
		if n > 0 {
			if err := protocol.EncodeHeader(stream, protocol.TypeData, uint32(n)); err != nil {
				return false, err
			}
			if _, err := stream.Write(buf[:n]); err != nil {
				return false, err
			}
			totalSent += int64(n)

			elapsed := time.Since(startTime).Seconds()
			var speed float64
			var eta time.Duration
			if elapsed > 0 {
				speed = float64(totalSent) / elapsed
				if speed > 0 {
					eta = time.Duration(float64(fileSize-totalSent)/speed) * time.Second
				}
			}

			sendMsg(ui.ProgressMsg{
				SentBytes:  totalSent,
				TotalBytes: fileSize,
				Speed:      speed,
				ETA:        eta,
				Protocol:   "QUIC (Direct)",
			})
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, err
		}
	}

	sendMsg(ui.ProgressMsg{
		SentBytes:  fileSize,
		TotalBytes: fileSize,
		Speed:      0,
		ETA:        0,
		Protocol:   "Done",
	})
	time.Sleep(time.Second) // Give receiver time to flush
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
