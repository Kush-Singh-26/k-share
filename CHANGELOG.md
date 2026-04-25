# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [6.0.0] - 2026-04-25

### Added
- **Smart Network Discovery**: Tiered TCP-based discovery system with priority zone scanning.
- **Enhanced Security**: 
    - Full TLS 1.3 / HTTPS encryption for all traffic.
    - Trust-On-First-Use (TOFU) certificate pinning.
    - Dual-Role System (Admin & Guest) with strict server-side enforcement.
- **Advanced File Transfer**:
    - Folder support with on-the-fly zipping/unzipping.
    - Recursive uploads from Android.
    - Automatic versioning to prevent file overwrites.
- **Modernized UI**:
    - High-performance dashboard in Fyne (Desktop).
    - Jetpack Compose-based Android App.
    - System tray integration with IP display.

### Changed
- Migrated from mDNS-only discovery to reliable TCP zone scanning.
- Improved background architecture for "set-and-forget" reliability.

## [1.0.0] - Initial Release

### Added

- **Server**: Local HTTPS/WebSocket service
  - File storage and downloads
  - Clipboard text and image sync
  - Clipboard history
  - Thumbnail generation
  - Role-based access (admin/guest)
  - mDNS discovery
  - WebSocket event fanout

- **Desktop App (GUI)**: Cross-platform desktop client
  - LAN server discovery
  - File browser with upload/download
  - Clipboard sync
  - Trust-on-first-use certificate pinning

- **Desktop App (TUI)**: Terminal UI
  - Lightweight terminal interface
  - File operations
  - Clipboard sync
  - History browsing

- **Android App**: Mobile client
  - Server discovery and trust
  - File transfer
  - Clipboard sync

### Documentation

- HTTP API reference
- WebSocket event types
- Configuration schema
- Trust model
- Protocol compatibility policy
- System architecture overview

---

## Version History

| Version | Date       | Notes |
|---------|------------|-------|
| 1.0.0   | 2026-04-25 | Initial release |

---

## Migration Guides

### Upgrading from 0.x

This is the first stable release. Fresh installation is recommended.

---

*This changelog was generated for the initial 1.0.0 release. Future releases will include detailed change logs per version.*