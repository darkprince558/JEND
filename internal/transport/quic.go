package transport

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"

	"time"

	"github.com/quic-go/quic-go"
)

// Transport defines the interface for our networking layer
type Transport interface {
	Listen(port string) (QUICListener, error)
	Dial(addr string) (*quic.Conn, error)
	ListenPacket(conn net.PacketConn) (QUICListener, error)
	DialPacket(conn net.PacketConn, addr net.Addr) (*quic.Conn, error)
}

// QUICListener abstracts *quic.Listener to allow MultiListener implementation
type QUICListener interface {
	Accept(context.Context) (*quic.Conn, error)
	Close() error
	Addr() net.Addr
}

// QUICTransport implements Transport using quic-go
type QUICTransport struct{}

// NewQUICTransport creates a new instance of QUICTransport
func NewQUICTransport() *QUICTransport {
	return &QUICTransport{}
}

// Listen starts a QUIC listener on the specified port.
// It creates a UDP PacketConn internally.
func (t *QUICTransport) Listen(port string) (QUICListener, error) {
	tlsConf, err := generateTLSConfig()
	if err != nil {
		return nil, err
	}
	quicConfig := getQuicConfig()
	return quic.ListenAddr(":"+port, tlsConf, quicConfig)
}

// ListenPacket starts a QUIC listener on an existing PacketConn (e.g. from ICE).
func (t *QUICTransport) ListenPacket(conn net.PacketConn) (QUICListener, error) {
	tlsConf, err := generateTLSConfig()
	if err != nil {
		return nil, err
	}
	quicConfig := getQuicConfig()
	return quic.Listen(conn, tlsConf, quicConfig)
}

func getQuicConfig() *quic.Config {
	return &quic.Config{
		MaxIdleTimeout:     10 * time.Second, // Increased timeout for P2P stability
		KeepAlivePeriod:    2 * time.Second,
		MaxIncomingStreams: 100,
	}
}

// Dial connects to a QUIC listener.
func (t *QUICTransport) Dial(addr string) (*quic.Conn, error) {
	tlsConf := getTLSConfig()
	return quic.DialAddr(context.Background(), addr, tlsConf, nil)
}

// DialPacket connects via an existing PacketConn (e.g. ICE).
// The addr arg is technically unused for routing if conn is bound, but required by API.
func (t *QUICTransport) DialPacket(conn net.PacketConn, addr net.Addr) (*quic.Conn, error) {
	tlsConf := getTLSConfig()
	return quic.Dial(context.Background(), conn, addr, tlsConf, nil)
}

func getTLSConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, // Self-signed certs for P2P
		NextProtos:         []string{"jend-protocol"},
	}
}

// generateTLSConfig generates a self-signed certificate for QUIC
func generateTLSConfig() (*tls.Config, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"jend-protocol"},
	}, nil
}
