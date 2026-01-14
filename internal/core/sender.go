package core

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/darkprince558/jend/internal/transport"
	"github.com/darkprince558/jend/internal/ui"
	"github.com/darkprince558/jend/pkg/protocol"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	Port      = "9000"
	ChunkSize = 1024 * 64
)

// RunSender handles the main sending logic
func RunSender(p *tea.Program, role ui.Role, filePath, code string, timeout time.Duration, forceTar, forceZip bool) {
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

	// Check if path is a directory
	info, err := os.Stat(filePath)
	if err != nil {
		sendMsg(ui.ErrorMsg(err))
		return
	}

	var file *os.File
	var fileName string
	var cleanup func()

	// Compression Logic
	if info.IsDir() || forceTar {
		sendMsg(ui.StatusMsg("Compressing to .tar.gz..."))
		tempPath, err := CompressPath(filePath, "tar.gz")
		if err != nil {
			sendMsg(ui.ErrorMsg(err))
			return
		}

		file, err = os.Open(tempPath)
		if err != nil {
			sendMsg(ui.ErrorMsg(err))
			return
		}
		fileName = filepath.Base(filePath) + ".tar.gz"
		cleanup = func() {
			file.Close()
			os.Remove(tempPath)
		}
		info, _ = file.Stat()
	} else if forceZip {
		sendMsg(ui.StatusMsg("Compressing to .zip..."))
		tempPath, err := CompressPath(filePath, "zip")
		if err != nil {
			sendMsg(ui.ErrorMsg(err))
			return
		}

		file, err = os.Open(tempPath)
		if err != nil {
			sendMsg(ui.ErrorMsg(err))
			return
		}
		fileName = filepath.Base(filePath) + ".zip"
		cleanup = func() {
			file.Close()
			os.Remove(tempPath)
		}
		info, _ = file.Stat()
	} else {
		// Normal File
		file, err = os.Open(filePath)
		if err != nil {
			sendMsg(ui.ErrorMsg(err))
			return
		}
		fileName = info.Name()
		cleanup = func() {
			file.Close()
		}
	}
	defer cleanup()

	// Start Listener
	// Note: We need to pass the transport or create it. For now creating new.
	tr := transport.NewQUICTransport()
	listener, err := tr.Listen(Port)
	if err != nil {
		sendMsg(ui.ErrorMsg(err))
		return
	}

	// Wait for connection
	sendMsg(ui.StatusMsg(fmt.Sprintf("Waiting for receiver (timeout: %s)...", timeout)))

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := listener.Accept(ctx)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			sendMsg(ui.ErrorMsg(fmt.Errorf("code has expired after %v of inactivity, please try again", timeout)))
		} else {
			sendMsg(ui.ErrorMsg(err))
		}
		return
	}
	sendMsg(ui.StatusMsg("Receiver connected! Opening stream..."))

	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		sendMsg(ui.ErrorMsg(err))
		return
	}

	// PAKE Authentication
	// We need to export performPAKE or move it here. Moving it to common would be best, but for now copying logic/making helper.
	// Assume PerformPAKE is available in core.
	sendMsg(ui.StatusMsg("Authenticating..."))
	if err := PerformPAKE(stream, code, 0); err != nil {
		sendMsg(ui.ErrorMsg(fmt.Errorf("authentication failed: %v", err)))
		return
	}
	sendMsg(ui.StatusMsg("Authenticated! Handshaking..."))

	// Calculate Code Hash
	sendMsg(ui.StatusMsg("Calculating checksum..."))
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		sendMsg(ui.ErrorMsg(err))
		return
	}
	fileHash := fmt.Sprintf("%x", hasher.Sum(nil))
	if _, err := file.Seek(0, 0); err != nil { // Reset file pointer
		sendMsg(ui.ErrorMsg(err))
		return
	}

	// Handshake
	meta := map[string]interface{}{
		"name": fileName,
		"size": info.Size(),
		"code": code,
		"hash": fileHash,
	}
	metaBytes, _ := json.Marshal(meta)
	if err := protocol.EncodeHeader(stream, protocol.TypeHandshake, uint32(len(metaBytes))); err != nil {
		sendMsg(ui.ErrorMsg(err))
		return
	}
	stream.Write(metaBytes)

	// Wait for Ack
	sendMsg(ui.StatusMsg("Handshake sent. Waiting for ACK..."))
	pType, _, err := protocol.DecodeHeader(stream)
	if err != nil || pType != protocol.TypeAck {
		sendMsg(ui.ErrorMsg(fmt.Errorf("handshake failed")))
		return
	}

	// Send Data
	sendMsg(ui.StatusMsg("Sending data..."))
	buf := make([]byte, ChunkSize)
	var totalSent int64

	startTime := time.Now()
	for {
		n, err := file.Read(buf)
		if n > 0 {
			if err := protocol.EncodeHeader(stream, protocol.TypeData, uint32(n)); err != nil {
				sendMsg(ui.ErrorMsg(err))
				return
			}
			if _, err := stream.Write(buf[:n]); err != nil {
				sendMsg(ui.ErrorMsg(err))
				return
			}
			totalSent += int64(n)

			elapsed := time.Since(startTime).Seconds()
			var speed float64
			var eta time.Duration
			if elapsed > 0 {
				speed = float64(totalSent) / elapsed
				if speed > 0 {
					eta = time.Duration(float64(info.Size()-totalSent)/speed) * time.Second
				}
			}

			sendMsg(ui.ProgressMsg{
				SentBytes:  totalSent,
				TotalBytes: info.Size(),
				Speed:      speed,
				ETA:        eta,
				Protocol:   "QUIC (Direct)",
			})
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			sendMsg(ui.ErrorMsg(err))
			return
		}
	}

	stream.Close()
	sendMsg(ui.ProgressMsg{
		SentBytes:  info.Size(),
		TotalBytes: info.Size(),
		Speed:      0,
		ETA:        0,
		Protocol:   "Done",
	})
	time.Sleep(time.Second)
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
