package discovery

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// FindSender scans the network for a JEND sender matching the code.
// It tries mDNS first, then falls back to a brute-force subnet scan
// if multicast is blocked. Returns the IP:Port string if found.
func FindSender(code string, timeout time.Duration) (string, error) {
	// --- Phase 1: mDNS (fast, preferred) ---
	mdnsTimeout := timeout / 2
	if mdnsTimeout < 3*time.Second {
		mdnsTimeout = 3 * time.Second
	}
	addr, err := findSenderMDNS(code, mdnsTimeout)
	if err == nil {
		return addr, nil
	}

	// --- Phase 2: Subnet Scan (fallback when mDNS/multicast is blocked) ---
	fmt.Println("  mDNS discovery failed, trying subnet scan...")

	localIP, ipErr := getLocalIPForScan()
	if ipErr != nil {
		return "", fmt.Errorf("mDNS failed and cannot determine local IP for subnet scan: %w", ipErr)
	}

	result, scanErr := ScanSubnet(localIP, ScanListenerPort, ComputeHash(code))
	if scanErr != nil {
		return "", fmt.Errorf("mDNS and subnet scan both failed: %w", scanErr)
	}

	return net.JoinHostPort(result.IP, fmt.Sprintf("%d", result.Port)), nil
}

// getLocalIPForScan returns the local IPv4 address for subnet scanning.
func getLocalIPForScan() (string, error) {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}

// findSenderMDNS is the original mDNS-based sender discovery.
func findSenderMDNS(code string, timeout time.Duration) (string, error) {
	entries := make(chan *MDNSServiceEntry)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Hash the code to compare with TXT records
	targetHash := ComputeHash(code)

	if err := NewMDNSProvider().Browse(ctx, ServiceType, "local.", entries); err != nil {
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
