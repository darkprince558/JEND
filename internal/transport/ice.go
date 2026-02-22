package transport

import (
	"context"
	"fmt"
	"net"

	"encoding/json"
	"net/http"
	"time"

	"github.com/darkprince558/jend/internal/config"
	"github.com/pion/ice/v2"
)

const (
	StunServer = config.DefaultSTUNServer
	AuthAPI    = config.DefaultAPIEndpoint + "/turn-auth"
)

// TurnCredentials represents the ephemeral credentials returned by the TURN Auth API.
type TurnCredentials struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	TTL      int      `json:"ttl"`
	URIs     []string `json:"uris"`
}

// CustomTurnConfig holds user-provided TURN credentials
type CustomTurnConfig struct {
	URL      string
	Username string
	Password string
}

// NewICEAgent creates a new ICE agent configured with our STUN/TURN servers.
// It fetches ephemeral credentials from the AuthAPI if custom config is nil.
// If custom config is provided, it uses that instead.
//
// The returned cleanup function MUST be called when the agent is no longer
// needed to close the TCP mux listener.
func NewICEAgent(ctx context.Context, isControlling bool, customTurn *CustomTurnConfig) (*ice.Agent, error) {
	// 1. Configure ICE Servers
	//nolint:staticcheck // ice/v2 requires ice.URL which is deprecated
	urls := []*ice.URL{}

	// STUN
	//nolint:staticcheck // ice/v2 requires ice.URL which is deprecated in favor of stun.URI, but ice/v2 still uses it.
	stunURL, err := ice.ParseURL(StunServer)
	if err != nil {
		return nil, fmt.Errorf("failed to parse stun url: %w", err)
	}
	urls = append(urls, stunURL)

	// TURN Configuration
	if customTurn != nil && customTurn.URL != "" {
		// Use User-Provided Relay
		//nolint:staticcheck // See above
		turnURL, err := ice.ParseURL(customTurn.URL)
		if err != nil {
			return nil, fmt.Errorf("invalid custom relay url: %w", err)
		}
		turnURL.Username = customTurn.Username
		turnURL.Password = customTurn.Password
		urls = append(urls, turnURL)
		fmt.Printf("Using Custom Relay: %s\n", customTurn.URL)
	} else {
		// Use Default (Dynamic Auth)
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(AuthAPI)
		if err != nil {
			fmt.Printf("Warning: Failed to fetch TURN credentials: %v\n", err)
		} else {
			defer resp.Body.Close() //nolint:errcheck
			var creds TurnCredentials
			if err := json.NewDecoder(resp.Body).Decode(&creds); err == nil {
				for _, uri := range creds.URIs {
					//nolint:staticcheck // See above
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
	}

	// 2. Create TCP mux listener for passive TCP ICE candidates.
	// This allows peer-to-peer TCP connections when UDP is fully blocked
	// by school/enterprise firewalls.
	var tcpMux ice.TCPMux
	tcpListener, err := net.ListenTCP("tcp", &net.TCPAddr{Port: 0}) // dynamic port
	if err == nil {
		tcpMux = ice.NewTCPMuxDefault(ice.TCPMuxParams{
			Listener: tcpListener,
		})
	}

	// 3. Create Agent with full network coverage:
	//    - UDP4/UDP6: standard + IPv6 link-local bypass for AP isolation
	//    - TCP4/TCP6: fallback when UDP is firewalled
	agentCfg := &ice.AgentConfig{
		Urls:           urls,
		CandidateTypes: []ice.CandidateType{ice.CandidateTypeHost, ice.CandidateTypeServerReflexive, ice.CandidateTypeRelay},
		NetworkTypes: []ice.NetworkType{
			ice.NetworkTypeUDP4,
			ice.NetworkTypeUDP6, // IPv6 link-local bypasses most AP isolation
			ice.NetworkTypeTCP4,
			ice.NetworkTypeTCP6,
		},
		Lite: false,
		InterfaceFilter: func(name string) bool {
			return true
		},
	}

	if tcpMux != nil {
		agentCfg.TCPMux = tcpMux
	}

	agent, err := ice.NewAgent(agentCfg)
	if err != nil {
		if tcpListener != nil {
			_ = tcpListener.Close()
		}
		return nil, fmt.Errorf("failed to create ice agent: %w", err)
	}

	return agent, nil
}
