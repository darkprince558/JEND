package transport

import (
	"context"
	"net"
	"sync"

	"github.com/quic-go/quic-go"
)

// MultiListener aggregates multiple QUICListeners into a single Accept loop.
// This allows the server to accept connections from both Direct UDP and P2P/ICE simultaneously.
type MultiListener struct {
	listeners []QUICListener
	conns     chan *quic.Conn
	errors    chan error
	done      chan struct{}
	mu        sync.Mutex
}

func NewMultiListener() *MultiListener {
	return &MultiListener{
		conns:  make(chan *quic.Conn), // Unbuffered or buffered? Unbuffered is safer flow control logic usually, but buffered is fine.
		errors: make(chan error),
		done:   make(chan struct{}),
	}
}

// Add registers a new listener and starts an accept loop for it.
func (m *MultiListener) Add(l QUICListener) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, l)

	go func() {
		for {
			conn, err := l.Accept(context.Background())
			if err != nil {
				// If the listener is closed, we stop this loop.
				// We don't propagate error necessarily as one listener failing shouldn't kill others (e.g. ICE fails but Direct works).
				// But real errors might be useful logging?
				return
			}
			select {
			case m.conns <- conn:
			case <-m.done:
				return
			}
		}
	}()
}

// Accept waits for and returns the next connection from any registered listener.
func (m *MultiListener) Accept(ctx context.Context) (*quic.Conn, error) {
	select {
	case conn := <-m.conns:
		return conn, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-m.done:
		return nil, net.ErrClosed
	}
}

// Close closes all underlying listeners.
func (m *MultiListener) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	select {
	case <-m.done:
		// Already closed
		return nil
	default:
		close(m.done)
	}

	for _, l := range m.listeners {
		l.Close()
	}
	return nil
}

// Addr returns the address of the first listener, or nil.
func (m *MultiListener) Addr() net.Addr {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.listeners) > 0 {
		return m.listeners[0].Addr()
	}
	return &net.UDPAddr{IP: net.IP{0, 0, 0, 0}, Port: 0}
}
