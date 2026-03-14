package core

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/darkprince558/jend/internal/osutils"
	"github.com/darkprince558/jend/internal/config"
	"github.com/darkprince558/jend/internal/web"
)

var uploadMu sync.Mutex

// handleStatus returns a JSON summary of the server's upload state.
//
// Response format:
//
//	{"uploads": <count>, "remaining": <remaining>}
//
// "remaining" is -1 when no upload limit is configured.
func (s *QRUploadServer) handleStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	count := s.getUploadCount()

	remaining := -1
	if s.config.MaxUploads > 0 {
		remaining = s.config.MaxUploads - count
		if remaining < 0 {
			remaining = 0
		}
	}

	fmt.Fprintf(w, `{"uploads":%d,"remaining":%d}`, count, remaining)
}

// handleInfo returns system info like the target hostname
func (s *QRUploadServer) handleInfo(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "Laptop"
	} else {
		// Clean up the ".local" suffix common on Macs
		hostname = strings.TrimSuffix(hostname, ".local")
	}

	fmt.Fprintf(w, `{"target":"%s","version":"%s"}`, hostname, config.AppVersion)
}

// handleUpload processes multipart file uploads from the phone browser.
//
// It accepts POST requests with multipart/form-data containing files under
// the "files" key (or "file" as fallback). Each file is saved to OutputDir
// with automatic conflict resolution (appending " (1)", " (2)", etc.).
//
// Lifecycle callbacks are invoked for each file:
//  1. OnUploadStart — when a file begins saving
//  2. OnProgress — periodically as bytes are written
//  3. OnComplete — after the file is fully saved
//  4. OnLimitReached — if MaxUploads is hit after this batch
func (s *QRUploadServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Enforce upload limit before processing.
	if s.isUploadLimitReached() {
		http.Error(w, `{"error":"Upload limit reached"}`, http.StatusGone)
		return
	}

	// Parse multipart form — 128MB kept in memory, overflow goes to temp files.
	if err := r.ParseMultipartForm(128 << 20); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to parse upload: %v"}`, err), http.StatusBadRequest)
		return
	}

	files := extractUploadedFiles(r.MultipartForm)
	if len(files) == 0 {
		http.Error(w, `{"error":"No files provided"}`, http.StatusBadRequest)
		return
	}

	// Save each file to the output directory.
	savedFiles, err := s.saveAllFiles(files)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Update the upload count and fire callbacks.
	count := s.incrementUploadCount(len(savedFiles))

	for _, name := range savedFiles {
		if s.config.OnComplete != nil {
			s.config.OnComplete(name, count)
		}
	}

	if s.config.MaxUploads > 0 && count >= s.config.MaxUploads && s.config.OnLimitReached != nil {
		go s.config.OnLimitReached()
	}

	// Respond with JSON success.
	writeUploadResponse(w, savedFiles, count)
}

// handlePage serves the mobile-friendly HTML upload page.
func (s *QRUploadServer) handlePage(w http.ResponseWriter, r *http.Request) {
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

// textUploadRequest describes the expected JSON body for text uploads.
type textUploadRequest struct {
	Text string `json:"text"`
}

// handleTextUpload processes incoming raw text or URL snippets from the browser.
func (s *QRUploadServer) handleTextUpload(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req textUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if req.Text != "" && s.config.OnTextComplete != nil {
		s.config.OnTextComplete(req.Text)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"ok":true}`)
}

// ── Helper Functions ──────────────────────────────────────────────────────────

// setCORSHeaders adds the CORS headers needed for cross-origin XHR uploads
// from the phone browser page.
func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// extractUploadedFiles retrieves file headers from the multipart form.
// It checks both "files" (multi-file) and "file" (single-file) form keys.
func extractUploadedFiles(form *multipart.Form) []*multipart.FileHeader {
	files := form.File["files"]
	if len(files) == 0 {
		files = form.File["file"]
	}
	return files
}

// saveAllFiles saves each uploaded file to the output directory.
// Returns the list of saved filenames, or an error if any file fails.
func (s *QRUploadServer) saveAllFiles(files []*multipart.FileHeader) ([]string, error) {
	var savedFiles []string

	for _, fh := range files {
		// Ask for user approval before saving the file (if configured)
		if s.config.OnApprovalRequired != nil {
			if !s.config.OnApprovalRequired(fh.Filename, fh.Size) {
				continue // Skip processing rejected file
			}
		}

		name, err := s.saveOneFile(fh)
		if err != nil {
			return savedFiles, err
		}
		savedFiles = append(savedFiles, name)
	}

	return savedFiles, nil
}

// saveOneFile saves a single uploaded file to OutputDir, streaming it with
// progress callbacks. Returns the saved filename (base name only).
func (s *QRUploadServer) saveOneFile(fh *multipart.FileHeader) (string, error) {
	// Sanitize filename to prevent directory traversal.
	name := sanitizeFilename(fh.Filename)

	// Choose a non-conflicting path (appends " (1)", " (2)" etc. if needed).
	uploadMu.Lock()
	destPath := resolveNonConflictingPath(s.config.OutputDir, name)
	dst, err := os.Create(destPath)
	uploadMu.Unlock()

	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer dst.Close()

	if s.config.OnUploadStart != nil {
		s.config.OnUploadStart(filepath.Base(destPath))
	}

	src, err := fh.Open()
	if err != nil {
		dst.Close()
		os.Remove(destPath)
		return "", fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	// Stream the file to disk with progress tracking.
	if err := streamWithProgress(src, dst, fh.Size, s.config.OnProgress); err != nil {
		return "", err
	}

	return filepath.Base(destPath), nil
}

// sanitizeFilename extracts a safe base filename, falling back to "upload"
// for empty or path-only filenames. Dangerous extensions are quarantined.
func sanitizeFilename(raw string) string {
	name := filepath.Base(raw)
	if name == "" || name == "." || name == "/" {
		return "upload"
	}

	if osutils.IsDangerousExtension(name) {
		name += ".jend-quarantine"
	}

	return name
}

// resolveNonConflictingPath returns a file path in dir that doesn't conflict
// with any existing file. If "photo.jpg" exists, it returns "photo (1).jpg", etc.
func resolveNonConflictingPath(dir, name string) string {
	dest := filepath.Join(dir, name)
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return dest
	}

	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// streamWithProgress copies bytes from src to dst in 64KB chunks,
// calling onProgress after each chunk if non-nil.
func streamWithProgress(src io.Reader, dst io.Writer, total int64, onProgress func(recv, total int64)) error {
	buf := make([]byte, 64*1024)
	var recv int64

	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("failed to write file: %w", writeErr)
			}
			recv += int64(n)
			if onProgress != nil {
				onProgress(recv, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read error: %w", readErr)
		}
	}

	return nil
}

// writeUploadResponse writes a JSON success response with the list of saved files.
func writeUploadResponse(w http.ResponseWriter, savedFiles []string, totalUploads int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	names := make([]string, len(savedFiles))
	for i, n := range savedFiles {
		names[i] = `"` + n + `"`
	}
	fmt.Fprintf(w, `{"ok":true,"files":[%s],"total_uploads":%d}`, strings.Join(names, ","), totalUploads)
}
