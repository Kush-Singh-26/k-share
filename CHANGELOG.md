# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- GitHub Actions workflow for automated cross-platform builds and releases
- CI/CD documentation for automated builds
- CONTRIBUTING.md for developer guidelines
- SECURITY.md for vulnerability reporting
- Build badges and automated build documentation

### Changed

- Repository organization now uses cross-platform naming (server, desktop-app, android-app)
- Desktop app uses consistent naming across platforms

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