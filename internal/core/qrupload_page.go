package core

import (
	"fmt"
	"time"
)

// buildUploadPageHTML generates the complete HTML page served to phone browsers.
//
// The page is self-contained (no external CSS/JS dependencies) so it works on
// any device that can reach the server over local WiFi. It includes:
//   - JEND branding (ASCII logo, dark theme)
//   - Tap-to-browse file picker with multi-file support
//   - Camera capture button (uses the "capture" attribute for mobile)
//   - Drag-and-drop zone
//   - Per-file upload progress
//   - "Upload Complete" confirmation with "Send More" option
//
// The maxUploads and expireAfter parameters add informational badges to the page.
func buildUploadPageHTML(maxUploads int, expireAfter time.Duration) string {
	metaHTML := buildMetaInfoHTML(maxUploads, expireAfter)
	return uploadPageHead() + uploadPageBody(metaHTML) + uploadPageScript()
}

// buildMetaInfoHTML generates optional HTML badges for upload limit and expiry.
func buildMetaInfoHTML(maxUploads int, expireAfter time.Duration) string {
	html := ""
	if maxUploads > 0 {
		html += fmt.Sprintf(
			`<div class="meta-item"><div class="meta-label">Limit</div>`+
				`<div class="meta-value">%d uploads</div></div>`, maxUploads)
	}
	if expireAfter > 0 {
		html += fmt.Sprintf(
			`<div class="meta-item"><div class="meta-label">Expires</div>`+
				`<div class="meta-value">%s</div></div>`, expireAfter.String())
	}
	return html
}

