// Package discovery handles peer discovery mechanisms including mDNS (local)
// and the Cloud Registry (global).
package discovery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/darkprince558/jend/internal/config"
)

// RegistryClient handles interaction with the global JEND Registry Service.
type RegistryClient struct {
	client   *http.Client
	Endpoint string
}

// NewRegistryClient creates a new client with a default timeout.
func NewRegistryClient() *RegistryClient {
	return &RegistryClient{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		Endpoint: config.DefaultAPIEndpoint,
	}
}

// RegistryItem represents the data structure stored/retrieved.
// RegistryItem represents the data structure stored/retrieved.
type RegistryItem struct {
	Code      string `json:"code"`
	IP        string `json:"ip"`
	Port      int    `json:"port"`
	PublicKey []byte `json:"public_key,omitempty"` // Used for S3 Metadata
}

// Register sends a POST request to register this peer.
func (c *RegistryClient) Register(code, ip string, port int, pubKey []byte) error {
	item := RegistryItem{
		Code:      code,
		IP:        ip,
		Port:      port,
		PublicKey: pubKey,
	}

	body, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("marshal failed: %w", err)
	}

	url := fmt.Sprintf("%s/register", c.Endpoint)
	resp, err := c.client.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("register request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("register failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// Lookup sends a GET request to find a peer by code.
func (c *RegistryClient) Lookup(code string) (*RegistryItem, error) {
	url := fmt.Sprintf("%s/lookup/%s", c.Endpoint, code)
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("lookup request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("peer not found")
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("lookup failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var item RegistryItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}

	return &item, nil
}

// Deregister sends a DELETE request to remove a peer by code.
func (c *RegistryClient) Deregister(code string) error {
	url := fmt.Sprintf("%s/lookup/%s", c.Endpoint, code)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("create deregister request failed: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("deregister request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("deregister failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}
