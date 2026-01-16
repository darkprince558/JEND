# JEND

JEND is a modern, high-speed file transfer tool designed for the command line. It combines the speed of UDP/QUIC with the simplicity of magic wormhole-style transfers. No servers*, no complex setup—just secure, peer-to-peer file sharing.

![Terminal Demo Placeholder](https://via.placeholder.com/800x400?text=JEND+TUI+Demo)

## Why Jend?

*   **Blazing Fast**: Built on **QUIC**, providing fast connection establishment and eliminating head-of-line blocking.
*   **Secure by Default**: Uses **PAKE** (Password-Authenticated Key Exchange) for secure, ephemeral encryption. No data is stored on intermediaries.
*   **Resilient**: Automatic network discovery (mDNS) finds peers on your LAN instantly. Transfers automatically resume if the connection drops.
*   **Smart Handling**: Archives directories automatically. Supports `tar.gz` and `zip`. Handling small text snippets? Use the new text mode.
*   **Automation Ready**: Headless mode supported for CI/CD and scripts.

---

## Installation

```bash
go install github.com/darkprince558/jend/cmd/jend@latest
```

---

## Quick Start

### 1. Send Anything
Directly send a file or an entire directory.

```bash
# Send a single file
jend send data.csv

# Send a folder (automatically compressed)
jend send my_project/
```
*You'll get a 3-word code like: `amaze-tiger-bravo`*

### 2. Receive Securely
On the other machine, just use the code.

```bash
# Receive the file
jend receive amaze-tiger-bravo

# Receive and automatically unzip
jend --unzip receive amaze-tiger-bravo
```

### 3. Send Text
Quickly share a URL or code snippet without creating a file.

```bash
jend send --text "https://github.com/darkprince558/jend"
# Receiver will print the text and copy it to the clipboard
```

---

## Power User Features

### Compression Control
Take full control over how your files are packaged.
*   `--zip`: Force zip compression before sending.
    ```bash
    jend --zip send huge_log.txt
    ```
*   `--tar`: Force tar.gz compression.
*   `--unzip`: Receiver flag to automatically extract archives upon arrival.
*   `--no-clipboard`: Disable automatic clipboard copying (useful for sensitive data).

### Headless & Automation
Perfect for DevOps. Run JEND without the UI in your background scripts.
```bash
# Run silently with a 5-minute timeout
jend --headless --timeout 5m send backup.db
```

### Resilience & Resume
*   **Auto-Resume**: If the network drops, JEND pauses and waits. Once connectivity is restored, the transfer resumes automatically from where it left off.
*   **Cancellation**: Sender interruption (Ctrl+C) instantly notifies the receiver to stop, preventing partial file corruption or infinite retries.

### Safety First
*   **Integrity Check**: Every transfer is verified with a SHA-256 checksum.
*   **Path Sanitization**: Protection against Zip Slip attacks.
*   **Timeout**: Transfers expire if not accepted within the default 10-minute window.

### Audit Trail
Keep track of every file you send or receive.
```bash
# View transfer history
jend history

# Send without recording to history (Incognito Mode)
jend send --incognito secret.txt
# Receive without recording to history
jend receive --no-history <code>

# View detailed proof for a specific transfer
jend history partial-red-panda

# Clear your logs
jend history --clear
```

---

## Coming Soon (Roadmap)

We are actively improving JEND. Planned features include:

*   **Advanced Networking**:
    *   **P2P Hole Punching / Relay**: Fallback mechanisms for complex network topologies (e.g. across different NATs).
    *   **IPv6 Support**: Full IPv6 compatibility.
*   **Performance**:
    *   **Parallel Streaming**: Splitting large files for maximum bandwidth utilization.
    *   **Bandwidth Throttling**: User-defined speed limits.
    *   **Clientless Web Receive**: Receive files directly in a browser.
    *   **Dead Drop**: Encrypted async upload for offline receivers.
    *   **Ghost Mode**: Force relay usage for absolute IP privacy.


---
*Note: Currently defaults to direct connection (localhost/IP). Use in trusted networks or tunnel accordingly.*
