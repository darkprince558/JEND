// Package transport provides the WebRTC upload receiver for cloud QR receive mode.
//
// WebRTCUploadReceiver is the reverse of WebRTCSender: the CLI waits for a
// phone browser to connect via WebRTC DataChannel and stream files to it.
//
// Protocol (browser → CLI):
//  1. Browser sends SDP offer via MQTT topic `jend/signal/qr-{token}`
//  2. CLI responds with SDP answer + exchanges ICE candidates
//  3. Browser sends metadata: {"type":"file-meta","name":"photo.jpg","size":12345}
//  4. Browser sends binary chunks (16KB each)
//  5. Browser sends text "EOF" when done
//  6. CLI responds with "file-ok" or "file-error:..."
//  7. Repeat from step 3 for additional files
//  8. Browser sends "done" when all files are sent
package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/darkprince558/jend/internal/osutils"
	"github.com/darkprince558/jend/internal/signaling"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/pion/webrtc/v3"
)

// WebRTCUploadReceiverConfig configures the WebRTC upload receiver.
type WebRTCUploadReceiverConfig struct {
	// Token is the 6-char transfer code used for MQTT topic and URL hash.
	Token string

	// OutputDir is where received files are saved.
	OutputDir string

	// OnConnected is called when a browser peer successfully connects.
	OnConnected func()

	// OnFileStart is called when the browser begins sending a new file.
	OnFileStart func(name string, size int64)

	// OnProgress is called periodically during file transfer.
	OnProgress func(received, total int64)

	// OnFileComplete is called after each file is fully saved.
	OnFileComplete func(name string, fileCount int)

	// OnApprovalRequired asks the user whether to accept a file. If it returns false, the file is skipped.
	OnApprovalRequired func(name string, size int64) bool

	// OnError is called when an error occurs during transfer.
	OnError func(err error)
}

// WebRTCUploadReceiver accepts file uploads from a browser via WebRTC DataChannel.
// It connects to MQTT signaling, waits for a browser to send an SDP offer,
// and receives files streamed over the DataChannel.
type WebRTCUploadReceiver struct {
	signaler *signaling.IoTClient
	config   WebRTCUploadReceiverConfig

	mu        sync.Mutex
	pcs       map[string]*webrtc.PeerConnection
	fileCount int
}

// NewWebRTCUploadReceiver creates a new receiver that waits for browser uploads.
func NewWebRTCUploadReceiver(signaler *signaling.IoTClient, cfg WebRTCUploadReceiverConfig) *WebRTCUploadReceiver {
	return &WebRTCUploadReceiver{
		signaler: signaler,
		config:   cfg,
		pcs:      make(map[string]*webrtc.PeerConnection),
	}
}

// Run starts listening for browser connections and receiving files.
// It blocks until ctx is cancelled.
func (r *WebRTCUploadReceiver) Run(ctx context.Context) error {
	topic := fmt.Sprintf("jend/signal/qr-%s", r.config.Token)

	err := r.signaler.Subscribe(topic, func(_ mqtt.Client, msg mqtt.Message) {
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
			go r.handleOffer(ctx, topic, offer)

		case "ice":
			var ice iceMessage
			if err := json.Unmarshal(msg.Payload(), &ice); err != nil {
				return
			}
			r.addICECandidate(ice)
		}
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to signaling topic: %w", err)
	}

	<-ctx.Done()

	// Close all active PeerConnections.
	r.mu.Lock()
	for _, pc := range r.pcs {
		_ = pc.Close()
	}
	r.mu.Unlock()
	return nil
}

// addICECandidate routes an ICE candidate to the correct PeerConnection.
func (r *WebRTCUploadReceiver) addICECandidate(ice iceMessage) {
	r.mu.Lock()
	pc, ok := r.pcs[ice.SessionID]
	r.mu.Unlock()
	if ok && pc != nil {
		_ = pc.AddICECandidate(webrtc.ICECandidateInit{
			Candidate: ice.Candidate,
		})
	}
}

// handleOffer processes an SDP offer from a browser, creates an answer,
// and sets up the DataChannel to receive files.
func (r *WebRTCUploadReceiver) handleOffer(_ context.Context, topic string, offer sdpMessage) {
	sessionID := offer.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("sess-%d", time.Now().UnixNano())
	}

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
			{URLs: []string{"stun:stun1.l.google.com:19302"}},
		},
	})
	if err != nil {
		r.emitError(fmt.Errorf("failed to create PeerConnection: %w", err))
		return
	}

	// Register this PeerConnection.
	r.mu.Lock()
	r.pcs[sessionID] = pc
	r.mu.Unlock()

	// Clean up on disconnect.
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateConnected:
			if r.config.OnConnected != nil {
				r.config.OnConnected()
			}
		case webrtc.PeerConnectionStateFailed,
			webrtc.PeerConnectionStateClosed,
			webrtc.PeerConnectionStateDisconnected:
			r.mu.Lock()
			delete(r.pcs, sessionID)
			r.mu.Unlock()
			_ = pc.Close()
		}
	})

	// Send ICE candidates back to the browser.
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		r.sendICECandidate(topic, sessionID, c)
	})

	// Handle DataChannel from the browser — this is where files arrive.
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnOpen(func() {
			go r.receiveFiles(dc)
		})
	})

	// Set remote description and create answer.
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offer.SDP,
	}); err != nil {
		r.emitError(fmt.Errorf("failed to set remote description: %w", err))
		r.cleanupPC(sessionID, pc)
		return
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		r.emitError(fmt.Errorf("failed to create answer: %w", err))
		r.cleanupPC(sessionID, pc)
		return
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		r.emitError(fmt.Errorf("failed to set local description: %w", err))
		r.cleanupPC(sessionID, pc)
		return
	}

	// Send the answer back to the browser.
	answerMsg := sdpMessage{
		Type:      "answer",
		SDP:       answer.SDP,
		SessionID: sessionID,
	}
	payload, _ := json.Marshal(answerMsg)
	_ = r.signaler.Publish(topic, payload)
}

