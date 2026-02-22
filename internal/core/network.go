package core

import (
	"fmt"
	"net"
)

// LocalAddresses holds the discovered IPv4 and IPv6 link-local addresses.
type LocalAddresses struct {
	IPv4     string // e.g. "192.168.1.50"
	IPv6     string // e.g. "fe80::1abc:2def:3456:7890"
	IPv6Zone string // e.g. "en0" (interface name, needed for link-local URLs)
}

// GetLocalAddresses returns both the IPv4 and IPv6 link-local addresses
// for the active network interface. Either may be empty if unavailable.
func GetLocalAddresses() LocalAddresses {
	addrs := LocalAddresses{}
	addrs.IPv4, _ = GetLocalIP()
	addrs.IPv6, addrs.IPv6Zone, _ = GetLocalIPv6()
	return addrs
}

// GetLocalIP returns the best non-loopback IPv4 address for use in QR code URLs.
// It prefers addresses from interfaces that have a default route (i.e., the one
// used to reach the internet).
func GetLocalIP() (string, error) {
	// Strategy: Dial a known public IP (no actual connection is made for UDP).
	// The OS will select the correct outbound interface / source IP.
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return fallbackLocalIP()
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	if localAddr.IP.IsLoopback() || localAddr.IP.IsUnspecified() {
		return fallbackLocalIP()
	}
	return localAddr.IP.String(), nil
}

// GetLocalIPv6 returns the IPv6 link-local address and zone ID for the default
// network interface. These fe80:: addresses bypass most AP Isolation firewalls.
func GetLocalIPv6() (ip string, zone string, err error) {
	// Find which interface carries the default route
	conn, dialErr := net.Dial("udp4", "8.8.8.8:80")
	if dialErr != nil {
		return "", "", fmt.Errorf("cannot determine default interface: %w", dialErr)
	}
	defer conn.Close()
	defaultIP := conn.LocalAddr().(*net.UDPAddr).IP

	// Find the interface that owns this IPv4 address
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", "", fmt.Errorf("failed to list interfaces: %w", err)
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		// Check if this interface owns the default IPv4 address
		ownsDefault := false
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok {
				if ipNet.IP.Equal(defaultIP) {
					ownsDefault = true
					break
				}
			}
		}

		if !ownsDefault {
			continue
		}

		// Found the right interface — now grab its link-local IPv6
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok {
				if ipNet.IP.To4() == nil && ipNet.IP.IsLinkLocalUnicast() {
					return ipNet.IP.String(), iface.Name, nil
				}
			}
		}
	}

	return "", "", fmt.Errorf("no IPv6 link-local address found")
}

// fallbackLocalIP iterates all interfaces and picks the first non-loopback IPv4.
func fallbackLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("failed to get interface addresses: %w", err)
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok {
			if !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no suitable local IP found")
}
