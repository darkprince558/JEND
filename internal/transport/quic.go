package transport

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"

	"time"

	"github.com/quic-go/quic-go"
)

// Transport defines the interface for our networking layer
type Transport interface {
	Listen(port string) (*quic.Listener, error)
	Dial(addr string) (*quic.Conn, error)
}

// QUICTransport implements Transport using quic-go
type QUICTransport struct{}

// NewQUICTransport creates a new instance of QUICTransport
func NewQUICTransport() *QUICTransport {
	return &QUICTransport{}
}

// Listen starts a QUIC listener on the specified port
func (t *QUICTransport) Listen(port string) (*quic.Listener, error) {
	tlsConf, err := generateTLSConfig()
	if err != nil {
		return nil, err
	}

	quicConfig := &quic.Config{
		MaxIdleTimeout:     5 * time.Second,
		KeepAlivePeriod:    2 * time.Second,
		MaxIncomingStreams: 100,
	}

	listener, err := quic.ListenAddr(":"+port, tlsConf, quicConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %w", err)
	}
	return listener, nil
}

// Dial connects to a QUIC listener
func (t *QUICTransport) Dial(addr string) (*quic.Conn, error) {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true, // Self-signed certs for P2P
		NextProtos:         []string{"jend-protocol"},
	}

	conn, err := quic.DialAddr(context.Background(), addr, tlsConf, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %w", err)
	}
	return conn, nil
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