// receiveFiles handles the DataChannel message stream from the browser.
// It receives file metadata, binary chunks, and EOF markers.
func (r *WebRTCUploadReceiver) receiveFiles(dc *webrtc.DataChannel) {
	var (
		outFile  *os.File
		fileName string
		fileSize int64
		received int64
	)

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if msg.IsString {
			r.handleTextMessage(dc, string(msg.Data), &outFile, &fileName, &fileSize, &received)
		} else {
			r.handleBinaryChunk(msg.Data, outFile, &received, fileSize)
		}
	})
}

// uploadFileMeta is the JSON structure sent by the browser before each file.
type uploadFileMeta struct {
	Type string `json:"type"` // "file-meta"
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// handleTextMessage processes string messages from the browser DataChannel.
func (r *WebRTCUploadReceiver) handleTextMessage(
	dc *webrtc.DataChannel,
	data string,
	outFile **os.File,
	fileName *string,
	fileSize *int64,
	received *int64,
) {
	// Handle EOF — file transfer complete.
	if data == "EOF" {
		if *outFile != nil {
			_ = (*outFile).Close()
			*outFile = nil

			r.mu.Lock()
			r.fileCount++
			count := r.fileCount
			r.mu.Unlock()

			if r.config.OnFileComplete != nil {
				r.config.OnFileComplete(*fileName, count)
			}

			_ = dc.SendText("file-ok")
		}
		*received = 0
		return
	}

	// Handle "done" — all files sent, session complete.
	if data == "done" {
		return
	}

	// Handle file metadata — start receiving a new file.
	var meta uploadFileMeta
	if err := json.Unmarshal([]byte(data), &meta); err != nil {
		return
	}

	if meta.Type != "file-meta" {
		return
	}

	// Ask for user approval before accepting the file
	if r.config.OnApprovalRequired != nil {
		if !r.config.OnApprovalRequired(meta.Name, meta.Size) {
			// Immediately tell the browser we rejected it so it skips sending chunks
			_ = dc.SendText("file-error:Rejected by user")
			return
		}
	}

	// Sanitize and resolve the destination path.
	safeName := sanitizeUploadFilename(meta.Name)

	uploadMu.Lock()
	destPath := resolveUploadPath(r.config.OutputDir, safeName)
	f, err := os.Create(destPath)
	uploadMu.Unlock()

	*fileName = filepath.Base(destPath)
	*fileSize = meta.Size
	*received = 0

	if r.config.OnFileStart != nil {
		r.config.OnFileStart(*fileName, meta.Size)
	}

	if err != nil {
		r.emitError(fmt.Errorf("failed to create file %s: %w", *fileName, err))
		_ = dc.SendText("file-error:" + err.Error())
		return
	}
	*outFile = f
}

// handleBinaryChunk writes a binary chunk to the current output file.
func (r *WebRTCUploadReceiver) handleBinaryChunk(
	data []byte,
	outFile *os.File,
	received *int64,
	fileSize int64,
) {
	if outFile == nil {
		return
	}

	n, err := outFile.Write(data)
	if err != nil {
		r.emitError(fmt.Errorf("write error: %w", err))
		return
	}

	*received += int64(n)
	if r.config.OnProgress != nil {
		r.config.OnProgress(*received, fileSize)
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

var uploadMu sync.Mutex

// sendICECandidate publishes an ICE candidate to the MQTT topic.
func (r *WebRTCUploadReceiver) sendICECandidate(topic, sessionID string, c *webrtc.ICECandidate) {
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
	_ = r.signaler.Publish(topic, payload)
}

// cleanupPC removes and closes a PeerConnection.
func (r *WebRTCUploadReceiver) cleanupPC(sessionID string, pc *webrtc.PeerConnection) {
	r.mu.Lock()
	delete(r.pcs, sessionID)
	r.mu.Unlock()
	_ = pc.Close()
}

// emitError safely calls the OnError callback.
func (r *WebRTCUploadReceiver) emitError(err error) {
	if r.config.OnError != nil {
		r.config.OnError(err)
	}
}

// sanitizeUploadFilename extracts a safe base filename.
// Dangerous extensions are automatically quarantined.
func sanitizeUploadFilename(raw string) string {
	name := filepath.Base(raw)
	if name == "" || name == "." || name == "/" {
		return "upload"
	}

	if osutils.IsDangerousExtension(name) {
		name += ".jend-quarantine"
	}

	return name
}

// resolveUploadPath returns a non-conflicting path in the output directory.
func resolveUploadPath(dir, name string) string {
	dest := filepath.Join(dir, name)
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return dest
	}

	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}
