package discovery

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"
)

// FindSender scans the network for a JEND sender matching the code.
// It returns the IP:Port string if found, or an error if timed out.
func FindSender(code string, timeout time.Duration) (string, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return "", err
	}

	entries := make(chan *zeroconf.ServiceEntry)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Hash the code to compare with TXT records
	targetHash := ComputeHash(code)

	if err := resolver.Browse(ctx, ServiceType, "local.", entries); err != nil {
		return "", err
	}

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("sender not found (timeout)")
		case entry := <-entries:
			if entry == nil {
				continue
			}
			// Check TXT record
			// Format: "hash=<hash>"
			for _, txt := range entry.Text {
				if strings.HasPrefix(txt, "hash=") {
					h := strings.TrimPrefix(txt, "hash=")
					if h == targetHash {
						// Match Found!
						// Match Found!
						// Prefer IPv6 for local link (usually better for P2P/AirDrop-like behavior)
						// But for now, let's just return the first available address.
						var ip net.IP
						if len(entry.AddrIPv6) > 0 {
							ip = entry.AddrIPv6[0]
						} else if len(entry.AddrIPv4) > 0 {
							ip = entry.AddrIPv4[0]
						}

						if ip != nil {
							port := entry.Port
							// Format IPv6 address correctly [::1]:port
							// internal/transport/quic.go Dial function expects "host:port" or "[host]:port"
							// net.JoinHostPort handles this.
							return net.JoinHostPort(ip.String(), fmt.Sprintf("%d", port)), nil
						}
					}
				}
			}
		}
	}
}

// LookupCloud queries the global registry for the sender.
func LookupCloud(code string) (string, error) {
	client := NewRegistryClient()
	item, err := client.Lookup(code)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", item.IP, item.Port), nil
}
