# System Overview

> **Version**: 6.0
> **Last Updated**: 2026-04-25

K-Share is a local-network sharing system with three products:

- `server`: the cross-platform background service that owns files, clipboard state, history, thumbnails, auth, and trustable endpoints
- `desktop-app`: the cross-platform desktop client for discovery, trust, browsing, and local desktop integration
- `android-app`: the Android client for phone-driven sharing and sync

## Core responsibilities

### Server

The server is the source of truth for:

- file storage and downloads
- role enforcement for `admin` and `guest`
- clipboard text and image state
- clipboard history
- thumbnail generation and caching
- WebSocket fanout for change events
- TLS certificate lifecycle and HTTPS transport

The server should stay usable on Windows, macOS, and Linux. Any OS-specific actions, such as native open-folder behavior or tray integration, must remain adapters around the core service.

### Desktop app

The desktop app is a client shell. It should:

- discover the server on the LAN
- connect using the saved pairing code
- pin and reuse trusted server certificates
- browse files and trigger uploads/downloads
- edit and sync clipboard content
- handle optional desktop integrations like folder open actions and notifications

The desktop app should not contain server-side business rules.

### Android app

The Android app is another client shell with the same core protocol expectations as the desktop app. It should:

- discover and trust a server
- sync files and clipboard content
- keep settings and cached network state locally
- run background sync when appropriate

## Data flow

1. A client discovers the server or uses a cached address.
2. The client connects over HTTPS using a pairing code.
3. The server validates the role from the bearer code.
4. The client pins the server certificate hash on first use.
5. File, clipboard, and history requests read or mutate server-owned state.
6. The server broadcasts WebSocket change events so clients can refresh local views.

## Design rules

- Keep transport logic separate from domain logic.
- Keep UI layers thin.
- Treat protocol contracts as first-class documentation and test targets.
- Prefer additive changes over breaking ones.
- Document any incompatible migration before changing behavior.

