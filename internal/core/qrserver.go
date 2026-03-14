package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/darkprince558/jend/internal/web"
	"github.com/darkprince558/jend/internal/config"
	"github.com/google/uuid"
)

// QRServerConfig holds the configuration for the QR download server.
type QRServerConfig struct {
	FilePath       string
	FileName       string
	FileSize       int64
	FileHash       string
	IsText         bool
	TextContent    string
	Token          string
	Port           int
	MaxDownloads   int           // 0 = unlimited
	ExpireAfter    time.Duration // 0 = never
	OnProgress     func(sent, total int64)
	OnComplete     func(downloadCount int)
	OnLimitReached func() // called when MaxDownloads is reached
	OnExpire       func() // called when ExpireAfter fires
}

// QRServer is a lightweight HTTP server that serves a single file for download.
type QRServer struct {
	config        QRServerConfig
	server        *http.Server
	mu            sync.Mutex
	downloadCount int
}

// NewQRServer creates a new QR download server.
func NewQRServer(cfg QRServerConfig) *QRServer {
	if cfg.Token == "" {
		cfg.Token = uuid.New().String()[:12]
	}
	if cfg.Port == 0 {
		cfg.Port = 8888
	}
	return &QRServer{config: cfg}
}

// Start begins serving on the configured port. It blocks until the server shuts down.
func (s *QRServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	prefix := fmt.Sprintf("/d/%s", s.config.Token)
	
	// Handle the exact prefix without trailing slash AND the one with trailing slash
	mux.HandleFunc(prefix, s.handlePage)
	mux.HandleFunc(prefix+"/", s.handlePage)
	
	mux.HandleFunc(prefix+"/download", s.handleDownload)
	mux.HandleFunc(prefix+"/info", s.handleInfo)

	// Serve React UI assets
	fsys, err := fs.Sub(web.Content, "dist")
	if err != nil {
		return fmt.Errorf("failed to load web assets: %w", err)
	}
	mux.Handle(prefix+"/assets/", http.StripPrefix(prefix+"/", http.FileServer(http.FS(fsys))))
	mux.Handle(prefix+"/logo-arrow.png", http.StripPrefix(prefix+"/", http.FileServer(http.FS(fsys))))
	mux.Handle(prefix+"/vite.svg", http.StripPrefix(prefix+"/", http.FileServer(http.FS(fsys))))


	addr := fmt.Sprintf(":%d", s.config.Port)
	s.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Graceful shutdown on context cancel
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdownCtx)
	}()

	// Auto-expire timer
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
				// Server already shutting down
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

// URL returns the full download URL for this server (IPv4).
func (s *QRServer) URL(localIP string) string {
	return fmt.Sprintf("http://%s:%d/d/%s", localIP, s.config.Port, s.config.Token)
}

// URLv6 returns the full download URL using an IPv6 link-local address.
// The zone ID (interface name) is required for link-local addresses.
func (s *QRServer) URLv6(ipv6 string, zone string) string {
	// Browsers require the zone ID to be URL-encoded as %25 instead of %
	return fmt.Sprintf("http://[%s%%25%s]:%d/d/%s", ipv6, zone, s.config.Port, s.config.Token)
}

// URLs returns both IPv4 and IPv6 URLs for the QR code display.
func (s *QRServer) URLs() (ipv4URL, ipv6URL string) {
	addrs := GetLocalAddresses()
	if addrs.IPv4 != "" {
		ipv4URL = s.URL(addrs.IPv4)
	}
	if addrs.IPv6 != "" && addrs.IPv6Zone != "" {
		ipv6URL = s.URLv6(addrs.IPv6, addrs.IPv6Zone)
	}
	return
}

func (s *QRServer) handleInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	fileType := "file"
	if s.config.IsText {
		fileType = "text"
	} else if ext := filepath.Ext(s.config.FileName); ext != "" {
		fileType = strings.TrimPrefix(ext, ".") + " file"
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "Laptop"
	} else {
		// Clean up the ".local" suffix common on Macs
		hostname = strings.TrimSuffix(hostname, ".local")
	}

	fmt.Fprintf(w, `{"name":"%s","size":%d,"type":"%s","hash":"%s","target":"%s","version":"%s"}`,
		s.config.FileName, s.config.FileSize, fileType, s.config.FileHash, hostname, config.AppVersion)
}

func (s *QRServer) handleDownload(w http.ResponseWriter, r *http.Request) {
	// Enforce download limit
	if s.config.MaxDownloads > 0 {
		s.mu.Lock()
		if s.downloadCount >= s.config.MaxDownloads {
			s.mu.Unlock()
			http.Error(w, "Download limit reached", http.StatusGone)
			return
		}
		s.mu.Unlock()
	}

	if s.config.IsText {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, "jend-text.txt"))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(s.config.TextContent)))
		_, _ = w.Write([]byte(s.config.TextContent))

		s.mu.Lock()
		s.downloadCount++
		count := s.downloadCount
		s.mu.Unlock()
		if s.config.OnComplete != nil {
			go s.config.OnComplete(count)
		}
		if s.config.MaxDownloads > 0 && count >= s.config.MaxDownloads && s.config.OnLimitReached != nil {
			go s.config.OnLimitReached()
		}
		return
	}

	f, err := os.Open(s.config.FilePath)
	if err != nil {
		http.Error(w, "File not found", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	// Detect content type
	contentType := mime.TypeByExtension(filepath.Ext(s.config.FileName))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, s.config.FileName))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", s.config.FileSize))

	// Stream with progress tracking
	buf := make([]byte, 64*1024)
	var sent int64
	for {
		n, err := f.Read(buf)
		if n > 0 {
			_, writeErr := w.Write(buf[:n])
			if writeErr != nil {
				return
			}
			sent += int64(n)
			if s.config.OnProgress != nil {
				s.config.OnProgress(sent, s.config.FileSize)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return
		}
	}

	s.mu.Lock()
	s.downloadCount++
	count := s.downloadCount
	s.mu.Unlock()
	if s.config.OnComplete != nil {
		go s.config.OnComplete(count)
	}
	if s.config.MaxDownloads > 0 && count >= s.config.MaxDownloads && s.config.OnLimitReached != nil {
		go s.config.OnLimitReached()
	}
}
func (s *QRServer) handlePage(w http.ResponseWriter, r *http.Request) {
	// Require trailing slash so React relative paths "./assets/..." resolve correctly
	if !strings.HasSuffix(r.URL.Path, "/") {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
		return
	}

	// Serve the React index.html
	indexHTML, err := fs.ReadFile(web.Content, "dist/index.html")
	if err != nil {
		http.Error(w, "Failed to load page", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}

// HashFile computes the SHA-256 hash of a file.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func formatBytesQR(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	suffixes := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), suffixes[exp])
}
