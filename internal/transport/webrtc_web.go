package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

// WebRTCWebSender establishes a WebRTC DataChannel with a browser peer
// using the simple JSON WebSocket signaling server (JEND Signaler).
type WebRTCWebSender struct {
	wsURL      string
	token      string
	filePath   string
	fileName   string
	fileSize   int64
	fileHash   string
	isText     bool
	textData   string
	onProgress func(sent, total int64)
	onComplete func(downloadCount int)

	mu sync.Mutex
	pc *webrtc.PeerConnection
	ws *websocket.Conn
}

type WebRTCWebConfig struct {
	SignalerURL string
	Token       string
	FilePath    string
	FileName    string
	FileSize    int64
	FileHash    string
	IsText      bool
	TextContent string
	OnProgress  func(sent, total int64)
	OnComplete  func(downloadCount int)
}

func NewWebRTCWebSender(cfg WebRTCWebConfig) *WebRTCWebSender {
	return &WebRTCWebSender{
		wsURL:      cfg.SignalerURL,
		token:      cfg.Token,
		filePath:   cfg.FilePath,
		fileName:   cfg.FileName,
		fileSize:   cfg.FileSize,
		fileHash:   cfg.FileHash,
		isText:     cfg.IsText,
		textData:   cfg.TextContent,
		onProgress: cfg.OnProgress,
		onComplete: cfg.OnComplete,
	}
}

type wsPayload struct {
	Room    string      `json:"room"`
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

func (s *WebRTCWebSender) Run(ctx context.Context) error {
	var err error
	s.ws, _, err = websocket.DefaultDialer.Dial(s.wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to signaler: %w", err)
	}
	defer s.ws.Close()

	// 1. Join the room
	joinMsg := wsPayload{
		Room:    s.token,
		Type:    "join",
		Payload: nil,
	}
	if err := s.ws.WriteJSON(joinMsg); err != nil {
		return err
	}

	done := make(chan error, 1)

	// 2. Setup PeerConnection
	s.pc, err = webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	})
	if err != nil {
		return err
	}
	defer s.pc.Close()

	s.pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		s.ws.WriteJSON(wsPayload{
			Room:    s.token,
			Type:    "candidate",
			Payload: c.ToJSON(),
		})
	})

	s.pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			done <- fmt.Errorf("connection lost")
		}
	})

	// Web Sender is the Offerer, so it MUST create the DataChannel.
	dc, err := s.pc.CreateDataChannel("jend-transfer", &webrtc.DataChannelInit{})
	if err != nil {
		return err
	}

	dc.OnOpen(func() {
		go s.sendMetaAndWait(dc, done)
	})

	// 3. Listen for WebSocket messages (wait for peer-joined)
	go func() {
		for {
			var msg wsPayload
			if err := s.ws.ReadJSON(&msg); err != nil {
				done <- err
				return
			}

			switch msg.Type {
			case "peer-joined":
				// The receiver has arrived, we create the Offer now
				offer, err := s.pc.CreateOffer(nil)
				if err != nil {
					done <- err
					return
				}
				if err := s.pc.SetLocalDescription(offer); err != nil {
					done <- err
					return
				}
				s.ws.WriteJSON(wsPayload{
					Room:    s.token,
					Type:    "offer",
					Payload: offer,
				})

			case "answer":
				payloadBytes, _ := json.Marshal(msg.Payload)
				var answer webrtc.SessionDescription
				if err := json.Unmarshal(payloadBytes, &answer); err == nil {
					_ = s.pc.SetRemoteDescription(answer)
				}

			case "candidate":
				payloadBytes, _ := json.Marshal(msg.Payload)
				var candidate webrtc.ICECandidateInit
				if err := json.Unmarshal(payloadBytes, &candidate); err == nil {
					_ = s.pc.AddICECandidate(candidate)
				}
			}
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func (s *WebRTCWebSender) sendMetaAndWait(dc *webrtc.DataChannel, done chan error) {
	// Hack: to match the new React JS behavior, we embed 'totalChunks' in the JSON instead of just emitting raw sizes.
	const chunkSize = 16 * 1024
	totalChunks := (s.fileSize + int64(chunkSize) - 1) / int64(chunkSize)

	type reactMetaMsg struct {
		Type        string `json:"type"`
		Name        string `json:"name"`
		Size        int64  `json:"size"`
		TotalChunks int64  `json:"totalChunks"`
	}

	reactMeta := reactMetaMsg{
		Type:        "file-meta",
		Name:        s.fileName,
		Size:        s.fileSize,
		TotalChunks: totalChunks,
	}

	metaJSON, _ := json.Marshal(reactMeta)
	_ = dc.SendText(string(metaJSON))

	// In the new React Receiver, we don't send "ready", it just accepts chunks directly.
	// So we can stream the file immediately after meta.
	time.Sleep(100 * time.Millisecond)
	s.streamFile(dc, int64(chunkSize), done)
}

func (s *WebRTCWebSender) streamFile(dc *webrtc.DataChannel, chunkSize int64, done chan error) {
	if s.isText {
		// New React receiver expects text natively as JS strings with "text" type
		type textMsg struct {
			Type    string `json:"type"`
			Payload string `json:"payload"`
		}
		tm := textMsg{Type: "text", Payload: s.textData}
		b, _ := json.Marshal(tm)
		_ = dc.SendText(string(b))

		if s.onComplete != nil {
			s.onComplete(1)
		}
		
		time.Sleep(500 * time.Millisecond) // buffer timeout so react app receives it before closing
		done <- nil
		return
	}

	f, err := os.Open(s.filePath)
	if err != nil {
		done <- err
		return
	}
	defer f.Close()

	buf := make([]byte, chunkSize)
	var sent int64
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if sendErr := dc.Send(buf[:n]); sendErr != nil {
				done <- sendErr
				return
			}
			sent += int64(n)
			if s.onProgress != nil {
				s.onProgress(sent, s.fileSize)
			}
			time.Sleep(1 * time.Millisecond)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			done <- err
			return
		}
	}

	if s.onComplete != nil {
		s.onComplete(1)
	}
	time.Sleep(500 * time.Millisecond)
	done <- nil
}
