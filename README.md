# JEND

JEND is a modern, high-speed file transfer tool designed for the command line. It combines the speed of UDP/QUIC with the simplicity of magic wormhole-style transfers. No servers*, no complex setup—just secure, peer-to-peer file sharing.

## Why Jend?

* **Blazing Fast**: Built on **QUIC**, providing fast connection establishment and eliminating head-of-line blocking. Large files (>100MB) are automatically accelerated using **Parallel Streaming**.
* **Secure by Default**: Uses **True PAKE** (Password-Authenticated Key Exchange) for secure, mutual authentication. No data is stored on intermediaries.
* **Resilient**: Automatic network discovery (mDNS) finds peers on your LAN instantly. Transfers automatically **resume** if the connection drops.
* **Privacy First**: Incognito mode for ephemeral transfers without logs or clipboard traces.
* **Smart Handling**: Archives directories automatically. Supports `tar.gz` and `zip`. Handling small text snippets? Use the new text mode.
* **Automation Ready**: Headless mode supported for CI/CD and scripts.

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

### 🔒 Privacy & Incognito

Need to send sensitive data without leaving a trace?

* `--incognito`: Disables audit logging (`history.jsonl`) and prevents automatic clipboard copying of codes/content.
* `--no-history`: Disable only audit logging.
* `--no-clipboard`: Disable only clipboard operations.

```bash
jend send --incognito secret_keys.txt
```

### 🚀 High Performance

* **Parallel Streaming**: Files larger than 100MB are automatically split into 4 concurrent streams to saturate your bandwidth.
* **Compression**:
  * `--zip`: Force zip compression.
  * `--tar`: Force tar.gz compression.

### 🛡️ Resilience & Resume

* **Auto-Resume**: If the network drops, JEND pauses and waits. Once connectivity is restored, the transfer resumes automatically from the last received chunk.
* **Cancellation**: Sender interruption (Ctrl+C) instantly notifies the receiver to stop.

### 🤖 Automation (Headless)

Perfect for DevOps pipelines.

```bash
# Run silently with a 5-minute timeout
jend --headless --timeout 5m send backup.db
```

### 📜 Audit Trail

Keep track of every file you send or receive (unless Incognito).

```bash
# View transfer history
jend history

# Clear your logs
jend history --clear
```

---

## Development & Testing

We include a robust test runner for verifying stability.

```bash
./run_tests.sh
```

This runs:

1. **Unit Tests**: Core logic verification.
2. **E2E Tests**: Full system tests including large file transfers, resume support, cancellation, and binary integrity checks.

---

## Roadmap

* **Advanced Networking**:
  * **P2P Hole Punching**: NAT traversal for internet-wide transfers.
  * **IPv6 Support**: Full IPv6 compatibility.
* **Platform Support**:
  * **Mobile App**: Android/iOS client.
  * **Web Client**: Receive directly in-browser via WebTransport.

---
*Note: Currently defaults to direct connection (LAN). Use in trusted networks or tunnel accordingly.*
