# Contributing to K-Share

Thank you for your interest in contributing to K-Share. This document outlines the contribution process and coding standards.

## Getting Started

1. **Fork the repository**
2. **Clone your fork**: `git clone https://github.com/YOUR_USERNAME/k-share.git`
3. **Add the upstream**: `git remote add upstream https://github.com/Kush-Singh-26/k-share.git`
4. **Create a branch**: `git checkout -b feat/my-feature`

## Development Setup

### Prerequisites

- Go 1.26+
- For desktop GUI: [Fyne prerequisites](https://fyne.io/develop/#prerequisites)
- For Android: Android Studio + SDK

### Building locally

```powershell
# Server
cd server
go build -trimpath -ldflags="-s -w" -o k-share-server.exe

# Desktop TUI
cd desktop-app
go build -tags tui -trimpath -ldflags="-s -w" -o k-share-tui.exe main_tui.go

# Desktop GUI
cd desktop-app
go get fyne.io/fyne/v2/cmd/fyne
fyne package -os windows -icon ../assets/Icon.png -name k-share-desktop -release --app-id com.kshare.desktop
```

## Coding Standards

### Go

- Use `go vet` and `gofmt` before committing
- Keep imports organized (standard library first, then external)
- Avoid global state where possible
- Add doc comments for public APIs
- Handle errors explicitly - don't ignore with `_`

### Protocol Changes

- All HTTP endpoints and WebSocket events are part of the public contract
- Follow the [compatibility policy](./docs/protocol/compatibility-policy.md)
- Additive changes are preferred over breaking changes
- Document any protocol changes in the relevant doc file

### Config Changes

- Follow the [migration plan](./docs/migrations/config-migration-plan.md)
- Configuration keys should remain stable
- Additive fields are allowed with defaults

## Testing

Before submitting:

```powershell
# Run go vet
go vet ./...

# Format check
gofmt -l .

# Build check (all components)
cd server && go build ./...
cd desktop-app && go build -tags tui ./...
```

For the desktop app, also verify the TUI builds without GUI dependencies:

```powershell
cd desktop-app
go build -tags tui -trimpath -ldflags="-s -w" -o k-share-tui.exe main_tui.go
```

## Pull Request Process

1. **Ensure branch is up-to-date**: `git rebase upstream/main`
2. **Run local tests** (see above)
3. **Push to your fork**: `git push origin feat/my-feature`
4. **Open a PR** with:
   - Clear title describing the change
   - Description of what and why
   - Related issue number (if applicable)

### PR Title Convention

Use conventional commits:

- `feat: add new feature` - New functionality
- `fix: fix bug` - Bug fix
- `docs: update docs` - Documentation
- `refactor: refactor code` - Code refactoring
- `build: update build` - Build system changes

## Review Process

- PRs require review before merging
- Address feedback promptly
- Request re-review after changes

## Code of Conduct

- Be respectful and inclusive
- Accept constructive criticism professionally
- Focus on what is best for the project

## Questions?

Open an issue for discussion or reach out via GitHub Discussions.