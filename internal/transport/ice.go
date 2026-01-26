package transport

import (
	"context"
	"fmt"

	"encoding/json"
	"net/http"
	"time"

	"github.com/pion/ice/v2"
)

const (
	StunServer = "stun:stun.l.google.com:19302"
	AuthAPI    = "https://k4fa8k5sjg.execute-api.us-east-1.amazonaws.com/turn-auth"
)

// TurnCredentials represents the ephemeral credentials returned by the TURN Auth API.
type TurnCredentials struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	TTL      int      `json:"ttl"`
	URIs     []string `json:"uris"`
}

// NewICEAgent creates a new ICE agent configured with our STUN/TURN servers.
// It fetches ephemeral credentials from the AuthAPI if needed.
func NewICEAgent(ctx context.Context, isControlling bool) (*ice.Agent, error) {
	// 1. Configure ICE Servers
	urls := []*ice.URL{}

	// STUN
	stunURL, err := ice.ParseURL(StunServer)
	if err != nil {
		return nil, fmt.Errorf("failed to parse stun url: %w", err)
	}
	urls = append(urls, stunURL)

	// TURN (Dynamic Auth)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(AuthAPI)
	if err != nil {
		fmt.Printf("Warning: Failed to fetch TURN credentials: %v\n", err)
	} else {
		defer resp.Body.Close()
		var creds TurnCredentials
		if err := json.NewDecoder(resp.Body).Decode(&creds); err == nil {
			for _, uri := range creds.URIs {
				turnURL, err := ice.ParseURL(uri)
				if err == nil {
					turnURL.Username = creds.Username
					turnURL.Password = creds.Password
					urls = append(urls, turnURL)
				}
			}
		} else {
			fmt.Printf("Warning: Failed to decode TURN credentials: %v\n", err)
		}
	}

	// 2. Create Agent
	agent, err := ice.NewAgent(&ice.AgentConfig{
		Urls:           urls,
		CandidateTypes: []ice.CandidateType{ice.CandidateTypeHost, ice.CandidateTypeServerReflexive, ice.CandidateTypeRelay},
		NetworkTypes:   []ice.NetworkType{ice.NetworkTypeUDP4, ice.NetworkTypeTCP4}, // Try both
		Lite:           false,
		InterfaceFilter: func(name string) bool {
			// Ignore docker interfaces if needed, but safer to try all
			return true
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create ice agent: %w", err)
	}

	return agent, nil
}
