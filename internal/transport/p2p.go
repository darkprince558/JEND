package transport

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/darkprince558/jend/internal/signaling"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/pion/ice/v2"
)

// P2PManager handles the establishment of a P2P connection via ICE & MQTT
type P2PManager struct {
	Signaling *signaling.IoTClient
	Code      string
	Agent     *ice.Agent
}

// NewP2PManager creates a manager for a specific transfer session
func NewP2PManager(sig *signaling.IoTClient, code string) *P2PManager {
	return &P2PManager{
		Signaling: sig,
		Code:      code,
	}
}

// EstablishConnection performs the ICE handshake.
// isOfferer: true (Receiver), false (Sender)
func (m *P2PManager) EstablishConnection(ctx context.Context, isOfferer bool) (*ice.Agent, error) {
	// 1. Create ICE Agent
	agent, err := NewICEAgent(ctx, isOfferer) // Defined in ice.go
	if err != nil {
		return nil, err
	}
	m.Agent = agent

	// 2. Setup Signaling Topic
	topic := fmt.Sprintf("jend/signal/%s", m.Code)

	// Channels for signaling flow
	remoteCandidates := make(chan string, 10)
	remoteUfrag := make(chan string, 1)
	remotePwd := make(chan string, 1)

	// 3. Subscribe to Signaling
	err = m.Signaling.Subscribe(topic, func(client mqtt.Client, msg mqtt.Message) {
		var sigMsg signaling.SignalMessage
		if err := json.Unmarshal(msg.Payload(), &sigMsg); err != nil {
			fmt.Printf("Invalid signal msg: %v\n", err)
			return
		}

		// Filter own messages (simple logic: check type vs role)
		if isOfferer && sigMsg.Type == signaling.TypeOffer {
			return
		}
		if !isOfferer && sigMsg.Type == signaling.TypeAnswer {
			return
		}

		if sigMsg.Candidate != "" {
			remoteCandidates <- sigMsg.Candidate
		}
		if sigMsg.Ufrag != "" {
			select {
			case remoteUfrag <- sigMsg.Ufrag:
			default:
			}
		}
		if sigMsg.Pwd != "" {
			select {
			case remotePwd <- sigMsg.Pwd:
			default:
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("mqtt subscribe failed: %w", err)
	}

	// 4. OnCandidate: Send to peer
	agent.OnCandidate(func(c ice.Candidate) {
		if c == nil {
			return
		}
		msg := signaling.SignalMessage{
			Type:      signaling.TypeCandidate,
			Candidate: c.Marshal(),
		}
		if isOfferer {
			msg.Type = signaling.TypeOffer
		} else {
			msg.Type = signaling.TypeAnswer
		}

		payload, _ := json.Marshal(msg)
		m.Signaling.Publish(topic, payload)
	})

	// 5. Gather Candidates
	if err := agent.GatherCandidates(); err != nil {
		return nil, err
	}

	// 6. Send Initial Credentials (Offer/Answer)
	ufrag, pwd, _ := agent.GetLocalUserCredentials()
	initMsg := signaling.SignalMessage{
		Ufrag: ufrag,
		Pwd:   pwd,
	}
	if isOfferer {
		initMsg.Type = signaling.TypeOffer
	} else {
		// Answerer (Sender) waits for Offer first?
		// Actually, standard ICE: Offerer sends first. Answerer responds.
		initMsg.Type = signaling.TypeAnswer
	}

	// If Offerer, send immediately. If Answerer, wait for Offer.
	if isOfferer {
		payload, _ := json.Marshal(initMsg)
		m.Signaling.Publish(topic, payload)
	}

	// 7. Wait for Remote Credentials
	select {
	case u := <-remoteUfrag:
		p := <-remotePwd
		if !isOfferer {
			// Answerer: Now send our credentials
			payload, _ := json.Marshal(initMsg)
			m.Signaling.Publish(topic, payload)
		}
		// Set Remote
		if err := agent.SetRemoteCredentials(u, p); err != nil {
			return nil, err
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// 8. Add Candidates
	go func() {
		for {
			select {
			case c := <-remoteCandidates:
				candidate, err := ice.UnmarshalCandidate(c)
				if err == nil {
					agent.AddRemoteCandidate(candidate)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// 9. Start Connectivity Checks
	// Agent automatically starts when remote candidates interacting
	// We wait for connection
	connected := make(chan struct{})
	agent.OnConnectionStateChange(func(s ice.ConnectionState) {
		if s == ice.ConnectionStateConnected {
			close(connected)
		}
	})

	select {
	case <-connected:
		return agent, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("ice connection timed out")
	}
}

// Helper: Wrap Agent in PacketConn?
// Usually accessing agent.GetSelectedCandidatePair().Conn() gives the underlying net.PacketConn (UDP)
// but it might be shared.
// For PoC: We return the Agent, caller handles stream/wrapping.
