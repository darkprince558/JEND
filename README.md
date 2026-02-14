# JEND: Just Enough Network Drop

![JEND Banner](https://img.shields.io/badge/Status-Stable-success) ![Go Version](https://img.shields.io/github/go-mod/go-version/darkprince558/jend) ![License](https://img.shields.io/github/license/darkprince558/jend)

JEND is a high-performance, secure file transfer tool designed for the modern web. It allows you to move files between computers **instantly**—whether you are on the same WiFi, across the internet, or behind strict corporate firewalls.

It combines the speed of local LAN transfers with the convenience of cloud-coordinated P2P, all wrapped in a beautiful, borderless terminal UI.

## ✨ Features

* **Hybrid Connectivity**: Automatically selects the fastest path:
  * **Local (mDNS)**: Direct LAN transfer (Fastest).
  * **Internet (P2P/ICE)**: Direct connection via NAT traversal.
  * **Relay (TURN)**: Fallback for strict firewalls.
* **Zero-Trust Security**: End-to-End Encrypted using PAKE (Password Authenticated Key Exchange) and AES-256-GCM. We never see your files.
* **S3 Mode**: Upload files to a temporary cloud bucket for later retrieval (`--s3`).
* **Resumable**: Transfers resume exactly where they left off if interrupted.
* **Beautiful UI**: Modern, gradient-based TUI built with Bubble Tea.

---

## 🚀 Installation

### macOS & Linux (Homebrew)

The easiest way to install JEND is via Homebrew:

```bash
brew install darkprince558/tap/jend
```

### Windows, Mac, Linux (Manual)

Download the latest binary for your OS from the [Releases Page](https://github.com/darkprince558/jend/releases).

1. Download the archive (e.g., `jend_Windows_x86_64.zip` or `jend_Darwin_arm64.tar.gz`).
2. Extract the binary.
3. Add it to your PATH (optional).

### From Source (Go)

```bash
go install github.com/darkprince558/jend/cmd/jend@latest
```

---

## 📚 Quick Start

### 1. Send a File

On machine A:

```bash
jend send my_large_file.zip
# Output: Code: happy-delta-seven
```

### 2. Receive a File

On machine B:

```bash
jend receive happy-delta-seven
```

*Note: If you are on the same WiFi, discovery is automatic!*

---

## 🛠 Advanced Usage

### Cloud Storage Mode (S3)

Need to send a file but the receiver isn't online yet? Upload it to the cloud:

```bash
jend send --s3 important_doc.pdf
```

The receiver can download it anytime using the same code.

### Headless Mode (Scripts/CI)

Run without the UI for automation:

```bash
jend send --headless --text "SecretAPIKey"
```

### Custom Relay

If you have your own TURN server:

```bash
jend config set-relay --url "turn:my-server.com:3478" --user "me" --pass "123"
```
