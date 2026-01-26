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
	// 1. Get AWS Credentials via Cognito
	// TODO: Externalize IdentityPoolID configuration.
	identityPoolID := os.Getenv("JEND_IDENTITY_POOL_ID")
	if identityPoolID == "" {
		identityPoolID = "us-east-1:63825811-2a43-4a2b-893c-ce78d256819d"
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
		return nil, fmt.Errorf("failed to retrieve aws credentials: %w", err)
	}

	// 2. Sign the Websocket URL
	// AWS IoT Core supports WSS on port 443 with SigV4
	signer := v4.NewSigner()
	req, _ := http.NewRequest("GET", fmt.Sprintf("wss://%s/mqtt", iotEndpoint), nil)

	// Sign the request
	// We need to sign with service "iotdevicegateway"
	// Payload hash for GET is empty string hash
	emptyHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	err = signer.SignHTTP(ctx, creds, req, emptyHash, "iotdevicegateway", region, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to sign websocket request: %w", err)
	}

	// 3. Configure MQTT Client
	opts := mqtt.NewClientOptions()
	opts.AddBroker(req.URL.String())
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
