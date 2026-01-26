package simulation

import (
	"math/rand"
	"net"
	"sync"
	"time"
)

// LossyPacketConn wraps a net.PacketConn and injects loss/latency
type LossyPacketConn struct {
	net.PacketConn
	lossRate float64       // 0.0 to 1.0 (e.g. 0.2 = 20% loss)
	latency  time.Duration // Fixed latency per packet
	jitter   time.Duration // Random jitter +/-
	mu       sync.Mutex
	rand     *rand.Rand
}

func NewLossyPacketConn(c net.PacketConn, lossRate float64, latency time.Duration) *LossyPacketConn {
	return &LossyPacketConn{
		PacketConn: c,
		lossRate:   lossRate,
		latency:    latency,
		rand:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (c *LossyPacketConn) SetLossRate(rate float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lossRate = rate
}

// WriteTo delays or drops packets
func (c *LossyPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	c.mu.Lock()
	loss := c.lossRate
	lat := c.latency
	r := c.rand.Float64()
	c.mu.Unlock()

	// 1. Simulate Loss
	if r < loss {
		// Drop packet (return success to caller so they don't know)
		return len(p), nil
	}

	// 2. Simulate Latency (in background goroutine to not block sender logic excessively,
	// although blocking might be more realistic for link congestion?
	// For UDP, non-blocking delay is better simulation of "on the wire" time)
	if lat > 0 {
		// Isolate data buffer for async
		data := make([]byte, len(p))
		copy(data, p)
		go func() {
			time.Sleep(lat)
			c.PacketConn.WriteTo(data, addr)
		}()
		return len(p), nil
	}

	return c.PacketConn.WriteTo(p, addr)
}

// ReadFrom - strictly speaking, loss/latency usually happens on the "wire" (WriteTo).
// But we could simulate inbound loss too. For now, outbound is sufficient for E2E.
func (c *LossyPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	return c.PacketConn.ReadFrom(p)
}
