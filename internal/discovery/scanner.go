package discovery

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	// scanTimeout is the aggressive per-host TCP dial timeout.
	// 200ms keeps the full /24 scan under 1 second.
	scanTimeout = 200 * time.Millisecond

	// scannerMagic is the handshake line sent/expected by the scanner
	// to confirm the remote is actually a JEND instance, not a random service.
	scannerMagic = "JEND-SCAN"
)

// ScanResult holds a discovered peer from the subnet scan.
type ScanResult struct {
	IP   string
	Port int
}

// ScanSubnet brute-force scans the local /24 subnet for a JEND sender
// listening on the given port. It spins up 254 goroutines (one per IP)
// and returns the first peer that responds with the JEND handshake.
//
// This is used as a fallback when mDNS multicast is blocked by the network.
func ScanSubnet(localIP string, port int, codeHash string) (*ScanResult, error) {
	// Derive the /24 subnet from our own IP
	parts := strings.Split(localIP, ".")
	if len(parts) != 4 {
		return nil, fmt.Errorf("invalid local IP for subnet scan: %s", localIP)
	}
	prefix := strings.Join(parts[:3], ".") // e.g. "192.168.1"

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		found *ScanResult
	)

	for i := 1; i <= 254; i++ {
		targetIP := fmt.Sprintf("%s.%d", prefix, i)

		// Skip our own IP
		if targetIP == localIP {
			continue
		}

		wg.Add(1)
		go func(ip string) {
			defer wg.Done()

			// Quick check: if another goroutine already found the peer, bail
			mu.Lock()
			alreadyFound := found != nil
			mu.Unlock()
			if alreadyFound {
				return
			}

			addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
			conn, err := net.DialTimeout("tcp", addr, scanTimeout)
			if err != nil {
				return // Host unreachable or port closed — expected for 253/254 hosts
			}
			defer conn.Close()

			// Send our scanner magic + the code hash for verification
			_ = conn.SetDeadline(time.Now().Add(scanTimeout))
			_, err = fmt.Fprintf(conn, "%s %s\n", scannerMagic, codeHash)
			if err != nil {
				return
			}

			// Read response — expect "JEND-SCAN-OK"
			buf := make([]byte, 128)
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			response := strings.TrimSpace(string(buf[:n]))
			if response == "JEND-SCAN-OK" {
				mu.Lock()
				if found == nil {
					found = &ScanResult{IP: ip, Port: port}
				}
				mu.Unlock()
			}
		}(targetIP)
	}

	wg.Wait()

	if found == nil {
		return nil, fmt.Errorf("no JEND peer found on subnet %s.0/24", prefix)
	}
	return found, nil
}

// ScanListenerPort is the port used by the subnet scanner's TCP listener.
// It must match the port passed to ScanSubnet.
const ScanListenerPort = 7291

// StartScanListener starts a TCP listener that responds to subnet scanner probes.
// It verifies the incoming code hash matches before responding with "JEND-SCAN-OK".
// Returns a shutdown function.
func StartScanListener(code string) (func(), error) {
	codeHash := ComputeHash(code)

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", ScanListenerPort))
	if err != nil {
		return nil, fmt.Errorf("scan listener failed: %w", err)
	}

	done := make(chan struct{})

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-done:
					return // clean shutdown
				default:
					continue
				}
			}

			go handleScanProbe(conn, codeHash)
		}
	}()

	shutdown := func() {
		close(done)
		_ = ln.Close()
	}
	return shutdown, nil
}

// handleScanProbe reads the scanner's magic + hash and responds if valid.
func handleScanProbe(conn net.Conn, expectedHash string) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(500 * time.Millisecond))

	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		return
	}

	line := strings.TrimSpace(string(buf[:n]))
	parts := strings.SplitN(line, " ", 2)
	if len(parts) != 2 || parts[0] != scannerMagic {
		return
	}

	receivedHash := parts[1]
	if receivedHash != expectedHash {
		return // Wrong code — not our peer
	}

	// Verified! Respond with OK
	_, _ = fmt.Fprintln(conn, "JEND-SCAN-OK")
}
