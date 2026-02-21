package core

import (
	"fmt"
	"net"
)

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
