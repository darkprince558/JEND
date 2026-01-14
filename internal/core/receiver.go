package core

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
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
				os.Exit(1)
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

	sendMsg(ui.StatusMsg("Dialing localhost:" + Port + "..."))
	tr := transport.NewQUICTransport()
	conn, err := tr.Dial("localhost:" + Port) // TODO: Make address configurable
	if err != nil {
		sendMsg(ui.ErrorMsg(err))
		return
	}
	sendMsg(ui.StatusMsg("Connected! Opening stream..."))

	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		sendMsg(ui.ErrorMsg(err))
		return
	}

	// PAKE Authentication
	sendMsg(ui.StatusMsg("Authenticating..."))
	if err := PerformPAKE(stream, code, 1); err != nil {
		sendMsg(ui.ErrorMsg(fmt.Errorf("authentication failed: %v", err)))
		return
	}
	sendMsg(ui.StatusMsg("Authenticated! Waiting for handshake..."))

	// Read Handshake
	pType, length, err := protocol.DecodeHeader(stream)
	if err != nil || pType != protocol.TypeHandshake {
		sendMsg(ui.ErrorMsg(fmt.Errorf("invalid handshake")))
		return
	}

	metaBytes := make([]byte, length)
	if _, err := io.ReadFull(stream, metaBytes); err != nil {
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
		sendMsg(ui.ErrorMsg(err))
		return
	}

	// Send Ack
	protocol.EncodeHeader(stream, protocol.TypeAck, 0)

	// Sanitize Filename (prevent Zip Slip)
	safeName := filepath.Base(meta.Name)
	if safeName == "." || safeName == "/" {
		safeName = "received_file"
	}

	sendMsg(ui.StatusMsg("Receiving " + safeName))

	// Ensure output directory exists (optional, but good practice)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			sendMsg(ui.ErrorMsg(fmt.Errorf("failed to create output dir: %w", err)))
			return
		}
	}

	finalPath := filepath.Join(outputDir, "received_"+safeName)
	outFile, err := os.Create(finalPath)
	if err != nil {
		sendMsg(ui.ErrorMsg(err))
		return
	}
	defer outFile.Close()

	// Receive Loop
	buf := make([]byte, ChunkSize)
	var totalRecv int64
	startTime := time.Now()

	hasher := sha256.New()
	mw := io.MultiWriter(outFile, hasher)

	for {
		pType, length, err := protocol.DecodeHeader(stream)
		if err != nil {
			if err == io.EOF {
				break
			}
			sendMsg(ui.ErrorMsg(err))
			return
		}

		if pType == protocol.TypeData {
			// Reallocate if buf too small
			if uint32(len(buf)) < length {
				buf = make([]byte, length)
			}
			if _, err := io.ReadFull(stream, buf[:length]); err != nil {
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

	// Verify Checksum
	if meta.Hash != "" {
		recvHash := fmt.Sprintf("%x", hasher.Sum(nil))
		if recvHash == meta.Hash {
			sendMsg(ui.StatusMsg("Integrity Check: PASSED"))
		} else {
			sendMsg(ui.ErrorMsg(fmt.Errorf("Integrity Check: FAILED (Expected %s, Got %s)", meta.Hash, recvHash)))
			return
		}
	} else {
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
				sendMsg(ui.ErrorMsg(err))
				return
			}
			defer f.Close()

			gzr, err := gzip.NewReader(f)
			if err != nil {
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
						sendMsg(ui.ErrorMsg(err))
						return
					}
				} else if header.Typeflag == tar.TypeReg {
					f, err := os.Create(target)
					if err != nil {
						sendMsg(ui.ErrorMsg(err))
						return
					}
					if _, err := io.Copy(f, tr); err != nil {
						f.Close()
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
					sendMsg(ui.ErrorMsg(err))
					return
				}

				outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
				if err != nil {
					sendMsg(ui.ErrorMsg(err))
					return
				}

				rc, err := f.Open()
				if err != nil {
					outFile.Close()
					sendMsg(ui.ErrorMsg(err))
					return
				}

				_, err = io.Copy(outFile, rc)
				outFile.Close()
				rc.Close()
				if err != nil {
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
