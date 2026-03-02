package discovery

import (
	"context"
	"net"
)

// MDNSServiceEntry represents a discovered service on the network
type MDNSServiceEntry struct {
	Instance string
	Service  string
	Domain   string
	HostName string
	Port     int
	Text     []string
	AddrIPv4 []net.IP
	AddrIPv6 []net.IP
}

// MDNSProvider defines an abstraction for mDNS operations.
// This allows us to use standard zeroconf on macOS/Windows
// and native Avahi D-Bus on Linux to prevent port conflicts.
type MDNSProvider interface {
	// Advertise announces a service on the local network.
	// Returns a shutdown function and an error.
	Advertise(instanceName, serviceType, domain string, port int, txt []string) (func(), error)

	// Browse searches the network for services matching the type and domain.
	Browse(ctx context.Context, serviceType, domain string, entriesCh chan<- *MDNSServiceEntry) error
}

// MDNSProviderFactory is a variable that should be set by OS-specific files
// (e.g., mdns_darwin.go, mdns_linux.go) to provide the appropriate implementation.
var DefaultMDNSProvider MDNSProvider

// NewMDNSProvider returns the platform-specific mDNS provider.
func NewMDNSProvider() MDNSProvider {
	return DefaultMDNSProvider
}
