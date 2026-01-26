package signaling

// MessageType defines the type of signaling message
type MessageType string

const (
	TypeOffer     MessageType = "offer"
	TypeAnswer    MessageType = "answer"
	TypeCandidate MessageType = "candidate"
)

// SignalMessage represents a P2P signaling message exchanged via MQTT.
type SignalMessage struct {
	Type MessageType `json:"type"`
	// Session description (ICE Ufrag/Pwd)
	Ufrag string `json:"ufrag,omitempty"`
	Pwd   string `json:"pwd,omitempty"`
	// Candidates (one per message or bundled)
	Candidate string `json:"candidate,omitempty"`
}
