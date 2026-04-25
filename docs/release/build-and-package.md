# Build and Package

> **Version**: 6.0
> **Last Updated**: 2026-04-25

## Automated Builds via GitHub Actions

Pre-built binaries for Windows, macOS, and Linux are automatically built and released when a new git tag is pushed. See the [Release Process](#release-process) section below.

Binaries are available on the [Releases](https://github.com/Kush-Singh-26/k-share/releases) page:

- `k-share-server-{os}-amd64` - Server binary
- `k-share-tui-{os}-amd64` - Terminal UI binary
- `k-share-desktop-{os}` - Desktop GUI (packed app)

## Manual Build

### Server

```powershell
cd server
go build -trimpath -ldflags="-s -w" -o k-share-server.exe
```

## Desktop app

### Graphical UI
```powershell
cd desktop-app
fyne package -os windows -icon ../assets/Icon.png -name k-share-desktop -release --app-id com.kshare.desktop
```

### Terminal UI
```powershell
cd desktop-app
go build -tags tui -trimpath -ldflags="-s -w" -o k-share-tui.exe main_tui.go
```

## Android app

- Open `android-app` in Android Studio.
- Build the `app` module normally.

## Release Process

To create a new release:

1. Ensure all changes are committed and pushed
2. Create and push a new version tag:

```powershell
git tag v1.0.0
git push origin v1.0.0
```

The GitHub Actions workflow will automatically:

1. Build the server for Windows, macOS, and Linux
2. Build the TUI for all platforms
3. Build the desktop GUI for all platforms
4. Create a new GitHub Release with all binaries

Release artifacts are attached to the GitHub Release page.

### Version Format

Use semantic versioning with a `v` prefix (e.g., `v1.0.0`, `v1.1.0-beta.1`).

