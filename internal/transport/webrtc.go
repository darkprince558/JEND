package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
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
	pc         *webrtc.PeerConnection
	filePath   string
	fileSize   int64
	isText     bool
	textData   string
	onProgress func(sent, total int64)
	onComplete func(downloadCount int)
	downloads  int
}

// WebRTCSenderConfig configures a new WebRTC file sender.
type WebRTCSenderConfig struct {
	Token       string
	FilePath    string
	FileName    string
	FileSize    int64
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
		fileSize:   cfg.FileSize,
		isText:     cfg.IsText,
		textData:   cfg.TextContent,
		onProgress: cfg.OnProgress,
		onComplete: cfg.OnComplete,
	}
}

// sdpMessage is the JSON structure for SDP exchange over MQTT.
type sdpMessage struct {
	Type string `json:"type"` // "offer", "answer"
	SDP  string `json:"sdp"`
}

// iceMessage is the JSON structure for ICE candidate exchange over MQTT.
type iceMessage struct {
	Type      string `json:"type"` // "ice"
	Candidate string `json:"candidate"`
	SDPMid    string `json:"sdpMid"`
	SDPMIndex uint16 `json:"sdpMLineIndex"`
}

// Run starts listening for browser connections and streaming files.
// It blocks until ctx is cancelled.
func (s *WebRTCSender) Run(ctx context.Context) error {
	topic := fmt.Sprintf("jend/qr/%s", s.token)

	// Subscribe to signaling topic for browser SDP offers
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
			if s.pc != nil {
				candidate := webrtc.ICECandidateInit{
					Candidate: ice.Candidate,
				}
				_ = s.pc.AddICECandidate(candidate)
			}
		}
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to signaling topic: %w", err)
	}

	// Wait for context cancellation
	<-ctx.Done()
	if s.pc != nil {
		_ = s.pc.Close()
	}
	return nil
}

func (s *WebRTCSender) handleOffer(ctx context.Context, topic string, offer sdpMessage) {
	// Create PeerConnection with STUN/TURN
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
	s.pc = pc

	// Send ICE candidates to the browser
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		candidateJSON := c.ToJSON()
		msg := iceMessage{
			Type:      "ice-server",
			Candidate: candidateJSON.Candidate,
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

	// Handle DataChannel opened by the browser
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnOpen(func() {
			go s.streamFile(dc)
		})
	})

	// Set the remote SDP offer
	err = pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offer.SDP,
	})
	if err != nil {
		fmt.Printf("WebRTC: failed to set remote description: %v\n", err)
		_ = pc.Close()
		return
	}

	// Create and send the answer
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

	// Send the answer back to the browser
	answerMsg := sdpMessage{
		Type: "answer",
		SDP:  answer.SDP,
	}
	payload, _ := json.Marshal(answerMsg)
	_ = s.signaler.Publish(topic, payload)
}

func (s *WebRTCSender) streamFile(dc *webrtc.DataChannel) {
	defer dc.Close()

	const chunkSize = 16 * 1024 // 16KB chunks

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
			time.Sleep(1 * time.Millisecond) // Flow control
		}
		_ = dc.SendText("EOF")
		s.downloads++
		if s.onComplete != nil {
			s.onComplete(s.downloads)
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
			time.Sleep(1 * time.Millisecond) // Flow control
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
	s.downloads++
	if s.onComplete != nil {
		s.onComplete(s.downloads)
	}
}