// uploadPageHead returns the HTML <head> block including all CSS styles.
func uploadPageHead() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
<title>JEND · Upload to Laptop</title>
<style>
/* ── Reset & Base ── */
*{margin:0;padding:0;box-sizing:border-box}
body{background:#16161A;color:#FFFFFE;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;
min-height:100vh;display:flex;align-items:center;justify-content:center;padding:20px}
.container{max-width:420px;width:100%;text-align:center}
.logo{font-family:'Courier New',Courier,monospace;font-size:0.55rem;line-height:1.15;
color:#7F5AF0;margin-bottom:16px;white-space:pre;text-align:left;display:inline-block}
.subtitle{color:#94A1B2;font-size:0.85rem;margin-bottom:28px}

/* ── Card & Meta ── */
.card{background:#242629;border-radius:16px;padding:28px 24px;margin-bottom:24px;
border:1px solid rgba(127,90,240,0.15)}
.meta{display:flex;gap:12px;justify-content:center;flex-wrap:wrap;margin-bottom:20px}
.meta-item{background:#16161A;border-radius:10px;padding:12px 14px;min-width:100px}
.meta-label{font-size:0.7rem;text-transform:uppercase;letter-spacing:0.08em;color:#94A1B2;margin-bottom:4px}
.meta-value{font-size:0.95rem;font-weight:600;color:#FFFFFE}

/* ── Drop Zone ── */
.drop-zone{border:2px dashed rgba(127,90,240,0.4);border-radius:16px;padding:40px 20px;
cursor:pointer;transition:all 0.3s cubic-bezier(0.4,0,0.2,1);margin-bottom:20px;position:relative}
.drop-zone:hover,.drop-zone.dragover{border-color:#7F5AF0;background:rgba(127,90,240,0.05);
transform:scale(1.02)}
.drop-zone.dragover{background:rgba(127,90,240,0.1)}
.drop-icon{font-size:48px;margin-bottom:12px;opacity:0.7}
.drop-title{font-size:1.1rem;font-weight:600;margin-bottom:6px}
.drop-hint{font-size:0.85rem;color:#94A1B2}

/* ── Buttons ── */
.btn{display:block;width:100%;padding:16px;border:none;border-radius:12px;cursor:pointer;
font-size:1.1rem;font-weight:700;letter-spacing:0.04em;transition:all 0.2s ease;
text-decoration:none;color:#FFFFFE;margin-bottom:12px;
background:linear-gradient(135deg,#7F5AF0,#6B3FD4);
box-shadow:0 4px 24px rgba(127,90,240,0.35);}

.btn:hover{transform:translateY(-2px);box-shadow:0 6px 32px rgba(127,90,240,0.5)}
.btn:active{transform:translateY(0)}
.btn:disabled{opacity:0.4;cursor:not-allowed;transform:none;box-shadow:none}

.btn-more{background:rgba(127,90,240,0.15);color:#7F5AF0;font-size:0.9rem;
padding:12px;border:1px solid rgba(127,90,240,0.3);box-shadow:none;margin-bottom:0}
.btn-more:hover{background:rgba(127,90,240,0.25);transform:translateY(-1px);box-shadow:none}

/* ── File List ── */
.file-list{text-align:left;margin:16px 0;max-height:200px;overflow-y:auto}
.file-item{display:flex;align-items:center;justify-content:space-between;
padding:10px 14px;background:#16161A;border-radius:10px;margin-bottom:6px;
border:1px solid rgba(255,255,255,0.05)}
.file-item-name{font-size:0.85rem;font-weight:500;white-space:nowrap;overflow:hidden;
text-overflow:ellipsis;max-width:200px}
.file-item-size{font-size:0.75rem;color:#94A1B2;white-space:nowrap;margin-left:8px}
.file-item-remove{color:#94A1B2;cursor:pointer;font-size:1.2rem;margin-left:8px;
width:24px;height:24px;display:flex;align-items:center;justify-content:center;
border-radius:50%;transition:all 0.2s}
.file-item-remove:hover{background:rgba(255,100,100,0.15);color:#ff6b6b}

/* ── Text Input ── */
.text-zone{margin-top:24px}
.text-area{width:100%;background:#16161A;border:1px solid rgba(127,90,240,0.3);border-radius:12px;padding:12px;
color:#FFFFFE;font-family:inherit;font-size:0.95rem;resize:vertical;min-height:80px;margin-bottom:12px;
transition:border-color 0.2s}
.text-area:focus{outline:none;border-color:#7F5AF0}

/* ── Progress ── */
.progress-wrap{display:none;margin-top:20px}
.progress-bar{height:6px;background:#1a1a1e;border-radius:3px;overflow:hidden}
.progress-fill{height:100%;width:0%;background:linear-gradient(90deg,#7F5AF0,#00F0FF);
border-radius:3px;transition:width 0.2s ease}
.progress-text{font-size:0.8rem;color:#94A1B2;margin-top:8px}
.upload-item{display:flex;align-items:center;gap:10px;padding:10px 14px;
background:#16161A;border-radius:10px;margin-bottom:6px;border:1px solid rgba(255,255,255,0.05)}
.upload-item-name{font-size:0.8rem;flex:1;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.upload-item-status{font-size:0.75rem;color:#94A1B2;white-space:nowrap}
.upload-item-status.done{color:#2CB67D}
.upload-item-status.error{color:#ff6b6b}

/* ── Done ── */
.done-wrap{display:none;margin-top:20px;padding:20px;background:rgba(44,182,125,0.08);
border-radius:12px;border:1px solid rgba(44,182,125,0.2);text-align:center}
.done-icon{font-size:48px;margin-bottom:8px}
.done-msg{color:#2CB67D;font-weight:700;font-size:1rem}
.done-detail{color:#94A1B2;font-size:0.85rem;margin-top:6px}

/* ── Footer & Misc ── */
.footer{color:#94A1B2;font-size:0.7rem;margin-top:24px;opacity:0.6}
.footer a{color:#7F5AF0;text-decoration:none}
.hidden{display:none}

@media(max-width:400px){.card{padding:20px 16px}.logo{font-size:0.45rem}.drop-zone{padding:30px 16px}}
</style>
</head>`
}

// uploadPageBody returns the HTML <body> block with the upload UI structure.
// metaHTML is injected into the status bar area for limit/expire badges.
func uploadPageBody(metaHTML string) string {
	return `
<body>
<div class="container">
  <pre class="logo">     ██╗███████╗███╗   ██╗██████╗ 
     ██║██╔════╝████╗  ██║██╔══██╗
     ██║█████╗  ██╔██╗ ██║██║  ██║
██   ██║██╔══╝  ██║╚██╗██║██║  ██║
╚█████╔╝███████╗██║ ╚████║██████╔╝
 ╚════╝ ╚══════╝╚═╝  ╚═══╝╚═════╝ </pre>
  <div class="subtitle">Send files to your laptop</div>
  <div class="card">
    <div class="meta">
      <div class="meta-item">
        <div class="meta-label">Status</div>
        <div class="meta-value" id="statusText">Ready</div>
      </div>` + metaHTML + `
    </div>

    <!-- Upload Area: file picker, camera, and file list -->
    <div id="uploadArea">
      <div class="drop-zone" id="dropZone" onclick="document.getElementById('fileInput').click()">
        <div class="drop-icon">📁</div>
        <div class="drop-title">Tap to select files</div>
        <div class="drop-hint">or drag & drop here</div>
      </div>

      <button class="btn" onclick="document.getElementById('cameraInput').click()">
        📷 Take Photo
      </button>

      <input type="file" id="fileInput" class="hidden" multiple accept="*/*,image/heic,image/heif">
      <input type="file" id="cameraInput" class="hidden" accept="image/*,image/heic,image/heif" capture="environment">

      <div class="file-list" id="fileList"></div>

      <button class="btn" id="uploadBtn" disabled onclick="startUpload()">
        Upload Files
      </button>

      <div class="text-zone">
        <textarea id="textInput" class="text-area" placeholder="Or paste text / URL here..."></textarea>
      </div>

      <button class="btn" id="sendTextBtn" onclick="sendText()">
        Send Text
      </button>
    </div>

    <!-- Progress Area: shown during upload -->
    <div id="progressArea" style="display:none">
      <div class="progress-wrap" style="display:block">
        <div class="progress-bar"><div class="progress-fill" id="progressFill"></div></div>
        <div class="progress-text" id="progressText">Uploading...</div>
      </div>
      <div id="uploadList" style="margin-top:16px"></div>
    </div>

    <!-- Done Area: shown after successful upload -->
    <div class="done-wrap" id="doneWrap">
      <div class="done-icon">✅</div>
      <div class="done-msg" id="doneMsg">Upload Complete!</div>
      <div class="done-detail" id="doneDetail"></div>
      <button class="btn btn-more" onclick="resetUpload()">Send More Files</button>
    </div>
  </div>
  <div class="footer">Powered by <a href="https://github.com/darkprince558/jend">JEND</a> · Secure file transfer</div>
</div>`
}

// uploadPageScript returns the JavaScript that powers the upload interaction.
// It handles file selection, drag-and-drop, XHR upload with progress, and state transitions.
func uploadPageScript() string {
	return `
<script>
// ── State ──
var selectedFiles = [];
var fileInput = document.getElementById('fileInput');
var cameraInput = document.getElementById('cameraInput');
var dropZone = document.getElementById('dropZone');
var fileList = document.getElementById('fileList');
var uploadBtn = document.getElementById('uploadBtn');

// ── File Selection ──

fileInput.addEventListener('change', function(e) {
  addFiles(Array.from(e.target.files));
  e.target.value = '';
});

cameraInput.addEventListener('change', function(e) {
  addFiles(Array.from(e.target.files));
  e.target.value = '';
});

// ── Drag & Drop ──

dropZone.addEventListener('dragover', function(e) {
  e.preventDefault();
  dropZone.classList.add('dragover');
});

dropZone.addEventListener('dragleave', function(e) {
  e.preventDefault();
  dropZone.classList.remove('dragover');
});

dropZone.addEventListener('drop', function(e) {
  e.preventDefault();
  dropZone.classList.remove('dragover');
  addFiles(Array.from(e.dataTransfer.files));
});

// ── File List Management ──

function addFiles(newFiles) {
  for (var i = 0; i < newFiles.length; i++) {
    selectedFiles.push(newFiles[i]);
  }
  renderFileList();
}

function removeFile(idx) {
  selectedFiles.splice(idx, 1);
  renderFileList();
}

function renderFileList() {
  fileList.innerHTML = '';
  for (var i = 0; i < selectedFiles.length; i++) {
    var f = selectedFiles[i];
    var div = document.createElement('div');
    div.className = 'file-item';
    div.innerHTML = '<span class="file-item-name">' + escapeHtml(f.name) + '</span>' +
      '<span class="file-item-size">' + formatBytes(f.size) + '</span>' +
      '<span class="file-item-remove" onclick="removeFile(' + i + ')">×</span>';
    fileList.appendChild(div);
  }
  uploadBtn.disabled = selectedFiles.length === 0;
  uploadBtn.textContent = selectedFiles.length === 0 ? 'Upload Files' :
    'Upload ' + selectedFiles.length + ' file' + (selectedFiles.length > 1 ? 's' : '');
}

// ── Upload ──

function startUpload() {
  if (selectedFiles.length === 0) return;

  // Switch to progress view.
  document.getElementById('uploadArea').style.display = 'none';
  var progressArea = document.getElementById('progressArea');
  progressArea.style.display = 'block';

  // Build per-file status items.
  var uploadList = document.getElementById('uploadList');
  uploadList.innerHTML = '';
  for (var i = 0; i < selectedFiles.length; i++) {
    var div = document.createElement('div');
    div.className = 'upload-item';
    div.id = 'upload-item-' + i;
    div.innerHTML = '<span class="upload-item-name">' + escapeHtml(selectedFiles[i].name) + '</span>' +
      '<span class="upload-item-status" id="upload-status-' + i + '">Waiting...</span>';
    uploadList.appendChild(div);
  }

  // Send all files in a single multipart POST.
  var formData = new FormData();
  for (var j = 0; j < selectedFiles.length; j++) {
    formData.append('files', selectedFiles[j]);
  }

  var xhr = new XMLHttpRequest();
  xhr.open('POST', window.location.pathname + '/upload', true);

  // Track upload progress.
  xhr.upload.addEventListener('progress', function(e) {
    if (e.lengthComputable) {
      var pct = Math.round((e.loaded / e.total) * 100);
      document.getElementById('progressFill').style.width = pct + '%';
      document.getElementById('progressText').textContent =
        pct + '% — ' + formatBytes(e.loaded) + ' / ' + formatBytes(e.total);
      for (var k = 0; k < selectedFiles.length; k++) {
        var st = document.getElementById('upload-status-' + k);
        if (st) st.textContent = 'Uploading...';
      }
    }
  });

  // Handle completion.
  xhr.addEventListener('load', function() {
    if (xhr.status === 200) {
      showUploadSuccess(progressArea);
    } else {
      showUploadError(xhr.responseText);
    }
  });

  // Handle network errors.
  xhr.addEventListener('error', function() {
    document.getElementById('progressText').textContent =
      'Network error — are you on the same WiFi?';
  });

  xhr.send(formData);
}

function showUploadSuccess(progressArea) {
  document.getElementById('progressFill').style.width = '100%';
  for (var k = 0; k < selectedFiles.length; k++) {
    var st = document.getElementById('upload-status-' + k);
    if (st) { st.textContent = '✓ Done'; st.className = 'upload-item-status done'; }
  }
  setTimeout(function() {
    progressArea.style.display = 'none';
    document.getElementById('doneWrap').style.display = 'block';
    document.getElementById('doneMsg').textContent = 'Upload Complete!';
    document.getElementById('doneDetail').textContent = selectedFiles.length + ' file' +
      (selectedFiles.length > 1 ? 's' : '') + ' sent to laptop';
    document.getElementById('statusText').textContent = 'Received';
  }, 600);
}

function showUploadError(responseText) {
  var errMsg = 'Upload failed';
  try { errMsg = JSON.parse(responseText).error || errMsg; } catch(e) {}
  for (var k = 0; k < selectedFiles.length; k++) {
    var st = document.getElementById('upload-status-' + k);
    if (st) { st.textContent = '✗ ' + errMsg; st.className = 'upload-item-status error'; }
  }
  document.getElementById('progressText').textContent = 'Error: ' + errMsg;
}

// ── Reset ──

function resetUpload() {
  selectedFiles = [];
  renderFileList();
  document.getElementById('uploadArea').style.display = 'block';
  document.getElementById('progressArea').style.display = 'none';
  document.getElementById('doneWrap').style.display = 'none';
  document.getElementById('progressFill').style.width = '0%';
  document.getElementById('statusText').textContent = 'Ready';
}

// ── Text Sending ──

function sendText() {
  var textInput = document.getElementById('textInput');
  var text = textInput.value.trim();
  if (!text) return;

  var btn = document.getElementById('sendTextBtn');
  var origText = btn.textContent;
  btn.textContent = 'Sending...';
  btn.disabled = true;

  var xhr = new XMLHttpRequest();
  xhr.open('POST', window.location.pathname + '/upload-text', true);
  xhr.setRequestHeader('Content-Type', 'application/json');
  
  xhr.addEventListener('load', function() {
    btn.disabled = false;
    if (xhr.status === 200) {
      btn.textContent = 'Sent!';
      textInput.value = '';
      setTimeout(function(){ btn.textContent = origText; }, 2000);
    } else {
      btn.textContent = 'Failed';
      setTimeout(function(){ btn.textContent = origText; }, 2000);
    }
  });

  xhr.addEventListener('error', function() {
    btn.disabled = false;
    btn.textContent = 'Error';
    setTimeout(function(){ btn.textContent = origText; }, 2000);
  });

  xhr.send(JSON.stringify({ text: text }));
}

// ── Utilities ──

function formatBytes(b) {
  if (b < 1024) return b + ' B';
  if (b < 1048576) return (b / 1024).toFixed(1) + ' KB';
  if (b < 1073741824) return (b / 1048576).toFixed(1) + ' MB';
  return (b / 1073741824).toFixed(1) + ' GB';
}

function escapeHtml(s) {
  var d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}
</script>
</body>
</html>`
}
