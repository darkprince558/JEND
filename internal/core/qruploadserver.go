// Package core provides the QR upload server for receiving files from a phone browser.
//
// The QR upload server is started by `jend receive --qr`. It serves a mobile-friendly
// HTML page over local WiFi where users can pick photos/files to upload directly to
// the laptop. No app is needed on the phone — just a browser.
//
// Architecture:
//   - qruploadserver.go    — Server struct, config, lifecycle (Start/Stop/URLs)
//   - qrupload_handlers.go — HTTP request handlers (upload, status, page)
//   - qrupload_page.go     — Inline HTML/CSS/JS template for the mobile upload page
package core

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// QRUploadServerConfig holds the configuration for the QR upload server.
//
// Callbacks are invoked during the upload lifecycle to provide terminal feedback.
// All callbacks are optional — nil callbacks are safely skipped.
type QRUploadServerConfig struct {
	// OutputDir is the directory where uploaded files are saved.
	OutputDir string

	// Port is the HTTP port to listen on. Defaults to 8888 if zero.
	Port int

	// MaxUploads limits the total number of file uploads accepted.
	// Zero means unlimited. When the limit is reached, OnLimitReached is called
	// and the server shuts down after a brief delay.
	MaxUploads int

	// ExpireAfter auto-expires the server after the given duration.
	// Zero means the server runs until manually cancelled.
	ExpireAfter time.Duration

	// OnUploadStart is called when a new file upload begins.
	OnUploadStart func(filename string)

	// OnProgress is called periodically during a file upload with bytes received so far.
	OnProgress func(recv, total int64)

	// OnComplete is called after each file is fully saved.
	OnComplete func(name string, fileCount int)

	// OnLimitReached is called when MaxUploads is reached.
	OnLimitReached func()

	// OnApprovalRequired asks the user whether to accept a file. If it returns false, the file is skipped.
	OnApprovalRequired func(name string, size int64) bool

	// OnExpire is called when ExpireAfter fires.
	OnExpire func()
}

// QRUploadServer is a lightweight HTTP server that accepts file uploads
// from a phone browser via a QR-code-accessible URL.
//
// It serves three endpoints under /u/{token}:
//   - GET  /u/{token}         — the mobile upload HTML page
//   - POST /u/{token}/upload  — multipart file upload endpoint
//   - GET  /u/{token}/status  — JSON status (upload count, remaining)
type QRUploadServer struct {
	config      QRUploadServerConfig
	server      *http.Server
	token       string
	mu          sync.Mutex
	uploadCount int
}

// NewQRUploadServer creates a new QR upload server with the given config.
// A unique URL token is generated automatically.
func NewQRUploadServer(cfg QRUploadServerConfig) *QRUploadServer {
	if cfg.Port == 0 {
		cfg.Port = 8888
	}
	return &QRUploadServer{
		config: cfg,
		token:  uuid.New().String()[:12],
	}
}

// Token returns the server's unique URL path token.
func (s *QRUploadServer) Token() string {
	return s.token
}

// Start begins serving HTTP on the configured port. It blocks until the context
// is cancelled, the server expires, or the upload limit is reached.
//
// The server registers three routes:
//   - GET  /u/{token}         → handlePage (mobile upload UI)
//   - POST /u/{token}/upload  → handleUpload (file upload endpoint)
//   - GET  /u/{token}/status  → handleStatus (JSON status)
func (s *QRUploadServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	prefix := fmt.Sprintf("/u/%s", s.token)
	mux.HandleFunc(prefix, s.handlePage)
	mux.HandleFunc(prefix+"/upload", s.handleUpload)
	mux.HandleFunc(prefix+"/status", s.handleStatus)

	addr := fmt.Sprintf(":%d", s.config.Port)
	s.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Graceful shutdown when the context is cancelled.
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdownCtx)
	}()

	// Auto-expire timer: shuts down the server after ExpireAfter.
	if s.config.ExpireAfter > 0 {
		go func() {
			select {
			case <-time.After(s.config.ExpireAfter):
				if s.config.OnExpire != nil {
					s.config.OnExpire()
				}
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				_ = s.server.Shutdown(shutdownCtx)
			case <-ctx.Done():
				// Server already shutting down via context.
			}
		}()
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	err = s.server.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// URLs returns the IPv4 and IPv6 URLs for QR code display.
// Either may be empty if the corresponding address is unavailable.
func (s *QRUploadServer) URLs() (ipv4URL, ipv6URL string) {
	addrs := GetLocalAddresses()
	if addrs.IPv4 != "" {
		ipv4URL = fmt.Sprintf("http://%s:%d/u/%s", addrs.IPv4, s.config.Port, s.token)
	}
	if addrs.IPv6 != "" && addrs.IPv6Zone != "" {
		ipv6URL = fmt.Sprintf("http://[%s%%25%s]:%d/u/%s", addrs.IPv6, addrs.IPv6Zone, s.config.Port, s.token)
	}
	return
}

// incrementUploadCount atomically increments the upload count by n
// and returns the new total.
func (s *QRUploadServer) incrementUploadCount(n int) int {
	s.mu.Lock()
	s.uploadCount += n
	count := s.uploadCount
	s.mu.Unlock()
	return count
}

// isUploadLimitReached checks whether the upload limit has been hit.
// Always returns false when MaxUploads is 0 (unlimited).
func (s *QRUploadServer) isUploadLimitReached() bool {
	if s.config.MaxUploads <= 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.uploadCount >= s.config.MaxUploads
}

// getUploadCount returns the current upload count.
func (s *QRUploadServer) getUploadCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.uploadCount
}
