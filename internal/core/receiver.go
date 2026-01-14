package core

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
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

	"github.com/darkprince558/jend/internal/transport"
	"github.com/darkprince558/jend/internal/ui"
	"github.com/darkprince558/jend/pkg/protocol"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/darkprince558/jend/internal/audit"
)

// RunReceiver handles the main receiving logic
func RunReceiver(p *tea.Program, code string, outputDir string, autoUnzip bool) {
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

		if p == nil && exitCode != 0 {
			os.Exit(exitCode)
		}
	}()

	sendMsg(ui.StatusMsg("Dialing localhost:" + Port + "..."))
	tr := transport.NewQUICTransport()
	conn, err := tr.Dial("localhost:" + Port) // TODO: Make address configurable
	if err != nil {
		finalErr = err
		sendMsg(ui.ErrorMsg(err))
		return
	}
	sendMsg(ui.StatusMsg("Connected! Opening stream..."))

	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		finalErr = err
		sendMsg(ui.ErrorMsg(err))
		return
	}

	// PAKE Authentication
	sendMsg(ui.StatusMsg("Authenticating..."))
	if err := PerformPAKE(stream, code, 1); err != nil {
		finalErr = err
		sendMsg(ui.ErrorMsg(fmt.Errorf("authentication failed: %v", err)))
		return
	}
	sendMsg(ui.StatusMsg("Authenticated! Waiting for handshake..."))

	// Read Handshake
	pType, length, err := protocol.DecodeHeader(stream)
	if err != nil || pType != protocol.TypeHandshake {
		finalErr = fmt.Errorf("invalid handshake")
		sendMsg(ui.ErrorMsg(finalErr))
		return
	}

	metaBytes := make([]byte, length)
	if _, err := io.ReadFull(stream, metaBytes); err != nil {
		finalErr = err
		sendMsg(ui.ErrorMsg(err))
		return
	}

	var meta struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		Code string `json:"code"`
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		finalErr = err
		sendMsg(ui.ErrorMsg(err))
		return
	}
	fileSize = meta.Size
	// Update audit log filename if we have it now
	// Ideally we could update the struct in the defer, referencing variables.
	// Since we use closure variables, setting fileName var (if we had one) would work.
	// But we initialized log entry in defer. I'll add a fileName var in the scope.

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

	if info, err := os.Stat(partialPath); err == nil {
		if info.Size() < meta.Size && info.Size() > 0 {
			offset = info.Size()
			sendMsg(ui.StatusMsg(fmt.Sprintf("Partial download found. Resuming from %d bytes...", offset)))
		}
	}

	if err := protocol.EncodeHeader(stream, protocol.TypeAck, 8); err != nil {
		finalErr = err
		sendMsg(ui.ErrorMsg(err))
		return
	}
	if err := binary.Write(stream, binary.LittleEndian, offset); err != nil {
		finalErr = err
		sendMsg(ui.ErrorMsg(err))
		return
	}

	sendMsg(ui.StatusMsg("Receiving " + safeName))

	// Ensure output directory exists (optional, but good practice)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			finalErr = err
			sendMsg(ui.ErrorMsg(fmt.Errorf("failed to create output dir: %w", err)))
			return
		}
	}

	var outFile *os.File
	if offset > 0 {
		// Resume: Open in Append mode
		outFile, err = os.OpenFile(partialPath, os.O_WRONLY|os.O_APPEND, 0644)
	} else {
		// New: Create/Truncate
		outFile, err = os.Create(partialPath)
	}

	if err != nil {
		finalErr = err
		sendMsg(ui.ErrorMsg(err))
		return
	}
	defer outFile.Close()

	// Receive Loop
	buf := make([]byte, ChunkSize)
	var totalRecv int64 = offset
	startTime = time.Now()

	hasher := sha256.New()

	// If resuming, we must hash the existing part first so the final hash matches the full file
	if offset > 0 {
		existingFile, err := os.Open(partialPath)
		if err != nil {
			finalErr = err
			sendMsg(ui.ErrorMsg(err))
			return
		}
		if _, err := io.CopyN(hasher, existingFile, offset); err != nil {
			finalErr = err
			existingFile.Close()
			sendMsg(ui.ErrorMsg(err))
			return
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
			finalErr = err
			sendMsg(ui.ErrorMsg(err))
			return
		}

		if pType == protocol.TypeData {
			// Reallocate if buf too small
			if uint32(len(buf)) < length {
				buf = make([]byte, length)
			}
			if _, err := io.ReadFull(stream, buf[:length]); err != nil {
				finalErr = err
				sendMsg(ui.ErrorMsg(err))
				return
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

	stream.Close()
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
				finalErr = err
				sendMsg(ui.ErrorMsg(fmt.Errorf("failed to save final file: %v", err)))
				return
			}
			fileHash = meta.Hash // Set hash for audit log only on success
			sendMsg(ui.StatusMsg("Saved to: " + filepath.Base(finalPath)))

		} else {
			finalErr = fmt.Errorf("integrity check failed")
			sendMsg(ui.ErrorMsg(fmt.Errorf("Integrity Check: FAILED (Expected %s, Got %s). Keeping .partial file.", meta.Hash, recvHash)))
			return
		}
	} else {
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
				finalErr = err
				sendMsg(ui.ErrorMsg(err))
				return
			}
			defer f.Close()

			gzr, err := gzip.NewReader(f)
			if err != nil {
				finalErr = err
				sendMsg(ui.ErrorMsg(err))
				return
			}
			defer gzr.Close()

			tr := tar.NewReader(gzr)

			for {
				header, err := tr.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					finalErr = err
					sendMsg(ui.ErrorMsg(err))
					return
				}

				// Zip Slip Protection
				target := filepath.Join(outputDir, header.Name)
				if !strings.HasPrefix(target, filepath.Clean(outputDir)+string(os.PathSeparator)) {
					// log.Println("zip slip attempt detected")
					continue
				}

				if header.Typeflag == tar.TypeDir {
					if err := os.MkdirAll(target, 0755); err != nil {
						finalErr = err
						sendMsg(ui.ErrorMsg(err))
						return
					}
				} else if header.Typeflag == tar.TypeReg {
					f, err := os.Create(target)
					if err != nil {
						finalErr = err
						sendMsg(ui.ErrorMsg(err))
						return
					}
					if _, err := io.Copy(f, tr); err != nil {
						f.Close()
						finalErr = err
						sendMsg(ui.ErrorMsg(err))
						return
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
				finalErr = err
				sendMsg(ui.ErrorMsg(err))
				return
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
					finalErr = err
					sendMsg(ui.ErrorMsg(err))
					return
				}

				outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
				if err != nil {
					finalErr = err
					sendMsg(ui.ErrorMsg(err))
					return
				}

				rc, err := f.Open()
				if err != nil {
					outFile.Close()
					finalErr = err
					sendMsg(ui.ErrorMsg(err))
					return
				}

				_, err = io.Copy(outFile, rc)
				outFile.Close()
				rc.Close()
				if err != nil {
					finalErr = err
					sendMsg(ui.ErrorMsg(err))
					return
				}
			}
			sendMsg(ui.StatusMsg("Extracted successfully!"))
		}
	}
}

