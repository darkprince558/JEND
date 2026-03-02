package discovery

import (
	"fmt"
)

// StartAdvertising announces the JEND service on the local network.
// It uses mDNS (primary) and a TCP scan listener (fallback) for discovery.
// It returns a shutdown function that should be called when advertising is no longer needed.
func StartAdvertising(port int, code string) (func(), error) {
	// Instance name: "JendSender-<Hash[:8]>"
	codeHash := ComputeHash(code)
	instanceName := fmt.Sprintf("JendSender-%s", codeHash[:8])

	// TXT record holds the full hash for the receiver to verify
	txt := []string{fmt.Sprintf("hash=%s", codeHash)}

	shutdownMdns, err := NewMDNSProvider().Advertise(
		instanceName,
		ServiceType,
		"local.",
		port,
		txt,
	)
	if err != nil {
		fmt.Printf("Warning: mDNS advertising failed: %v\n", err)
	}

	// Start subnet scan listener (fallback when mDNS is blocked)
	scanShutdown, scanErr := StartScanListener(code)
	if scanErr != nil {
		fmt.Printf("Warning: Scan listener failed: %v\n", scanErr)
	}

	// Register with Cloud Registry (AWS) in parallel
	// Log errors but do not block execution.
	if err := RegisterWithCloud(code, "", port); err != nil {
		fmt.Printf("Warning: Cloud registration failed: %v\n", err)
	}

	shutdown := func() {
		if shutdownMdns != nil {
			shutdownMdns()
		}
		if scanShutdown != nil {
			scanShutdown()
		}
	}
	return shutdown, nil
}

// RegisterWithCloud registers the instance with the global AWS registry.
func RegisterWithCloud(code string, ip string, port int) error {
	client := NewRegistryClient()
	return client.Register(code, ip, port, nil)
}
