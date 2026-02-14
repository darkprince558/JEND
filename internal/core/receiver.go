package core

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/darkprince558/jend/internal/config"
	"github.com/darkprince558/jend/internal/transfer"
	"github.com/darkprince558/jend/internal/transport"
	"github.com/darkprince558/jend/internal/ui"
	"github.com/darkprince558/jend/pkg/protocol"
	"github.com/quic-go/quic-go"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/darkprince558/jend/internal/audit"
	"github.com/darkprince558/jend/internal/discovery"
	"github.com/darkprince558/jend/internal/signaling"
)

// RunReceiver handles the main receiving logic
func RunReceiver(p *tea.Program, code string, port string, outputDir string, autoUnzip bool, noClipboard bool, noHistory bool, concurrency int, turnCfg *transport.CustomTurnConfig, autoApprove bool) {
	sendMsg := func(msg tea.Msg) {
		if p != nil {
			p.Send(msg)
		} else {
			switch m := msg.(type) {
			case ui.DetailedErrorMsg:
				fmt.Printf("Error (%v): %v\n", m.Level, m.Err)
				if m.Level == ui.LevelFatal {
					// os.Exit(1) handled in defer
				}
			case ui.ErrorMsg:
				fmt.Println("Error:", m)
			case ui.StatusMsg:
				fmt.Println("Status:", m)
			case ui.TextReceivedMsg:
				fmt.Printf("\nReceived Text:\n%s\n", m.Content)
				if m.ClipboardOk {
					fmt.Println("Text copied to clipboard!")
				} else if noClipboard {
					fmt.Println("Clipboard copy skipped (--no-clipboard)")
				}
			case ui.ProgressMsg:
				if m.TotalBytes > 0 && m.SentBytes == m.TotalBytes {
					fmt.Println("Done!")
				}
			}
		}
	}

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

	var dialFunc func(context.Context) (*quic.Conn, error)
	var connectionDesc string

	foundIP, err := discovery.FindSender(code, config.DefaultLocalDiscoveryTimeout)
	if err == nil {
		sendMsg(ui.StatusMsg(fmt.Sprintf("Found sender at %s!", foundIP)))
		dialectAddr := foundIP
		connectionDesc = foundIP
		dialFunc = func(ctx context.Context) (*quic.Conn, error) {
			return tr.Dial(dialectAddr)
		}
	} else {
		sendMsg(ui.StatusMsg("Local discovery timed out, checking Cloud Registry..."))

		item, errCloud := discovery.LookupCloud(code)
		if errCloud == nil {
			// Check for S3 Metadata via PublicKey
			var meta map[string]string
			if len(item.PublicKey) > 0 {
				_ = json.Unmarshal(item.PublicKey, &meta)
			}

			if meta != nil && meta["type"] == "s3" {
				key := meta["key"]
				sendMsg(ui.StatusMsg("Found S3 transfer! Downloading..."))

				identityPoolID := config.IdentityPoolID()
				region := config.DefaultRegion

				downloadPath, err := transfer.DownloadFromS3(context.Background(), key, outputDir, identityPoolID, region)
				if err != nil {
					sendMsg(ui.DetailedErrorMsg{Err: fmt.Errorf("S3 Download Failed: %w", err), Level: ui.LevelFatal})
					return
				}

				sendMsg(ui.StatusMsg(fmt.Sprintf("Download Complete! Saved to %s", downloadPath)))
				return
			}

			// Standard P2P via Cloud
			cloudIP := fmt.Sprintf("%s:%d", item.IP, item.Port)
			sendMsg(ui.StatusMsg(fmt.Sprintf("Found sender via Cloud at %s!", cloudIP)))
			dialectAddr := cloudIP
			connectionDesc = cloudIP
			dialFunc = func(ctx context.Context) (*quic.Conn, error) {
				return tr.Dial(dialectAddr)
			}
		} else {
			sendMsg(ui.StatusMsg("Cloud lookup failed. Initiating P2P Signaling (ICE)..."))

			sigClient, errSig := signaling.NewIoTClient(context.Background(), "receiver-"+code)
			if errSig == nil {
				p2p := transport.NewP2PManager(sigClient, code, turnCfg)
				pc, errIce := p2p.EstablishConnection(context.Background(), true)

				sigClient.Disconnect()

				if errIce == nil {
					sendMsg(ui.StatusMsg("P2P (ICE) Connected! Switching transport..."))
					connectionDesc = "via P2P ICE"
					dialFunc = func(ctx context.Context) (*quic.Conn, error) {
						return tr.DialPacket(pc, nil)
					}
				} else {
					sendMsg(ui.DetailedErrorMsg{Err: fmt.Errorf("P2P ICE Failed: %v", errIce), Level: ui.LevelWarning})
				}
			} else {
				sendMsg(ui.DetailedErrorMsg{Err: fmt.Errorf("Signaling Auth Failed: %v. Using local network only.", errSig), Level: ui.LevelWarning})
			}
		}
	}

	// Fallback to Localhost if everything failed (Legacy/Testing)
	if dialFunc == nil {
		sendMsg(ui.StatusMsg("Fallback exhausted. Defaulting to localhost dial..."))
		connectionDesc = "localhost"
		dialFunc = func(ctx context.Context) (*quic.Conn, error) {
			return tr.Dial("localhost:" + port)
		}
	}

	retryCount := 0
	maxRetries := config.MaxConnectionRetries
	triedLocalhost := false

	for {

		sendMsg(ui.StatusMsg("Dialing " + connectionDesc + "..."))

		conn, err := dialFunc(context.Background())

		if err != nil {
			retryCount++
			if retryCount > maxRetries {
				finalErr = err
				sendMsg(ui.DetailedErrorMsg{Err: fmt.Errorf("max retries exceeded: %v", err), Level: ui.LevelFatal})
				return
			}

			if retryCount > 2 && !triedLocalhost && connectionDesc != "localhost" {
				sendMsg(ui.StatusMsg("Connection failing. Attempting fallback to localhost..."))
				dialFunc = func(ctx context.Context) (*quic.Conn, error) {
					return tr.Dial("localhost:" + port)
				}
				connectionDesc = "localhost (Fallback)"
				triedLocalhost = true
				retryCount = 0
				time.Sleep(500 * time.Millisecond)
				continue
			}

			sendMsg(ui.StatusMsg(fmt.Sprintf("Connection failed. Retrying in %d seconds...", retryCount)))
			time.Sleep(time.Duration(retryCount) * time.Second)
			continue
		}

		retryCount = 0
		sendMsg(ui.StatusMsg("Connected! Opening stream..."))

		stream, err := conn.OpenStreamSync(context.Background())
		if err != nil {
			sendMsg(ui.DetailedErrorMsg{Err: fmt.Errorf("failed to open stream: %v", err), Level: ui.LevelWarning})
			conn.CloseWithError(0, "stream open failed")
			time.Sleep(time.Second)
			continue
		}

		// Handle Session
		done, size, hash, err := handleReceiveSession(conn, stream, code, outputDir, autoUnzip, noClipboard, sendMsg, concurrency, autoApprove, p)
		fileSize = size
		fileHash = hash

		if done {
			return
		}

		if err != nil {
			if strings.Contains(err.Error(), "transfer cancelled by sender") {
				finalErr = err
				sendMsg(ui.DetailedErrorMsg{Err: err, Level: ui.LevelFatal})
				return
			}
			sendMsg(ui.DetailedErrorMsg{Err: fmt.Errorf("transfer interrupted (%v). Retrying...", err), Level: ui.LevelWarning})
			stream.Close()
			conn.CloseWithError(0, "interrupted")
			time.Sleep(time.Second)
			continue
		}
	}
}

