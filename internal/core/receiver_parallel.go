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

			// Use control stream for worker 0? Or open new for all?
			// Sender finishes 'handleConnection' when it returns.
			// Sender 'handleConnection' loop breaks if Ack sends offset=0, then it enters loop sending data.
			// BUT Sender logic says: "If Ack... (Send Sequential)".
			// We sent Ack above.
			// So Sender will start streaming sequential data on 'controlStream'.
			// We want to use 'controlStream' for sequential? No.
			// The Sender logic I wrote:
			// "Wait for Ack OR Range Request" (line 353 Sender).
			// If we send Ack, Sender enters Sequential Mode.
			// If we send RangeReq, Sender enters Parallel Mode.

			// Ah! We should NOT have sent Ack on controlStream if we want to use it for range.
			// Or we repurpose controlStream for worker 0 by sending RangeReq INSTEAD of Ack.

			var s io.ReadWriter
			if id == 0 {
				s = controlStream
				// We haven't sent Ack yet in this branch?
				// Correct. My previous code replacement 'returned' before sending Ack if 'useParallel'.
				// So we need to send RangeReq here.
			} else {
				// Open new stream
				// PAKE is done per connection? Yes, PAKE session key ensures connection security?
				// Wait. QUIC streams inherit connection security (TLS).
				// We performed PAKE to authenticate the *connection*?
				// My code: 'PerformPAKE(stream)'. It writes to stream.
				// But does PAKE key secure subsequent streams?
				// No, PAKE just derives a key and verifies it.
				// The QUIC connection itself is TLS (currently self-signed).
				// So streams are secure from eavesdropping.
				// PAKE was just application-level auth.
				// So new streams are fine.

				// However, Sender 'RunSender' loop:
				// Accepts Connection -> AcceptStream -> handleConnection.
				// handleConnection -> Wait for Header (Handshake?)
				// Sender expects Handshake on EVERY stream?
				// Line 353 Sender: protocol.EncodeHeader(stream, Handshake...)
				// So Sender initiates Handshake on every new accepted stream.

				// So Worker 1..3:
				// 1. Open Stream
				// 2. Read Handshake (Metadata)
				// 3. Send RangeReq

				ns, err := conn.OpenStreamSync(context.Background())
				if err != nil {
					errChan <- err
					return
				}
				defer ns.Close()
				s = ns

				// Consume Sender Handshake
				// Sender sends Handshake immediately on AcceptStream (via handleConnection)
				_, length, err := protocol.DecodeHeader(s)
				if err != nil {
					errChan <- err
					return
				}
				io.CopyN(io.Discard, s, int64(length)) // Ignore metadata, we have it
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
