package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/darkprince558/jend/internal/signaling"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/pion/webrtc/v3"
)

// WebRTCSender establishes a WebRTC DataChannel with a browser peer
// and streams a file through it. Signaling is performed over MQTT.
type WebRTCSender struct {
	signaler   *signaling.IoTClient
	token      string
	filePath   string
	fileName   string
	fileSize   int64
	fileHash   string
	isText     bool
	textData   string
	onProgress func(sent, total int64)
	onComplete func(downloadCount int)
	downloads  int

	mu sync.Mutex
	// pcs maps sessionID -> *webrtc.PeerConnection so multiple browsers
	// can connect simultaneously without interfering with each other.
	pcs map[string]*webrtc.PeerConnection
}

// WebRTCSenderConfig configures a new WebRTC file sender.
type WebRTCSenderConfig struct {
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

// NewWebRTCSender creates a WebRTC sender that waits for browser peers.
func NewWebRTCSender(signaler *signaling.IoTClient, cfg WebRTCSenderConfig) *WebRTCSender {
	return &WebRTCSender{
		signaler:   signaler,
		token:      cfg.Token,
		filePath:   cfg.FilePath,
		fileName:   cfg.FileName,
		fileSize:   cfg.FileSize,
		fileHash:   cfg.FileHash,
		isText:     cfg.IsText,
		textData:   cfg.TextContent,
		onProgress: cfg.OnProgress,
		onComplete: cfg.OnComplete,
		pcs:        make(map[string]*webrtc.PeerConnection),
	}
}

// sdpMessage is the JSON structure for SDP exchange over MQTT.
type sdpMessage struct {
	Type      string `json:"type"` // "offer", "answer"
	SDP       string `json:"sdp"`
	SessionID string `json:"sessionId,omitempty"`
}

// iceMessage is the JSON structure for ICE candidate exchange over MQTT.
type iceMessage struct {
	Type      string `json:"type"` // "ice", "ice-server"
	Candidate string `json:"candidate"`
	SDPMid    string `json:"sdpMid"`
	SDPMIndex uint16 `json:"sdpMLineIndex"`
	SessionID string `json:"sessionId,omitempty"`
}

// fileMetaMessage is sent to the browser first so it can display file info.
type fileMetaMessage struct {
	Type        string `json:"type"` // "meta"
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	Hash        string `json:"hash"`
	IsText      bool   `json:"isText"`
	TextPreview string `json:"textPreview,omitempty"`
}

// Run starts listening for browser connections and streaming files.
// It blocks until ctx is cancelled.
func (s *WebRTCSender) Run(ctx context.Context) error {
	topic := fmt.Sprintf("jend/signal/qr-%s", s.token)

	err := s.signaler.Subscribe(topic, func(client mqtt.Client, msg mqtt.Message) {
		var raw map[string]interface{}
		if err := json.Unmarshal(msg.Payload(), &raw); err != nil {
			return
		}
		msgType, _ := raw["type"].(string)

		switch msgType {
		case "offer":
			var offer sdpMessage
			if err := json.Unmarshal(msg.Payload(), &offer); err != nil {
				return
			}
			go s.handleOffer(ctx, topic, offer)

		case "ice":
			var ice iceMessage
			if err := json.Unmarshal(msg.Payload(), &ice); err != nil {
				return
			}
			s.mu.Lock()
			pc, ok := s.pcs[ice.SessionID]
			s.mu.Unlock()
			if ok && pc != nil {
				_ = pc.AddICECandidate(webrtc.ICECandidateInit{
					Candidate: ice.Candidate,
				})
			}
		}
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to signaling topic: %w", err)
	}

	<-ctx.Done()

	// Close all active PeerConnections
	s.mu.Lock()
	for _, pc := range s.pcs {
		_ = pc.Close()
	}
	s.mu.Unlock()
	return nil
}

func (s *WebRTCSender) handleOffer(ctx context.Context, topic string, offer sdpMessage) {
	sessionID := offer.SessionID
	if sessionID == "" {
		// Fallback: generate a pseudo-ID from SDP hash (older browsers)
		sessionID = fmt.Sprintf("sess-%d", time.Now().UnixNano())
	}

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
			{URLs: []string{"stun:stun1.l.google.com:19302"}},
		},
	}

	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		fmt.Printf("WebRTC: failed to create PeerConnection: %v\n", err)
		return
	}

	// Register this PeerConnection under its session ID
	s.mu.Lock()
	s.pcs[sessionID] = pc
	s.mu.Unlock()

	// Clean up when the peer disconnects
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateClosed ||
			state == webrtc.PeerConnectionStateDisconnected {
			s.mu.Lock()
			delete(s.pcs, sessionID)
			s.mu.Unlock()
			_ = pc.Close()
		}
	})

	// Send each ICE candidate back to THIS specific browser via session ID
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		candidateJSON := c.ToJSON()
		msg := iceMessage{
			Type:      "ice-server",
			Candidate: candidateJSON.Candidate,
			SessionID: sessionID,
		}
		if candidateJSON.SDPMid != nil {
			msg.SDPMid = *candidateJSON.SDPMid
		}
		if candidateJSON.SDPMLineIndex != nil {
			msg.SDPMIndex = *candidateJSON.SDPMLineIndex
		}
		payload, _ := json.Marshal(msg)
		_ = s.signaler.Publish(topic, payload)
	})

	// Handle DataChannel created by the browser
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnOpen(func() {
			go s.sendMetaAndWait(dc)
		})
	})

	// Set remote description (the browser's offer)
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offer.SDP,
	}); err != nil {
		fmt.Printf("WebRTC: failed to set remote description: %v\n", err)
		s.mu.Lock()
		delete(s.pcs, sessionID)
		s.mu.Unlock()
		_ = pc.Close()
		return
	}

	// Create the answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		fmt.Printf("WebRTC: failed to create answer: %v\n", err)
		_ = pc.Close()
		return
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		fmt.Printf("WebRTC: failed to set local description: %v\n", err)
		_ = pc.Close()
		return
	}

	// Send the answer back (include session ID so the right browser uses it)
	answerMsg := sdpMessage{
		Type:      "answer",
		SDP:       answer.SDP,
		SessionID: sessionID,
	}
	payload, _ := json.Marshal(answerMsg)
	_ = s.signaler.Publish(topic, payload)
}

