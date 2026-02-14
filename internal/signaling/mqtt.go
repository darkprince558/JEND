package signaling

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/darkprince558/jend/internal/auth"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	iotEndpoint = "a10ofg7qwmr003-ats.iot.us-east-1.amazonaws.com"
	region      = "us-east-1"
)

// IoTClient handles MQTT connections to AWS IoT Core.
type IoTClient struct {
	client mqtt.Client
}

// NewIoTClient creates a new authenticated MQTT client.
func NewIoTClient(ctx context.Context, clientID string) (*IoTClient, error) {
	// Check for custom endpoint (Testing/Local)
	customEndpoint := os.Getenv("JEND_MQTT_ENDPOINT")
	var brokerURL string
	var req *http.Request

	if customEndpoint != "" {
		brokerURL = customEndpoint
		// Skip AWS Signing and Credential Retrieval for custom/local broker
	} else {
		// 1. Get AWS Credentials via Cognito
		identityPoolID := os.Getenv("JEND_IDENTITY_POOL_ID")
		if identityPoolID == "" {
			// Updated with new cost-optimized deployment ID
			identityPoolID = "us-east-1:6b98c4f2-9fea-4591-8b0f-f34be0a4da23"
		}

		// Initial config to get region/defaults
		cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
		if err != nil {
			return nil, fmt.Errorf("failed to load base aws config: %w", err)
		}

		// Use Cognito Provider
		credsProvider := auth.NewCognitoProvider(cfg, identityPoolID)

		// Reload config with credentials provider
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithCredentialsProvider(credsProvider),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load aws config with cognito: %w", err)
		}

		creds, err := cfg.Credentials.Retrieve(ctx)
		if err != nil {
			// This captures the ResourceNotFoundException
			return nil, fmt.Errorf("cloud credentials unavailable: %w", err)
		}

		// 2. Sign the Websocket URL
		// AWS IoT Core supports WSS on port 443 with SigV4
		signer := v4.NewSigner()
		req, _ = http.NewRequest("GET", fmt.Sprintf("wss://%s/mqtt", iotEndpoint), nil)

		// Sign the request
		// We need to sign with service "iotdevicegateway"
		// Payload hash for GET is empty string hash
		emptyHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

		// Use SignHTTP (Header-based) which we verified works for WebSocket handshake
		err = signer.SignHTTP(ctx, creds, req, emptyHash, "iotdevicegateway", region, time.Now())
		if err != nil {
			return nil, fmt.Errorf("failed to sign websocket request: %w", err)
		}
		brokerURL = req.URL.String()
	}

	// 3. Configure MQTT Client
	opts := mqtt.NewClientOptions()
	opts.AddBroker(brokerURL)

	// Pass signed headers to Paho to use during WebSocket Handshake
	if customEndpoint == "" && req != nil {
		opts.SetHTTPHeaders(req.Header)
	}
	opts.SetClientID(clientID)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		fmt.Printf("MQTT Connection lost: %v\n", err)
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("mqtt connect failed: %w", token.Error())
	}

	return &IoTClient{client: client}, nil
}

// Subscribe listens to a topic.
func (c *IoTClient) Subscribe(topic string, handler mqtt.MessageHandler) error {
	if token := c.client.Subscribe(topic, 1, handler); token.Wait() && token.Error() != nil {
		return fmt.Errorf("subscribe failed: %w", token.Error())
	}
	return nil
}

// Publish sends a message to a topic.
func (c *IoTClient) Publish(topic string, payload []byte) error {
	if token := c.client.Publish(topic, 1, false, payload); token.Wait() && token.Error() != nil {
		return fmt.Errorf("publish failed: %w", token.Error())
	}
	return nil
}

// Disconnect closes the connection.
func (c *IoTClient) Disconnect() {
	if c.client != nil {
		c.client.Disconnect(250)
	}
}
