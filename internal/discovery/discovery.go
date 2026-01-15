package discovery

import (
	"crypto/sha256"
	"fmt"
)

// ServiceType is the mDNS service type for JEND
const ServiceType = "_jend._udp"

// ComputeHash returns the SHA256 hash of the code.
// We broadcast the hash, not the code itself, to provide a minimal layer of privacy,
// preventing casual observers from seeing the exact PAKE code on the wire.
func ComputeHash(code string) string {
	sum := sha256.Sum256([]byte(code))
	return fmt.Sprintf("%x", sum)
}
