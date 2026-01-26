package e2e

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/darkprince558/jend/internal/simulation"
	"github.com/darkprince558/jend/internal/transport"
)

// TestPacketLoss verifies basic data integrity over a lossy link.
// We are simulating "QUIC over UDP" manually here since we can't easily hook into
// the internal `RunSender` logic without heavy DI.
// Instead, we test the Transport/Protocol layer resiliency directly.
func TestPacketLoss(t *testing.T) {
	// 1. Setup Lossy Connection Pair
	// Using localhost UDP but wrapped
	pc1, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc1.Close()

	pc2, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc2.Close()

	// Wrap PC1 (Sender) with 20% loss
	lossyPC1 := simulation.NewLossyPacketConn(pc1, 0.20, 10*time.Millisecond)

	// 2. Setup QUIC Listeners
	tr := transport.NewQUICTransport()

	// Receiver listens on PC2
	ln, err := tr.ListenPacket(pc2)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// 3. Sender Dials Receiver using Lossy PC1
	// Context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	// Receiver Logic
	go func() {
		defer wg.Done()
		conn, err := ln.Accept(ctx)
		if err != nil {
			t.Logf("Accept error: %v", err)
			return
		}
		stream, err := conn.AcceptStream(ctx)
		if err != nil {
			t.Error(err)
			return
		}

		// Echo Loop
		buf := make([]byte, 1024)
		for {
			n, err := stream.Read(buf)
			if err != nil {
				break
			}
			stream.Write(buf[:n])
		}
	}()

	// Dial
	conn, err := tr.DialPacket(lossyPC1, pc2.LocalAddr())
	if err != nil {
		t.Fatal(err)
	}

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// 4. Send Data and Verify
	msg := []byte("Hello Resilience World!")
	for i := 0; i < 100; i++ { // Send enough packets to guarantee some are dropped
		_, err := stream.Write(msg)
		if err != nil {
			t.Fatal(err)
		}

		reply := make([]byte, len(msg))
		_, err = stream.Read(reply)
		if err != nil {
			t.Fatal(err)
		}

		if string(reply) != string(msg) {
			t.Fatalf("Mismatch at iteration %d", i)
		}
	}

	stream.Close()
	wg.Wait()
	t.Log("Successfully sent 100 messages with 20% packet loss!")
}

func TestHighLatency(t *testing.T) {
	// 1. Setup Latency Connection Pair (500ms RTT = 250ms one way)
	pc1, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc1.Close()

	pc2, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc2.Close()

	// Wrap PC1 (Sender) with 250ms latency (500ms RTT)
	// No loss, just delay
	simPC1 := simulation.NewLossyPacketConn(pc1, 0.0, 250*time.Millisecond)

	// 2. Setup QUIC Listeners
	tr := transport.NewQUICTransport()

	ln, err := tr.ListenPacket(pc2)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// 3. Sender Dials Receiver using Simulated PC
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second) // Increased timeout for latency
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		conn, err := ln.Accept(ctx)
		if err != nil {
			t.Logf("Accept error: %v", err)
			return
		}
		stream, err := conn.AcceptStream(ctx)
		if err != nil {
			t.Error(err)
			return
		}
		// Echo
		io.Copy(stream, stream)
	}()

	conn, err := tr.DialPacket(simPC1, pc2.LocalAddr())
	if err != nil {
		t.Fatal(err)
	}

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	msg := []byte("Ping")
	_, err = stream.Write(msg)
	if err != nil {
		t.Fatal(err)
	}

	reply := make([]byte, len(msg))
	_, err = stream.Read(reply)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	if string(reply) != string(msg) {
		t.Fatal("Message corrupted")
	}

	// Verify Latency
	// QUIC handshake (1-RTT) + Data (1-RTT) = ~500ms minimum if delay applied correctly?
	// Actually, simulated latency is on WriteTo.
	// Handshake: Client Hello (delayed 250ms) -> Server Hello (immediate) -> Client ...
	// Since we wrap PC1, only OUTBOUND packets from PC1 are delayed.
	// PC2 is normal.
	// C->S (250ms)
	// S->C (0ms)
	// RTT seen by QUIC is 250ms.
	// We expect at least 250ms elapsed.
	if elapsed < 250*time.Millisecond {
		t.Fatalf("Operation too fast (%v), latency simulation failed", elapsed)
	}

	t.Logf("High Latency Test Passed! Round Trip: %v", elapsed)

	stream.Close()
	wg.Wait()
}
