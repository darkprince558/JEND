# Debugging JEND

This guide explains how to debug JEND during development.

## 1. Headless Mode (Best for Debugging)

The TUI (Bubble Tea) swallows stdout/stderr. To see **raw logs**, always run in headless mode:

```bash
# Sender
go run ./cmd/jend send --headless --text "hello" --port 8080

# Receiver
go run ./cmd/jend receive --headless happy-delta-seven --output ./tmp
```

## 2. Common Issues & Components

### Signaling (MQTT)

**Role**: Exchanges connection info (ICE candidates, public IPs) between peers.
**Debug**:

- Logs will show `[MQTT] Connected` or `[MQTT] Error`.
- If signaling fails, peers can't find each other via P2P.
- **Code**: `internal/signaling/mqtt.go`

### Discovery (mDNS & Cloud)

**Role**: Finds peers on the same WiFi (mDNS) or via Registry (Cloud).
**Debug**:

- Watch for `Found locally via mDNS` vs `Found via Registry`.
- **Code**: `internal/discovery/`

### Transport (QUIC)

**Role**: The actual file transfer tunnel (UDP).
**Debug**:

- Connect errors usually mean UDP is blocked (Firewall/NAT).
- "Handshake failed" means PAKE auth mismatch (wrong code).
- **Code**: `internal/transport/quic.go`

### PAKE (Authentication)

**Role**: Zero-knowledge password exchange.
**Debug**:

- If `PAKE Failed` appears, the code was typed wrong or the curve implementation mismatches.
- **Code**: `internal/core/pake.go`

## 3. Resume & State

JEND saves transfer state to `.parallel.meta` (JSON) to allow resuming.
**To force a clean start**, delete this file:

```bash
rm .parallel.meta
```

## 4. Test Hooks

For simulating network conditions:

- **`JEND_TEST_DELAY=500ms`**: Adds latency to sender loops (see `sender.go`).
- **`JEND_Mock_Fail=true`**: (Internal) triggers failure paths in tests.
