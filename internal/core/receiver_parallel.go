package core

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/darkprince558/jend/internal/ui"
	"github.com/darkprince558/jend/pkg/protocol"
	"github.com/quic-go/quic-go"

	tea "github.com/charmbracelet/bubbletea"
)

type FileMeta struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Code string `json:"code"`
	Hash string `json:"hash"`
	Type string `json:"type"`
}

func downloadParallel(
	conn *quic.Conn, // Try interface 'Connection' again, check import
	controlStream io.ReadWriter,
	meta FileMeta,
	outputDir string,
	safeName string,
	sendMsg func(tea.Msg),
	password string,
) (bool, int64, string, error) {

	// 1. Setup Output File
	partialPath := filepath.Join(outputDir, safeName+".parallel.partial")
	f, err := os.OpenFile(partialPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return false, meta.Size, "", err
	}
	defer f.Close()

	if err := f.Truncate(meta.Size); err != nil {
		return false, meta.Size, "", fmt.Errorf("failed to pre-allocate file: %w", err)
	}

	// 3. Define Workers
	numWorkers := 4
	chunkSize := meta.Size / int64(numWorkers)
	var wg sync.WaitGroup
	errChan := make(chan error, numWorkers)
	progressChan := make(chan int64, 100)

	startTime := time.Now()

	for i := 0; i < numWorkers; i++ {
		start := int64(i) * chunkSize
		length := chunkSize
		if i == numWorkers-1 {
			length = meta.Size - start // Catch remainder
		}

		wg.Add(1)
		// Worker Routine
		go func(id int, start, length int64) {
			defer wg.Done()

			// Worker 0 reuses the existing control stream for the first chunk.
			// Workers 1..N open new streams, perform PAKE, and consume the handshake.

			var s io.ReadWriter
			if id == 0 {
				s = controlStream
				// We haven't sent Ack yet, so we send RangeReq below.
				// This signals the Sender to enter Parallel Mode.

				ns, err := conn.OpenStreamSync(context.Background())
				if err != nil {
					errChan <- err
					return
				}
				defer ns.Close()
				s = ns

				// Perform PAKE on new stream
				// Perform PAKE on new stream
				// Note: Use same password. Role 1 (Receiver).
				if err := PerformPAKE(s, password, 1); err != nil {
					errChan <- fmt.Errorf("worker %d pake failed: %w", i, err)
					return
				}

				// Consume Sender Handshake (sent after PAKE)
				_, length, err := protocol.DecodeHeader(s)
				if err != nil {
					errChan <- err
					return
				}
				io.CopyN(io.Discard, s, int64(length))
			}

			// Send Range Request
			// Payload: [Start int64] [Length int64]
			if err := protocol.EncodeHeader(s, protocol.TypeRangeReq, 16); err != nil {
				errChan <- err
				return
			}
			if err := binary.Write(s, binary.LittleEndian, start); err != nil {
				errChan <- err
				return
			}
			if err := binary.Write(s, binary.LittleEndian, length); err != nil {
				errChan <- err
				return
			}

			// Receive Data Loop
			buf := make([]byte, 64*1024)
			var received int64 = 0
			for {
				pType, l, err := protocol.DecodeHeader(s)
				if err != nil {
					if err == io.EOF {
						break
					}
					errChan <- err
					return
				}
				if pType == protocol.TypeData {
					// Ensure buffer is large enough
					if int(l) > len(buf) {
						buf = make([]byte, l)
					}
					if _, err := io.ReadFull(s, buf[:l]); err != nil {
						errChan <- err
						return
					}
					// WriteAt
					if _, err := f.WriteAt(buf[:l], start+received); err != nil {
						errChan <- err
						return
					}
					received += int64(l)
					progressChan <- int64(l)
				} else {
					// Error or Cancel
					break
				}
			}
		}(i, start, length)
	}

	// Progress Monitor
	monitorDone := make(chan struct{})
	go func() {
		var total int64 = 0
		for n := range progressChan {
			total += n
			// Report UI
			elapsed := time.Since(startTime).Seconds()
			speed := float64(total) / elapsed
			eta := time.Duration(float64(meta.Size-total)/speed) * time.Second
			sendMsg(ui.ProgressMsg{
				SentBytes:  total,
				TotalBytes: meta.Size,
				Speed:      speed,
				ETA:        eta,
				Protocol:   "QUIC (4x Parallel)",
			})
		}
		close(monitorDone)
	}()

	wg.Wait()
	close(progressChan)
	close(errChan)
	<-monitorDone

	// Check errors
	if len(errChan) > 0 {
		return false, meta.Size, "", <-errChan
	}

	// Rename
	finalPath := filepath.Join(outputDir, meta.Name) // Use safeName logic from caller ideally
	os.Rename(partialPath, finalPath)

	sendMsg(ui.StatusMsg("Parallel Download Complete!"))
	return true, meta.Size, meta.Hash, nil
}
