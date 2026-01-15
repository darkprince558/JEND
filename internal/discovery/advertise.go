package discovery

import (
	"fmt"

	"github.com/grandcat/zeroconf"
)

// StartAdvertising announces the JEND service on the local network.
// It returns a shutdown function that should be called when advertising is no longer needed.
func StartAdvertising(port int, code string) (func(), error) {
	// Instance name can be anything distinctive.
	// We use the Code Hash as the TXT record to verify intent,
	// but we can also put it in the instance name for easier debugging/visibility.
	// Let's use "JendSender-<Hash[:8]>"

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

	return server.Shutdown, nil
}
