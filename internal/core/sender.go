package core

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/darkprince558/jend/internal/config"
	"github.com/darkprince558/jend/internal/transfer"
	"github.com/darkprince558/jend/internal/transport"
	"github.com/darkprince558/jend/internal/ui"
	"github.com/darkprince558/jend/pkg/protocol"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/darkprince558/jend/internal/audit"
	"github.com/darkprince558/jend/internal/discovery"
	"github.com/darkprince558/jend/internal/signaling"
	"github.com/gofrs/flock"
)

// RunSender handles the main sending logic
func RunSender(ctx context.Context, p *tea.Program, role ui.Role, filePath, textContent string, isText bool, code string, port string, timeout time.Duration, forceTar, forceZip bool, noHistory bool, turnCfg *transport.CustomTurnConfig, useS3 bool) {
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
			case ui.DetailedErrorMsg:
				fmt.Printf("Error (%v): %v\n", m.Level, m.Err)
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

	// 1. S3 Transfer Mode
	if useS3 {

		identityPoolID := config.IdentityPoolID()
		region := config.DefaultRegion

		sendMsg(ui.StatusMsg("Uploading to S3 (max 200MB)..."))

		if isText {
			tmpFile, err := os.CreateTemp("", "jend-text-*.txt")
			if err != nil {
				sendMsg(ui.DetailedErrorMsg{Err: err, Level: ui.LevelFatal})
				return
			}
			defer os.Remove(tmpFile.Name())
			if _, err := tmpFile.WriteString(textContent); err != nil {
				sendMsg(ui.DetailedErrorMsg{Err: err, Level: ui.LevelFatal})
				return
			}
			tmpFile.Close()
			filePath = tmpFile.Name()
		}

		key, err := transfer.UploadToS3(ctx, filePath, code, identityPoolID, region)
		if err != nil {
			sendMsg(ui.DetailedErrorMsg{Err: fmt.Errorf("S3 Upload Failed: %w", err), Level: ui.LevelFatal})
			return
		}

		sendMsg(ui.StatusMsg("Upload Complete! Registering code..."))

		// Register with S3 Metadata
		regClient := discovery.NewRegistryClient()

		// Parse Port
		portInt, _ := strconv.Atoi(port)

		// Metadata (marshal to PublicKey)
		meta := map[string]string{
			"type": "s3",
			"key":  key,
		}
		metaBytes, _ := json.Marshal(meta)

		err = regClient.Register(code, "", portInt, metaBytes)
		if err != nil {
			sendMsg(ui.DetailedErrorMsg{Err: fmt.Errorf("Registry Registration Failed: %w", err), Level: ui.LevelFatal})
			return
		}

		sendMsg(ui.StatusMsg("Code Registered! Waiting for receiver to download..."))

		sendMsg(ui.ProgressMsg{SentBytes: fileSize, TotalBytes: fileSize}) // Show full progress
		sendMsg(ui.StatusMsg("File is ready for pickup. Waiting for receiver..."))

		// Poll the registry every few seconds to see if the receiver picked it up
		pollTicker := time.NewTicker(5 * time.Second)
		defer pollTicker.Stop()
		pollTimeout := time.After(timeout)
		for {
			select {
			case <-ctx.Done():
				return
			case <-pollTimeout:
				sendMsg(ui.StatusMsg("S3 transfer timed out. Exiting."))
				return
			case <-pollTicker.C:
				// Check if the registry entry is still there
				_, err := regClient.Lookup(code)
				if err != nil {
					// Entry gone = receiver picked it up
					sendMsg(ui.StatusMsg("Receiver downloaded the file! Transfer complete."))
					return
				}
			}
		}
	}

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
			sendMsg(ui.DetailedErrorMsg{Err: err, Level: ui.LevelFatal})
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
				sendMsg(ui.DetailedErrorMsg{Err: err, Level: ui.LevelFatal})
				return
			}

			fileObj, err = os.Open(tempPath)
			if err != nil {
				finalErr = err
				sendMsg(ui.DetailedErrorMsg{Err: err, Level: ui.LevelFatal})
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
				sendMsg(ui.DetailedErrorMsg{Err: err, Level: ui.LevelFatal})
				return
			}

			fileObj, err = os.Open(tempPath)
			if err != nil {
				finalErr = err
				sendMsg(ui.DetailedErrorMsg{Err: err, Level: ui.LevelFatal})
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
				sendMsg(ui.DetailedErrorMsg{Err: err, Level: ui.LevelFatal})
				return
			}

			// Try to Lock (Best Effort)
			fileLock := flock.New(filePath)
			locked, err := fileLock.TryLock()
			if err != nil {
				sendMsg(ui.DetailedErrorMsg{Err: fmt.Errorf("Could not enable file lock: %v", err), Level: ui.LevelWarning})
			} else if !locked {
				sendMsg(ui.DetailedErrorMsg{Err: fmt.Errorf("File is currently in use by another process. Changes during transfer may corrupt data."), Level: ui.LevelWarning})
			} else {
				// Lock acquired
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

	// Create MultiListener to handle Direct + P2P
	multiListener := transport.NewMultiListener()
	defer multiListener.Close()

	// 1. Direct Listener
	directListener, err := tr.Listen(port)
	if err != nil {
		finalErr = err
		sendMsg(ui.DetailedErrorMsg{Err: err, Level: ui.LevelFatal})
		return
	}
	multiListener.Add(directListener)

	// Start Advertising
	portInt := 9000
	fmt.Sscanf(port, "%d", &portInt)
	stopAdvertising, err := discovery.StartAdvertising(portInt, code)
	if err != nil {
		sendMsg(ui.DetailedErrorMsg{Err: fmt.Errorf("Failed to advertise on local network: %v", err), Level: ui.LevelWarning})
	} else {
		defer stopAdvertising()
		sendMsg(ui.StatusMsg("Broadcasting on local network..."))

		regClient := discovery.NewRegistryClient()
		portInt, _ := strconv.Atoi(port)
		regClient.Register(code, "", portInt, nil)
	}

	// Start Signaling (MQTT)
	go func() {
		sendMsg(ui.StatusMsg("Connecting to Signaling Network..."))
		sigClient, err := signaling.NewIoTClient(context.Background(), "sender-"+code)
		if err != nil {
			sendMsg(ui.DetailedErrorMsg{Err: fmt.Errorf("Cloud signaling unavailable (%v). Using local network only.", err), Level: ui.LevelWarning})
			return
		}
		defer sigClient.Disconnect()

		p2p := transport.NewP2PManager(sigClient, code, turnCfg)

		pc, err := p2p.EstablishConnection(ctx, false)
		if err != nil {
			sendMsg(ui.DetailedErrorMsg{Err: fmt.Errorf("P2P Signaling failed: %v", err), Level: ui.LevelWarning})
			return
		}
		sendMsg(ui.StatusMsg("P2P (ICE) Connected! Joining listener pool..."))

		iceListener, err := tr.ListenPacket(pc)
		if err != nil {
			sendMsg(ui.DetailedErrorMsg{Err: fmt.Errorf("Failed to listen on ICE: %v", err), Level: ui.LevelWarning})
			return
		}

		multiListener.Add(iceListener)
		sendMsg(ui.StatusMsg("ICE Tunnel Active (Dual-Mode)"))
	}()

	sendMsg(ui.StatusMsg(fmt.Sprintf("Waiting for receiver (timeout: %s)...", timeout)))

	var currentOffset int64 = 0

	for {
		if time.Since(startTime) > timeout {
			finalErr = fmt.Errorf("session timed out")
			sendMsg(ui.DetailedErrorMsg{Err: finalErr, Level: ui.LevelFatal})
			return
		}

		// Check cancellation
		select {
		case <-ctx.Done():
			return
		default:
		}

		acceptCtx, cancel := context.WithTimeout(ctx, timeout-time.Since(startTime))
		conn, err := multiListener.Accept(acceptCtx)
		cancel()

		if err != nil {
			// If context canceled (timeout or manual), we exit
			if acceptCtx.Err() == context.Canceled {
				return
			}
			if acceptCtx.Err() == context.DeadlineExceeded {
				finalErr = fmt.Errorf("code has expired or connection lost")
				sendMsg(ui.DetailedErrorMsg{Err: finalErr, Level: ui.LevelFatal})
				return
			}
			finalErr = err
			sendMsg(ui.DetailedErrorMsg{Err: err, Level: ui.LevelFatal})
			return
		}

		sendMsg(ui.StatusMsg(fmt.Sprintf("Receiver connected (%s)! Opening stream...", conn.RemoteAddr())))

		var wg sync.WaitGroup
		var streamID int = 0
		var transferDone bool
		acceptCtx, cancelAccept := context.WithCancel(context.Background())
		defer cancelAccept()

		for {
			stream, err := conn.AcceptStream(acceptCtx)
			if err != nil {
				break
			}

			isFirst := (streamID == 0)
			streamID++

			wg.Add(1)
			go func(s io.ReadWriter, first bool) {
				defer wg.Done()
				defer func() {
					if c, ok := s.(io.Closer); ok {
						c.Close()
					}
				}()

				done, _ := handleConnection(ctx, s, file, isText, fileName, code, currentOffset, fileSize, startTime, startModTime, sendMsg, false)
				if done {
					cancelAccept()
					transferDone = true
				}
			}(stream, isFirst)
		}
		wg.Wait()

		if transferDone {
			sendMsg(ui.ProgressMsg{SentBytes: fileSize, TotalBytes: fileSize})
			sendMsg(ui.StatusMsg("Transfer complete!"))
			return
		}

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
		key, err := PerformPAKE(stream, code, 0)
		if err != nil {
			return false, fmt.Errorf("authentication failed: %v", err)
		}

		secureStream, err := NewSecureStream(stream, key)
		if err != nil {
			return false, fmt.Errorf("failed to create secure stream: %v", err)
		}
		stream = secureStream

		sendMsg(ui.StatusMsg("Authenticated! Connection Encrypted."))
	}

	sendMsg(ui.StatusMsg("Calculating checksum..."))
	hasher := sha256.New()

	if seeker, ok := file.(io.Seeker); ok {
		if _, err := seeker.Seek(0, 0); err != nil {
			return false, err
		}
	}

	if _, err := io.Copy(hasher, file); err != nil {
		return false, err
	}
	fileHash := fmt.Sprintf("%x", hasher.Sum(nil))

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
		if length == 8 {
			if err := binary.Read(stream, binary.LittleEndian, &offset); err != nil {
				return false, err
			}
			if offset > 0 {
				sendMsg(ui.StatusMsg(fmt.Sprintf("Resuming transfer from %d bytes...", offset)))
			}
		}
	} else if pType == protocol.TypeRangeReq {
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

	var dataReader io.Reader
	if readerAt, ok := file.(io.ReaderAt); ok {
		limit := fileSize - offset
		if byteLimit > 0 {
			limit = byteLimit
		}
		dataReader = io.NewSectionReader(readerAt, offset, limit)
	} else {
		if offset > 0 {
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

	buf := make([]byte, config.ChunkSize)
	var totalSent int64 = 0

	var bytesRemaining int64 = -1
	if byteLimit > 0 {
		bytesRemaining = byteLimit
	}

	for {
		select {
		case <-ctx.Done():
			protocol.EncodeHeader(stream, protocol.TypeCancel, 0)
			return false, ctx.Err()
		default:
		}

		// TEST HOOK: Slow down transfer for cancellation testing
		if delay := os.Getenv("JEND_TEST_DELAY"); delay != "" {
			d, _ := time.ParseDuration(delay)
			time.Sleep(d)
		}

		readSize := config.ChunkSize
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
			break
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, err
		}
	}
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

			base := filepath.Dir(filePath)
			if base == "." {
				base = ""
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
