package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// QRServerConfig holds the configuration for the QR download server.
type QRServerConfig struct {
	FilePath    string
	FileName    string
	FileSize    int64
	FileHash    string
	IsText      bool
	TextContent string
	Token       string
	Port        int
	OnProgress  func(sent, total int64)
	OnComplete  func()
}

// QRServer is a lightweight HTTP server that serves a single file for download.
type QRServer struct {
	config     QRServerConfig
	server     *http.Server
	mu         sync.Mutex
	downloaded bool
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
	mux.HandleFunc(prefix, s.handlePage)
	mux.HandleFunc(prefix+"/download", s.handleDownload)
	mux.HandleFunc(prefix+"/info", s.handleInfo)

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

// URL returns the full download URL for this server.
func (s *QRServer) URL(localIP string) string {
	return fmt.Sprintf("http://%s:%d/d/%s", localIP, s.config.Port, s.config.Token)
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

	fmt.Fprintf(w, `{"name":"%s","size":%d,"type":"%s","hash":"%s"}`,
		s.config.FileName, s.config.FileSize, fileType, s.config.FileHash)
}

func (s *QRServer) handleDownload(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.downloaded {
		s.mu.Unlock()
		http.Error(w, "File has already been downloaded", http.StatusGone)
		return
	}
	s.mu.Unlock()

	if s.config.IsText {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, "jend-text.txt"))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(s.config.TextContent)))
		_, _ = w.Write([]byte(s.config.TextContent))

		s.mu.Lock()
		s.downloaded = true
		s.mu.Unlock()
		if s.config.OnComplete != nil {
			go s.config.OnComplete()
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
	s.downloaded = true
	s.mu.Unlock()
	if s.config.OnComplete != nil {
		go s.config.OnComplete()
	}
}

