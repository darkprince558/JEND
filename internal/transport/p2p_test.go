package transport

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/darkprince558/jend/internal/signaling"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/pion/ice/v2"
)

// MockSignaler implements Signaler interface
type MockSignaler struct {
	SubscribeChan chan string
	PublishChan   chan []byte
	Handler       mqtt.MessageHandler
}

func NewMockSignaler() *MockSignaler {
	return &MockSignaler{
		SubscribeChan: make(chan string, 1),
		PublishChan:   make(chan []byte, 10),
	}
}

func (m *MockSignaler) Subscribe(topic string, handler mqtt.MessageHandler) error {
	m.Handler = handler
	m.SubscribeChan <- topic
	return nil
}

func (m *MockSignaler) Publish(topic string, payload []byte) error {
	m.PublishChan <- payload
	return nil
}

// MockICEAgent creates a bare agent for testing
func MockNewAgent(ctx context.Context, isControlling bool, customTurn *CustomTurnConfig) (*ice.Agent, error) {
	return ice.NewAgent(&ice.AgentConfig{
		Lite:           true,
		CandidateTypes: []ice.CandidateType{ice.CandidateTypeHost},
	})
}

func TestP2PManager_SignalFlow(t *testing.T) {
	// 1. Setup
	mockSig := NewMockSignaler()
	code := "test-code"
	mgr := NewP2PManager(mockSig, code, nil)

	// Override Agent creation
	originalFunc := NewAgentFunc
	defer func() { NewAgentFunc = originalFunc }()
	NewAgentFunc = MockNewAgent

	// 2. Start (Establish Connection context)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Run in goroutine
	done := make(chan error, 1)
	go func() {
		_, err := mgr.EstablishConnection(ctx, true)
		done <- err
	}()

	// 3. Verify Subscription (Wait for channel)
	expectedTopic := "jend/signal/test-code"
	select {
	case topic := <-mockSig.SubscribeChan:
		if topic != expectedTopic {
			t.Errorf("Expected subscription to %s, got %s", expectedTopic, topic)
		}
	case err := <-done:
		t.Fatalf("EstablishConnection returned early: %v", err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timed out waiting for Subscribe")
	}

	// 4. Verify Initial Offer Publish (Since we are Offerer)
	select {
	case payload := <-mockSig.PublishChan:
		var msg signaling.SignalMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			t.Errorf("Failed to unmarshal published message: %v", err)
		}
		if msg.Type != signaling.TypeOffer {
			t.Errorf("Expected Offer message, got %s", msg.Type)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timed out waiting for Offer Publish")
	}

	// 5. Simulate Incoming Answer
	answerMsg := signaling.SignalMessage{
		Type:  signaling.TypeAnswer,
		Ufrag: "remote-ufrag",
		Pwd:   "remote-pwd",
	}
	payload, _ := json.Marshal(answerMsg)
	mockMsg := &mockMessage{payload: payload}

	// Call the handler to simulate receiving MQTT message
	if mockSig.Handler != nil {
		mockSig.Handler(nil, mockMsg)
	}

	// Note: We don't check for ICE connection success because MockNewAgent doesn't connect.
	// But we verified the Signaling Flow (Subscribe -> Offer -> Receive Answer).
}

// Helper for MQTT mock
type mockMessage struct {
	payload []byte
}

func (m *mockMessage) Duplicate() bool   { return false }
func (m *mockMessage) Qos() byte         { return 0 }
func (m *mockMessage) Retained() bool    { return false }
func (m *mockMessage) Topic() string     { return "" }
func (m *mockMessage) MessageID() uint16 { return 0 }
func (m *mockMessage) Payload() []byte   { return m.payload }
func (m *mockMessage) Ack()              {}
