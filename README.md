# JEND

<p align="center">
  <img src="FullLogoLight.png" alt="JEND Logo" width="300">
</p>

JEND is a high-performance, peer-to-peer file transfer tool written in Go. It allows you to move files securely between computers—regardless of whether they are on the same WiFi or behind strict corporate firewalls—without configuring servers or opening ports.

It was built to solve the fragility of existing tools (like `scp` or `rsync`) in unstable network conditions, using modern transport protocols to guarantee delivery.

## Installation

### Windows (Winget) — Recommended

```powershell
winget install jend
```

### macOS / Linux (Homebrew)

```bash
brew install darkprince558/tap/jend
```

### Arch Linux (AUR)

```bash
yay -S jend-bin
```

### Debian / Ubuntu / Fedora / Alpine

Download the `.deb`, `.rpm`, or `.apk` files directly from the [GitHub Releases](https://github.com/darkprince558/JEND/releases) page.

### Windows (Scoop)

```powershell
scoop bucket add darkprince558 https://github.com/darkprince558/homebrew-tap
scoop install jend
```

### From Source (Developers)

If you have Go installed:

```bash
go install github.com/darkprince558/jend/cmd/jend@latest
```

## Quick Start

The fastest way to send a file is to just type `jend send` and follow the interactive wizard. Or, you can pass the file directly:

**Sender**:

```bash
jend send my_project.zip
# JEND will output a 3-word code, for example: happy-delta-seven
```

**Receiver**:

```bash
jend receive happy-delta-seven
```

## Core Features and Usage

### The Interactive Wizard

If you simply run `jend send` without arguments, JEND launches a rich, interactive menu. Here you can browse your file system, select files, or type a text snippet to send. You can also visually toggle compression formats, privacy modes, and transfer types.

### QR Code Mode (Browser Sharing)

The easiest way to share a file with someone who does not have JEND installed, such as sending a document to your phone or a colleague on a restricted network.

* **Local Mode:** For sharing on the same WiFi.

  ```bash
  jend send --qr report.pdf
  ```

  This runs a lightweight local server and displays a QR code in your terminal. When the receiver scans it, the file downloads directly in their browser.

* **Cloud Mode (WebRTC):** For sharing across different networks or distances.

  ```bash
  jend send --qr --qr-mode cloud report.pdf
  ```

  Instead of a local server, JEND establishes an encrypted WebRTC connection directly to the receiver's browser anywhere in the world. It provides a 6-character transfer code (e.g. `Af38HJ`) that they can enter at the pairing website, or they can simply scan the QR code.

### Power User Workflows

* **Multi-File Sending:** Pass multiple files or directories, and JEND will bundle them into a single zip archive on the fly.

  ```bash
  jend send notes.txt schema.sql diagram.png
  ```

* **Piping from STDIN:** JEND detects when data is piped into it and treats the incoming bytes as a secure text transfer.

  ```bash
  cat notes.txt | jend send
  git log --oneline -10 | jend send
  ```

* **Cloud Storage (S3 Mode):** Upload files to temporary cloud storage for the receiver to download later if they are not currently online to accept a direct P2P transfer.

  ```bash
  jend send --s3 data.tar.gz
  ```

* **Incognito Mode:** Disables local audit log history saving and clipboard copying for highly sensitive transfers.

  ```bash
  jend send --incognito secret.txt
  ```

## Command Reference

### Configuration (`jend config`)

Persistent configuration to save your preferences globally.

* `jend config set-relay` — Save your private TURN server credentials for custom routing.
* `jend config clear-relay` — Reset to default relay settings.

### Custom Themes (`jend theme`)

JEND includes a fully interactive color theme picker. Run `jend theme` to see a live-updating mockup of the JEND interface. Scroll through available palettes (such as Dracula, Nord, or Catppuccin) to update your terminal colors instantly. Validations hit `enter` to save.

### Audit Log (`jend history`)

JEND maintains a local audit log of all your transfers. Running `jend history` launches an interactive viewer where you can scroll through past transfers, view file details, filter by Sent/Received, and search by filename. Run with `--headless` to print the log as a standard text table.

---

## Architecture & Internals

I built JEND to handle the edge cases that break other transfer tools. Here is how it works under the hood:

### 1. Transport Layer: QUIC (UDP)

Instead of standard TCP, JEND runs over QUIC (the protocol powering HTTP/3).

* **Why?** TCP suffers from head-of-line blocking; if one packet is lost, the entire connection halts. QUIC multiplexes streams, so if a packet drops on one stream, the others keep moving. This effectively saturates available bandwidth on lossy networks (like public WiFi).

### 2. Security: End-to-End Encrypted & Zero-Trust

JEND assumes the network is compromised. Every decision was made to ensure data integrity and privacy even if an attacker controls the Wi-Fi or the relay server.

* **Password-Authenticated Key Exchange (PAKE)**: Sending a password/code to a server allows the server to see it (Man-in-the-Middle). JEND uses an Augmented PAKE protocol. The sender and receiver mathematically prove they know the same 3-word code without ever exchanging the code itself.
* **Hardening**: Because short codes are prone to brute-force, key derivation uses Argon2id (Memory=64MB, Time=3). This forces an attacker to spend prohibitive CPU resources to guess a single code.
* **Authenticated Encryption (AEAD)**: All file data is encrypted using AES-256-GCM. This guarantees both Confidentiality (no one can read it) and Integrity (no one can tamper with it). Even if a malicious relay routes your traffic, they only see opaque noise.

### 3. Network Traversal: ICE & Custom Relays

Direct P2P connectivity is blocked by most NATs, strict enterprise networks, and AP isolation.

* **Discovery**: JEND attempts local mDNS discovery (IPv4/IPv6). On networks where multicast is blocked, it falls back to cloud registry matching: devices with the same public IP are recognized as co-located and given each other's local address.
* **Hole Punching**: If local discovery fails, it uses ICE (Interactive Connectivity Establishment) to traverse NATs.
* **Configurable Relays**: For impossible networks, JEND supports a "Bring Your Own Relay" system. You can point JEND at any standard TURN server to route traffic when P2P is blocked.

### 4. Reliability: State-Machine Resumption

Transfers over 100MB fail often on bad connections.

* If a transfer stops halfway, re-running the command causes JEND to read a local state journal, verify the file hash of downloaded chunks, and resume exactly where it left off. No downloading from 0% again.
