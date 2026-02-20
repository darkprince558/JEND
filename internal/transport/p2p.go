package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/darkprince558/jend/internal/signaling"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/pion/ice/v2"
)

// Signaler abstracts the signaling channel (MQTT)
type Signaler interface {
	Subscribe(topic string, handler mqtt.MessageHandler) error
	Publish(topic string, payload []byte) error
}

var NewAgentFunc = NewICEAgent

// P2PManager handles the establishment of a P2P connection via ICE & MQTT
type P2PManager struct {
	Signaling  Signaler
	Code       string
	Agent      *ice.Agent
	TurnConfig *CustomTurnConfig
}

// NewP2PManager creates a manager for a specific transfer session
func NewP2PManager(sig Signaler, code string, turnCfg *CustomTurnConfig) *P2PManager {
	return &P2PManager{
		Signaling:  sig,
		Code:       code,
		TurnConfig: turnCfg,
	}
}

// EstablishConnection performs the ICE handshake to setup a P2P connection.
// It acts as the Offerer if isOfferer is true (Receiver role), otherwise as Answerer (Sender role).
func (m *P2PManager) EstablishConnection(ctx context.Context, isOfferer bool) (net.PacketConn, error) {
	// 1. Create ICE Agent
	agent, err := NewAgentFunc(ctx, isOfferer, m.TurnConfig)
	if err != nil {
		return nil, err
	}
	m.Agent = agent

	topic := fmt.Sprintf("jend/signal/%s", m.Code)

	remoteCandidates := make(chan string, 10)
	remoteUfrag := make(chan string, 1)
	remotePwd := make(chan string, 1)

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

		payload, err := json.Marshal(msg)
		if err != nil {
			fmt.Printf("Failed to marshal candidate: %v\n", err)
			return
		}
		if err := m.Signaling.Publish(topic, payload); err != nil {
			fmt.Printf("Failed to publish candidate: %v\n", err)
		}
	})

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
		// Answerer (Sender) waits for Offer first.
		initMsg.Type = signaling.TypeAnswer
	}

	// If Offerer, send immediately. If Answerer, wait for Offer.
	if isOfferer {
		payload, _ := json.Marshal(initMsg)
		m.Signaling.Publish(topic, payload)
	}

	// 7. Wait for Remote Credentials
	var rUfrag, rPwd string
	select {
	case u := <-remoteUfrag:
		rUfrag = u
		p := <-remotePwd
		rPwd = p

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
	// We wait for connection via Dial

	// Dial returns the connection. It acts as a "connect until done".
	// Since we already did SetRemoteCredentials, Dial handles the check loop.
	// Note: Allow cancel via context.

	conn, err := agent.Dial(ctx, rUfrag, rPwd)
	if err != nil {
		return nil, fmt.Errorf("ice dial failed: %w", err)
	}

	return &IcePacketConn{Conn: conn}, nil
}

// IcePacketConn wraps *ice.Conn to satisfy net.PacketConn.
type IcePacketConn struct {
	*ice.Conn
}

func (c *IcePacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, err = c.Read(p)
	return n, c.RemoteAddr(), err
}

func (c *IcePacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	return c.Write(p)
}