// handleReceiveSession encapsulates the logic for a single resume attempt
func handleReceiveSession(
	conn *quic.Conn,
	stream io.ReadWriter,
	code string,
	outputDir string,
	autoUnzip bool,
	noClipboard bool,
	sendMsg func(tea.Msg),
	concurrency int,
	autoApprove bool,
	p *tea.Program,
) (bool, int64, string, error) {
	var fileSize int64
	var fileHash string

	// PAKE Authentication
	sendMsg(ui.StatusMsg("Authenticating..."))
	key, err := PerformPAKE(stream, code, 1)
	if err != nil {
		return false, 0, "", fmt.Errorf("authentication failed: %v", err)
	}

	secureStream, err := NewSecureStream(stream, key)
	if err != nil {
		return false, 0, "", fmt.Errorf("failed to create secure stream: %v", err)
	}
	stream = secureStream

	// Handshake
	sendMsg(ui.StatusMsg("Authenticated! Waiting for handshake..."))

	pType, length, err := protocol.DecodeHeader(stream)
	if err != nil || pType != protocol.TypeHandshake {
		return false, 0, "", fmt.Errorf("invalid handshake")
	}

	metaBytes := make([]byte, length)
	if _, err := io.ReadFull(stream, metaBytes); err != nil {
		return false, 0, "", err
	}

	var meta FileMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return false, 0, "", err
	}
	fileSize = meta.Size

	// ---- CONFIRMATION PROMPT ----
	if !autoApprove {
		if p != nil {
			respChan := make(chan bool)
			p.Send(ui.RequestApprovalMsg{
				Name: meta.Name,
				Size: meta.Size,
				Resp: respChan,
			})

			accepted := <-respChan
			if !accepted {
				protocol.EncodeHeader(stream, protocol.TypeCancel, 0)
				return false, 0, "", fmt.Errorf("transfer cancelled by user")
			}
		}
	}
	// -----------------------------

	if meta.Type == "text" {
		sendMsg(ui.StatusMsg("Receiving text snippet..."))

		limit := int64(config.MaxTextSize)
		if meta.Size > limit {
			return false, meta.Size, "", fmt.Errorf("text content too large (>1MB)")
		}
	}

	// Prepare Output
	safeName := filepath.Base(meta.Name)
	if safeName == "." || safeName == "/" {
		safeName = "received_file"
	}

	// Ensure output directory exists
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return false, fileSize, "", fmt.Errorf("failed to create output dir: %w", err)
		}
	}

	useParallel := meta.Size > config.ParallelThreshold && meta.Type != "text"

	if useParallel {
		sendMsg(ui.StatusMsg(fmt.Sprintf("Large file detected (%d MB). Using %d parallel streams...", meta.Size/1024/1024, concurrency)))
		return downloadParallel(conn, stream, meta, outputDir, safeName, sendMsg, code, concurrency) // Call specialized function
	}

	// Sequential Download
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

	var outFile io.WriteCloser
	var textBuf *bytes.Buffer

	if meta.Type == "text" {
		textBuf = new(bytes.Buffer)
		outFile = &nopCloser{textBuf}
	} else {
		var f *os.File
		if offset > 0 {
			f, err = os.OpenFile(partialPath, os.O_WRONLY|os.O_APPEND, 0644)
		} else {
			f, err = os.Create(partialPath)
		}
		if err != nil {
			return false, fileSize, "", err
		}
		outFile = f
	}
	defer outFile.Close()

	buf := make([]byte, config.ChunkSize)
	var totalRecv int64 = offset
	startTime := time.Now()

	hasher := sha256.New()

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
			// If we received all data but connection dropped, treat as success
			if totalRecv == meta.Size {
				break
			}
			return false, fileSize, "", err
		}

		if pType == protocol.TypeCancel {
			return false, fileSize, "", fmt.Errorf("transfer cancelled by sender")
		}

		if pType == protocol.TypeData {
			if uint32(len(buf)) < length {
				buf = make([]byte, length)
			}
			if _, err := io.ReadFull(stream, buf[:length]); err != nil {
				return false, fileSize, "", err
			}
			mw.Write(buf[:length])
			totalRecv += int64(length)

			// Telemetry
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

	if c, ok := stream.(io.Closer); ok {
		c.Close()
	}
	// Don't send 100% progress yet — text/file handling below needs to
	// run before the TUI quits (ProgressMsg with ratio>=1.0 triggers tea.Quit)

	outFile.Close()

	finalPath := filepath.Join(outputDir, safeName)
	if meta.Hash != "" {
		recvHash := fmt.Sprintf("%x", hasher.Sum(nil))
		if recvHash == meta.Hash {
			sendMsg(ui.StatusMsg("Integrity Check: PASSED"))

			if meta.Type == "text" {
				content := textBuf.String()
				clipOk := false
				if !noClipboard {
					if err := clipboard.WriteAll(content); err == nil {
						clipOk = true
					}
				}
				sendMsg(ui.TextReceivedMsg{Content: content, ClipboardOk: clipOk})
				sendMsg(ui.ProgressMsg{SentBytes: meta.Size, TotalBytes: meta.Size})
				return true, fileSize, meta.Hash, nil
			}

			counter := 0
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
			fileHash = meta.Hash
			sendMsg(ui.StatusMsg("Saved to: " + filepath.Base(finalPath)))
			sendMsg(ui.ProgressMsg{SentBytes: meta.Size, TotalBytes: meta.Size})

		} else {
			return false, fileSize, "", fmt.Errorf("Integrity Check: FAILED (Expected %s, Got %s).", meta.Hash, recvHash)
		}
	} else {
		if meta.Type == "text" {
			content := textBuf.String()
			clipOk := false
			if !noClipboard {
				if err := clipboard.WriteAll(content); err == nil {
					clipOk = true
				}
			}
			sendMsg(ui.TextReceivedMsg{Content: content, ClipboardOk: clipOk})
			sendMsg(ui.ProgressMsg{SentBytes: meta.Size, TotalBytes: meta.Size})
			return true, fileSize, "", nil
		}

		// No hash provided, move file without verification
		os.Rename(partialPath, finalPath)
		sendMsg(ui.StatusMsg("Integrity Check: SKIPPED (No hash provided)"))
		sendMsg(ui.ProgressMsg{SentBytes: meta.Size, TotalBytes: meta.Size})
	}

	time.Sleep(time.Second)

	// Auto-Unzip Logic
	if autoUnzip {
		ext := filepath.Ext(safeName)
		if strings.HasSuffix(safeName, ".tar.gz") {
			sendMsg(ui.StatusMsg("Unzipping .tar.gz archive..."))
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

			zr, err := zip.OpenReader(finalPath)
			if err != nil {
				return true, fileSize, fileHash, err
			}
			defer zr.Close()

			for _, f := range zr.File {
				fpath := filepath.Join(outputDir, f.Name)

				// Zip Slip protection
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

type nopCloser struct {
	io.Writer
}

func (n *nopCloser) Close() error {
	return nil
}
