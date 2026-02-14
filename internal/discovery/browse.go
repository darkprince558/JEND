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
						var ip net.IP
						if len(entry.AddrIPv4) > 0 {
							ip = entry.AddrIPv4[0]
						} else if len(entry.AddrIPv6) > 0 {
							// Try to find a Global Unicast IPv6 address first
							for _, v6 := range entry.AddrIPv6 {
								if !v6.IsLinkLocalUnicast() {
									ip = v6
									break
								}
							}
							// Fallback to first IPv6 if no global address found
							if ip == nil {
								ip = entry.AddrIPv6[0]
							}
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
func LookupCloud(code string) (*RegistryItem, error) {
	client := NewRegistryClient()
	return client.Lookup(code)
}
