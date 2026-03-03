# JEND Web Platform Roadmap

This task list tracks the full vision and implementation plan for the hosted JEND Web App.

- [ ] **Phase 1: Foundational Architecture & Tech Stack**
  - [ ] Initialize modern web frontend (Next.js / Vue / SvelteKit).
  - [ ] Set up the hosted Signaling Server (Go WebSockets).
  - [ ] Deploy basic STUN/TURN servers for WebRTC NAT traversal.
  - [ ] Define the common signaling protocol to unite the CLI and the Web App.
- [ ] **Phase 2: Core Web-to-Web Data Transfer**
  - [ ] Implement WebRTC Data Channels in the browser.
  - [ ] Build the landing page UI (drag-and-drop zone, peer list).
  - [ ] Implement connection handshakes via short 4-digit numeric/word codes.
  - [ ] Support sending/receiving single files and text snippets.
- [ ] **Phase 3: CLI ↔ Web Integration**
  - [ ] Update JEND CLI to connect to the global Signaling Server (using a specific flag like `--web` or by default if local discovery fails).
  - [ ] Implement cross-platform capability so CLI can send files to the Web UI using matching room codes.
- [ ] **Phase 4: Advanced Dropzones & Multi-Peer**
  - [ ] Implement "Rooms" where 3+ people can join the same code.
  - [ ] Implement a shared live "Dropzone" (files dropped are available to all).
  - [ ] Implement simple E2EE local chat within the room.
- [ ] **Phase 5: UX & Quality of Life Features**
  - [ ] Progressive Web App (PWA) configuration for mobile installability.
  - [ ] Mobile native Share-Target integration (share directly from iOS/Android menus).
  - [ ] QR Code fast-pairing (Scan QR on desktop screen to join room on phone).
  - [ ] Interactive file approval prompts (accept/reject file).
- [ ] **Phase 6: Enterprise / Power-User Features**
  - [ ] Hosted relay fallback (for symmetric NAT networks where P2P fails).
  - [ ] End-to-End Encrypted persistent link sharing (temporary cloud storage for offline sending).
  - [ ] Local directory web streaming (browse a shared folder via web UI).
