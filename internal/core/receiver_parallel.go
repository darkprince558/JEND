package core

import (
	"context"
	"encoding/binary"
	"encoding/json"
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
	conn *quic.Conn,
	controlStream io.ReadWriter,
	meta FileMeta,
	outputDir string,
	safeName string,
	sendMsg func(tea.Msg),
	password string,
	concurrency int,
) (bool, int64, string, error) {

	// Setup Output File and Meta File
	finalPath := filepath.Join(outputDir, safeName)
	parallelPath := filepath.Join(outputDir, safeName+".parallel.part")
	metaPath := filepath.Join(outputDir, safeName+".parallel.meta")

	state, err := loadOrInitState(metaPath, meta.Size, concurrency)
	if err != nil {
		return false, meta.Size, "", fmt.Errorf("metadata error: %w", err)
	}

	if len(state.Chunks) != concurrency && len(state.Chunks) > 0 {
		sendMsg(ui.StatusMsg(fmt.Sprintf("Resuming with saved concurrency: %d (ignoring requested %d)", len(state.Chunks), concurrency)))
		concurrency = len(state.Chunks)
	}

	f, err := os.OpenFile(parallelPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return false, meta.Size, "", err
	}
	defer func() { _ = f.Close() }()

	if err := f.Truncate(meta.Size); err != nil {
		return false, meta.Size, "", fmt.Errorf("failed to pre-allocate file: %w", err)
	}

	var completedBytes int64 = 0
	for _, c := range state.Chunks {
		if c.Done {
			completedBytes += c.Length
		}
	}

	if completedBytes > 0 {
		sendMsg(ui.StatusMsg(fmt.Sprintf("Resuming parallel download... (%d%% done)", (completedBytes*100)/meta.Size)))
	}

	// Launch Workers
	var wg sync.WaitGroup
	errChan := make(chan error, concurrency)
	progressChan := make(chan int64, 100)

	startTime := time.Now()

	activeWorkers := 0
	for i, chunk := range state.Chunks {
		if chunk.Done {
			continue
		}
		activeWorkers++
		wg.Add(1)

		go func(id int, start, length int64) {
			defer wg.Done()

			// Each worker opens a new stream for robustness on resume.
			ns, err := conn.OpenStreamSync(context.Background())
			if err != nil {
				errChan <- err
				return
			}
			defer func() { _ = ns.Close() }()

			key, err := PerformPAKE(ns, password, 1)
			if err != nil {
				errChan <- fmt.Errorf("worker %d pake failed: %w", id, err)
				return
			}

			secureStream, err := NewSecureStream(ns, key)
			if err != nil {
				errChan <- fmt.Errorf("worker %d failed to upgrade stream: %w", id, err)
				return
			}

			// Consume sender handshake
			_, l, err := protocol.DecodeHeader(secureStream)
			if err != nil {
				errChan <- err
				return
			}
			_, _ = io.CopyN(io.Discard, secureStream, int64(l))

			if err := protocol.EncodeHeader(secureStream, protocol.TypeRangeReq, 16); err != nil {
				errChan <- err
				return
			}
			if err := binary.Write(secureStream, binary.LittleEndian, start); err != nil {
				errChan <- err
				return
			}
			if err := binary.Write(secureStream, binary.LittleEndian, length); err != nil {
				errChan <- err
				return
			}

			buf := make([]byte, 64*1024)
			var receivedLocal int64 = 0
			for {
				pType, l, err := protocol.DecodeHeader(secureStream)
				if err != nil {
					if err == io.EOF {
						break
					}
					errChan <- err
					return
				}
				if pType == protocol.TypeData {
					if int(l) > len(buf) {
						buf = make([]byte, l)
					}
					if _, err := io.ReadFull(secureStream, buf[:l]); err != nil {
						errChan <- err
						return
					}
					if _, err := f.WriteAt(buf[:l], start+receivedLocal); err != nil {
						errChan <- err
						return
					}
					receivedLocal += int64(l)
					progressChan <- int64(l)
				} else {
					break
				}
			}

			if receivedLocal == length {
				markChunkDone(metaPath, id)
			}
		}(i, chunk.Start, chunk.Length)
	}

	if activeWorkers == 0 {
		sendMsg(ui.StatusMsg("All chunks already downloaded."))
	}

	// Progress Monitor
	monitorDone := make(chan struct{})
	go func() {
		var total = completedBytes
		for n := range progressChan {
			total += n
			elapsed := time.Since(startTime).Seconds()
			speed := 0.0
			eta := time.Duration(0)
			if elapsed > 0 {
				bytesSinceStart := total - completedBytes
				speed = float64(bytesSinceStart) / elapsed
				if speed > 0 {
					eta = time.Duration(float64(meta.Size-total)/speed) * time.Second
				}
			}
			sendMsg(ui.ProgressMsg{
				SentBytes:  total,
				TotalBytes: meta.Size,
				Speed:      speed,
				ETA:        eta,
				Protocol:   fmt.Sprintf("QUIC (%dx Parallel)", concurrency),
			})
		}
		close(monitorDone)
	}()

	wg.Wait()
	close(progressChan)
	close(errChan)
	<-monitorDone

	if len(errChan) > 0 {
		return false, meta.Size, "", <-errChan
	}

	_ = os.Rename(parallelPath, finalPath)
	_ = os.Remove(metaPath)

	sendMsg(ui.StatusMsg("Parallel Download Complete!"))
	return true, meta.Size, meta.Hash, nil
}

// State Management
type DownloadState struct {
	TotalSize int64   `json:"total_size"`
	Chunks    []Chunk `json:"chunks"`
}

type Chunk struct {
	ID     int   `json:"id"`
	Start  int64 `json:"start"`
	Length int64 `json:"length"`
	Done   bool  `json:"done"`
}

func loadOrInitState(metaPath string, totalSize int64, chunks int) (*DownloadState, error) {
	data, err := os.ReadFile(metaPath)
	if err == nil {
		var state DownloadState
		if err := json.Unmarshal(data, &state); err == nil {
			if state.TotalSize == totalSize {
				return &state, nil
			}
		}
	}

	state := &DownloadState{
		TotalSize: totalSize,
		Chunks:    make([]Chunk, chunks),
	}

	chunkSize := totalSize / int64(chunks)
	for i := 0; i < chunks; i++ {
		start := int64(i) * chunkSize
		length := chunkSize
		if i == chunks-1 {
			length = totalSize - start
		}
		state.Chunks[i] = Chunk{
			ID:     i,
			Start:  start,
			Length: length,
			Done:   false,
		}
	}

	saveState(metaPath, state)
	return state, nil
}

func saveState(path string, state *DownloadState) {
	data, _ := json.Marshal(state)
	_ = os.WriteFile(path, data, 0644)
}

func markChunkDone(path string, id int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var state DownloadState
	_ = json.Unmarshal(data, &state)
	if id < len(state.Chunks) {
		state.Chunks[id].Done = true
		saveState(path, &state)
	}
}
