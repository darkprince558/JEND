package discovery

import (
	"context"
	"fmt"
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
						if len(entry.AddrIPv4) > 0 {
							port := entry.Port
							ip := entry.AddrIPv4[0] // Just take first IPv4
							return fmt.Sprintf("%s:%d", ip, port), nil
						}
					}
				}
			}
		}
	}
}