func PerformPAKE(stream io.ReadWriter, password string, role int) error {
	// Simple Challenge-Response Auth
	// Role 0 = Sender (Verifier), Role 1 = Receiver (Prover/Client)

	// Step 0: Sync Stream (Client speaks first to trigger AcceptStream on Server)
	if role == 1 { // Receiver
		if err := protocol.EncodeHeader(stream, protocol.TypePAKE, 0); err != nil {
			return err
		}
		// Write empty PAKE packet as "Hello"
		// Header (5 bytes) is enough to trigger stream
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

	if role == 0 { // Sender: Verify the Receiver knows the code
		// 1. Generate Challenge
		challenge := make([]byte, 32)
		if _, err := rand.Read(challenge); err != nil {
			return err
		}

		// 2. Send Challenge
		if err := protocol.EncodeHeader(stream, protocol.TypePAKE, 32); err != nil {
			return err
		}
		if _, err := stream.Write(challenge); err != nil {
			return err
		}

		// 3. Read Response
		pType, length, err := protocol.DecodeHeader(stream)
		if err != nil {
			return err
		}
		if pType != protocol.TypePAKE {
			return fmt.Errorf("expected auth response")
		}
		resp := make([]byte, length)
		if _, err := io.ReadFull(stream, resp); err != nil {
			return err
		}

		// 4. Verify
		// Expected = SHA256(Challenge + Password)
		blob := append(challenge, []byte(password)...)
		expected := sha256.Sum256(blob)

		if subtle.ConstantTimeCompare(resp, expected[:]) != 1 {
			return fmt.Errorf("invalid code")
		}
		return nil

	} else { // Receiver: Prove we know the code
		// 1. Read Challenge
		pType, length, err := protocol.DecodeHeader(stream)
		if err != nil {
			return err
		}
		if pType != protocol.TypePAKE {
			return fmt.Errorf("expected auth challenge")
		}
		challenge := make([]byte, length)
		if _, err := io.ReadFull(stream, challenge); err != nil {
			return err
		}

		// 2. Compute Response
		blob := append(challenge, []byte(password)...)
		hash := sha256.Sum256(blob)

		// 3. Send Response
		if err := protocol.EncodeHeader(stream, protocol.TypePAKE, uint32(len(hash))); err != nil {
			return err
		}
		if _, err := stream.Write(hash[:]); err != nil {
			return err
		}

		return nil
	}
}
