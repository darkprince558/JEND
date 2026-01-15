# Network Intelligence (Auto-Discovery) Implementation Plan

## Objective
Enable seamless file transfer between devices on the same local network (LAN) without minimal user configuration. The Receiver should automatically find the Sender (or vice-versa) using the shared secret "Code" as a discovery key.

## Strategy: mDNS (Multicast DNS)
We will use mDNS to broadcast service presence.
- **Service Type**: `_jend._udp`
- **Instance Name**: The "Code" (e.g., `amaze-tiger-bravo`) hash. We won't broadcast the code directly for privacy, but we can broadcast a hash of it. Or, simpler:
    - **Sender**: Advertises `_jend._udp` service.
    - **Receiver**: Browses for `_jend._udp`.
    - **Matching**: The mDNS TXT record will contain a hashed version of the Code (or a session ID derived from it). If the receiver sees a matching ID, it connects.

## Implementation Steps

### 1. New Package: `internal/discovery`
Create a clean abstraction for advertising and browsing.
- **Library**: `github.com/grandcat/zeroconf` is a standard, pure Go mDNS library.
- **Files**:
    - `discovery.go`: Interfaces and shared logic.
    - `advertise.go`: Logic for the "Server" (Sender) to announce presence.
    - `browse.go`: Logic for the "Client" (Receiver) to find the server.

### 2. Integration with `sender.go`
- **Start**: Before blocking on `listener.Accept()`, start MDNS Advertising.
- **Stop**: Stop advertising once a connection is established or timeout occurs.

### 3. Integration with `receiver.go`
- **Discovery Phase**: Instead of immediately dialing `localhost`, start MDNS Browsing.
- **Wait**: Look for a service instance where `txt["code_hash"] == hash(my_code)`.
- **Connect**: Upon finding the IP/Port, proceed with QUIC dialing to that address.

### 4. Updates to `main.go`
- Remove hardcoded `localhost` default.
- Add `--ip` flag to manually override discovery (fallback).

## Complexity Assessment
**Medium**.
- Core logic is straightforward (library usage).
- Edge cases (firewalls, multiple interfaces, timeouts) are where the complexity lies.
- We need to handle the case where "discovery fails" gracefully (prompt user or fail with helpful message).

## Plan
1.  **Add Dependency**: `go get github.com/grandcat/zeroconf`
2.  **Create Package**: Implement `internal/discovery`.
3.  **Update Sender**: Broadcast presence.
4.  **Update Receiver**: Scan and Connect.
5.  **Test**: Manual test with two terminals (mimicking two machines).

## Security Note
Broadcasting the Code (even hashed) on the LAN lets anyone on the LAN know a transfer is happening. This is acceptable for a "local file transfer" tool, but we must ensure the *connection* itself remains authenticated via PAKE (which we already have).
