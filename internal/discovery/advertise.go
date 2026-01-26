package discovery

import (
	"fmt"

	"github.com/grandcat/zeroconf"
)

// StartAdvertising announces the JEND service on the local network.
// It returns a shutdown function that should be called when advertising is no longer needed.
func StartAdvertising(port int, code string) (func(), error) {
	// Instance name: "JendSender-<Hash[:8]>"
	codeHash := ComputeHash(code)
	instanceName := fmt.Sprintf("JendSender-%s", codeHash[:8])

	// TXT record holds the full hash for the receiver to verify
	txt := []string{fmt.Sprintf("hash=%s", codeHash)}

	server, err := zeroconf.Register(
		instanceName,
		ServiceType,
		"local.",
		port,
		txt,
		nil, // Check all interfaces
	)
	if err != nil {
		return nil, err
	}

	// Register with Cloud Registry (AWS) in parallel
	// Note: We don't block on this, or we could.
	// For simplicity, let's just log errors.
	if err := RegisterWithCloud(code, "", port); err != nil {
		fmt.Printf("Warning: Cloud registration failed: %v\n", err)
	}

	return server.Shutdown, nil
}

// RegisterWithCloud registers the instance with the global AWS registry.
func RegisterWithCloud(code string, ip string, port int) error {
	client := NewRegistryClient()
	return client.Register(code, ip, port)
}