// sendMetaAndWait sends file metadata then waits for "ready" before streaming.
func (s *WebRTCSender) sendMetaAndWait(dc *webrtc.DataChannel) {
	textPreview := ""
	if s.isText {
		preview := s.textData
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		preview = strings.ReplaceAll(preview, "<", "&lt;")
		preview = strings.ReplaceAll(preview, ">", "&gt;")
		textPreview = preview
	}

	meta := fileMetaMessage{
		Type:        "meta",
		Name:        s.fileName,
		Size:        s.fileSize,
		Hash:        s.fileHash,
		IsText:      s.isText,
		TextPreview: textPreview,
	}
	metaJSON, _ := json.Marshal(meta)
	_ = dc.SendText(string(metaJSON))

	// Wait for "ready" from the browser
	ready := make(chan struct{}, 1)
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if msg.IsString && string(msg.Data) == "ready" {
			select {
			case ready <- struct{}{}:
			default:
			}
		}
	})

	select {
	case <-ready:
		s.streamFile(dc)
	case <-time.After(5 * time.Minute):
		_ = dc.SendText("ERROR:Transfer timed out — no download initiated")
	}
}

func (s *WebRTCSender) streamFile(dc *webrtc.DataChannel) {
	const chunkSize = 16 * 1024

	if s.isText {
		data := []byte(s.textData)
		for offset := 0; offset < len(data); offset += chunkSize {
			end := offset + chunkSize
			if end > len(data) {
				end = len(data)
			}
			if err := dc.Send(data[offset:end]); err != nil {
				return
			}
			if s.onProgress != nil {
				s.onProgress(int64(end), int64(len(data)))
			}
			time.Sleep(1 * time.Millisecond)
		}
		_ = dc.SendText("EOF")
		s.mu.Lock()
		s.downloads++
		count := s.downloads
		s.mu.Unlock()
		if s.onComplete != nil {
			s.onComplete(count)
		}
		return
	}

	f, err := os.Open(s.filePath)
	if err != nil {
		_ = dc.SendText("ERROR:" + err.Error())
		return
	}
	defer f.Close()

	buf := make([]byte, chunkSize)
	var sent int64
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if sendErr := dc.Send(buf[:n]); sendErr != nil {
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
			_ = dc.SendText("ERROR:" + err.Error())
			return
		}
	}

	_ = dc.SendText("EOF")
	s.mu.Lock()
	s.downloads++
	count := s.downloads
	s.mu.Unlock()
	if s.onComplete != nil {
		s.onComplete(count)
	}
}