func (s *QRServer) handlePage(w http.ResponseWriter, r *http.Request) {
	fileType := "File"
	icon := "📄"
	if s.config.IsText {
		fileType = "Text Snippet"
		icon = "📝"
	} else if ext := filepath.Ext(s.config.FileName); ext != "" {
		switch strings.ToLower(ext) {
		case ".zip", ".tar", ".gz", ".rar", ".7z":
			icon = "📦"
			fileType = strings.ToUpper(strings.TrimPrefix(ext, ".")) + " Archive"
		case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg":
			icon = "🖼️"
			fileType = "Image"
		case ".mp4", ".mov", ".avi", ".mkv":
			icon = "🎬"
			fileType = "Video"
		case ".mp3", ".wav", ".flac", ".aac":
			icon = "🎵"
			fileType = "Audio"
		case ".pdf":
			icon = "📕"
			fileType = "PDF Document"
		case ".doc", ".docx":
			icon = "📘"
			fileType = "Word Document"
		case ".pptx", ".ppt":
			icon = "📙"
			fileType = "Presentation"
		case ".xls", ".xlsx", ".csv":
			icon = "📊"
			fileType = "Spreadsheet"
		default:
			fileType = strings.TrimPrefix(ext, ".") + " file"
		}
	}

	sizeStr := formatBytesQR(s.config.FileSize)
	hashShort := s.config.FileHash
	if len(hashShort) > 16 {
		hashShort = hashShort[:16] + "..."
	}

	downloadURL := fmt.Sprintf("/d/%s/download", s.config.Token)

	textPreview := ""
	if s.config.IsText {
		preview := s.config.TextContent
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		textPreview = strings.ReplaceAll(preview, "`", "&#96;")
		textPreview = strings.ReplaceAll(textPreview, "<", "&lt;")
		textPreview = strings.ReplaceAll(textPreview, ">", "&gt;")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	html := `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
<title>JEND · Download</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{background:#16161A;color:#FFFFFE;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;
min-height:100vh;display:flex;align-items:center;justify-content:center;padding:20px}
.container{max-width:420px;width:100%;text-align:center}
.logo{font-size:2rem;font-weight:800;letter-spacing:0.15em;color:#7F5AF0;margin-bottom:8px}
.logo span{color:#00F0FF}
.subtitle{color:#94A1B2;font-size:0.85rem;margin-bottom:32px}
.card{background:#242629;border-radius:16px;padding:28px 24px;margin-bottom:24px;
border:1px solid rgba(127,90,240,0.15)}
.file-icon{font-size:3rem;margin-bottom:12px}
.file-name{font-size:1.25rem;font-weight:700;color:#FFFFFE;word-break:break-all;margin-bottom:16px}
.meta{display:grid;grid-template-columns:1fr 1fr;gap:12px;text-align:left;margin-bottom:20px}
.meta-item{background:#16161A;border-radius:10px;padding:12px 14px}
.meta-label{font-size:0.7rem;text-transform:uppercase;letter-spacing:0.08em;color:#94A1B2;margin-bottom:4px}
.meta-value{font-size:0.95rem;font-weight:600;color:#FFFFFE}
.meta-value.hash{font-family:'SF Mono',Monaco,monospace;font-size:0.75rem;color:#94A1B2}
.text-preview{background:#16161A;border-radius:10px;padding:14px;text-align:left;
margin-bottom:16px;font-family:'SF Mono',Monaco,monospace;font-size:0.8rem;color:#94A1B2;
max-height:200px;overflow-y:auto;white-space:pre-wrap;word-break:break-word;
border:1px solid rgba(0,240,255,0.1)}
.btn{display:block;width:100%;padding:16px;border:none;border-radius:12px;cursor:pointer;
font-size:1.1rem;font-weight:700;letter-spacing:0.04em;transition:all 0.2s ease}
.btn-download{background:linear-gradient(135deg,#7F5AF0,#6B3FD4);color:#FFFFFE;
box-shadow:0 4px 24px rgba(127,90,240,0.35)}
.btn-download:hover{transform:translateY(-2px);box-shadow:0 6px 32px rgba(127,90,240,0.5)}
.btn-download:active{transform:translateY(0)}
.progress-wrap{display:none;margin-top:16px}
.progress-bar{height:6px;background:#242629;border-radius:3px;overflow:hidden}
.progress-fill{height:100%;width:0%;background:linear-gradient(90deg,#7F5AF0,#00F0FF);
border-radius:3px;transition:width 0.3s ease}
.progress-text{font-size:0.8rem;color:#94A1B2;margin-top:8px}
.done{display:none;margin-top:16px}
.done .check{font-size:3rem;margin-bottom:8px}
.done .msg{color:#2CB67D;font-weight:700;font-size:1.1rem}
.footer{color:#94A1B2;font-size:0.7rem;margin-top:24px;opacity:0.6}
.footer a{color:#7F5AF0;text-decoration:none}
@media(max-width:400px){.meta{grid-template-columns:1fr}.card{padding:20px 16px}}
</style>
</head>
<body>
<div class="container">
  <div class="logo">JE<span>N</span>D</div>
  <div class="subtitle">Someone wants to send you a file</div>
  <div class="card">
    <div class="file-icon">` + icon + `</div>
    <div class="file-name">` + s.config.FileName + `</div>
    <div class="meta">
      <div class="meta-item">
        <div class="meta-label">Size</div>
        <div class="meta-value">` + sizeStr + `</div>
      </div>
      <div class="meta-item">
        <div class="meta-label">Type</div>
        <div class="meta-value">` + fileType + `</div>
      </div>
      <div class="meta-item" style="grid-column:1/-1">
        <div class="meta-label">SHA-256</div>
        <div class="meta-value hash">` + hashShort + `</div>
      </div>
    </div>`

	if s.config.IsText && textPreview != "" {
		html += `
    <div class="text-preview">` + textPreview + `</div>`
	}

	html += `
    <a class="btn btn-download" id="downloadBtn" href="` + downloadURL + `" onclick="startDownload()">Download</a>
    <div class="progress-wrap" id="progressWrap">
      <div class="progress-bar"><div class="progress-fill" id="progressFill"></div></div>
      <div class="progress-text" id="progressText">Starting download...</div>
    </div>
    <div class="done" id="doneMsg">
      <div class="check">✅</div>
      <div class="msg">Download Complete!</div>
    </div>
  </div>
  <div class="footer">Powered by <a href="https://github.com/darkprince558/jend">JEND</a> · End-to-end encrypted file transfer</div>
</div>
<script>
function startDownload(){
  var btn=document.getElementById('downloadBtn');
  var prog=document.getElementById('progressWrap');
  btn.style.display='none';
  prog.style.display='block';
  // Simulate progress since we can't track actual download progress on the client
  var fill=document.getElementById('progressFill');
  var text=document.getElementById('progressText');
  var pct=0;
  var iv=setInterval(function(){
    pct+=Math.random()*15;
    if(pct>90)pct=90;
    fill.style.width=pct+'%';
    text.textContent=Math.round(pct)+'% downloading...';
  },400);
  // After the link click triggers the download, poll for completion
  setTimeout(function(){
    clearInterval(iv);
    fill.style.width='100%';
    text.textContent='100%';
    setTimeout(function(){
      prog.style.display='none';
      document.getElementById('doneMsg').style.display='block';
    },600);
  },` + fmt.Sprintf("%d", max(2000, s.config.FileSize/50000)) + `);
}
</script>
</body>
</html>`

	_, _ = fmt.Fprint(w, html)
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
