package discovery

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
)

func TestHashComputation(t *testing.T) {
	code := "test-code-123"
	expectedSum := sha256.Sum256([]byte(code))
	expected := fmt.Sprintf("%x", expectedSum)

	result := ComputeHash(code)
	if result != expected {
		t.Errorf("ComputeHash(%q) = %q, want %q", code, result, expected)
	}
}

func TestAdvertiseAndBrowse(t *testing.T) {
	// This test integrates both Advertise and Browse on the loopback interface.
	// Note: mDNS tests can be flaky in some CI/container environments that don't support multicast.
	// We will try our best to run it locally.

	port := 9999 // Arbitrary test port
	code := "unit-test-code-discovery"

	// 1. Start Advertising
	stop, err := StartAdvertising(port, code)
	if err != nil {
		t.Fatalf("Failed to start advertising: %v", err)
	}
	defer stop()

	// Give a moment for the service to register
	time.Sleep(500 * time.Millisecond)

	// 2. Try to Find it
	// Reduce timeout for test speed
	foundAddr, err := FindSender(code, 2*time.Second)
	if err != nil {
		// Diagnostic: check if we can find ANY jend service
		resolver, _ := zeroconf.NewResolver(nil)
		entries := make(chan *zeroconf.ServiceEntry)
		go func() {
			resolver.Browse(context.Background(), ServiceType, "local.", entries)
		}()
		select {
		case e := <-entries:
			t.Logf("Found unrelated service: %s %v", e.Instance, e.Text)
		case <-time.After(1 * time.Second):
			t.Log("No services found at all")
		}

		t.Fatalf("FindSender failed: %v", err)
	}

	// 3. Verify
	// IP might vary (IPv4 vs IPv6), but port should match
	// Format is ip:port
	// We expect port 9999
	expectedSuffix := fmt.Sprintf(":%d", port)
	if len(foundAddr) <= len(expectedSuffix) || foundAddr[len(foundAddr)-len(expectedSuffix):] != expectedSuffix {
		t.Errorf("Found address %q, expected port %d", foundAddr, port)
	}
}

func TestBrowseNotFound(t *testing.T) {
	// Search for a code that definitely doesn't exist
	code := "non-existent-ghost-code"

	// Should timeout
	start := time.Now()
	_, err := FindSender(code, 500*time.Millisecond)
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected error (timeout), got success")
	}

	if duration < 500*time.Millisecond {
		t.Error("Returned too early, didn't wait for timeout")
	}
}
