# K-Share

K-Share is a local-network file and clipboard sharing system with three products:

- `server`: the local HTTPS/WebSocket service that stores files, clipboard state, and history
- `desktop-app`: the desktop client (GUI and TUI) for browsing files, syncing clipboard, and trusting servers
- `android-app`: the Android client for sharing files and clipboard data from a phone
- `k-share-tui`: a lightweight terminal client (TUI) for headless or CLI-first workflows

The codebase is organized to support Windows, macOS, and Linux desktop/server targets, with platform-specific integrations isolated behind adapters.

## What It Does

- LAN discovery
- HTTPS file upload and download
- clipboard text sync
- clipboard image sync
- clipboard history
- guest/admin access separation
- thumbnail previews
- trust-on-first-use certificate pinning

## Repository Layout

- [`server`](./server) - Go server
- [`desktop-app`](./desktop-app) - Go desktop client
- [`android-app`](./android-app) - Android client
- [`docs`](./docs) - architecture and protocol docs, if present
- [`assets`](./assets) - shared artwork and icons

## Build

### Server

```powershell
cd server
go build -trimpath -ldflags="-s -w" -o k-share-server.exe
```

### Desktop app

#### Graphical UI (Fyne)
```powershell
cd desktop-app
fyne package -os windows -icon ../assets/Icon.png -name k-share-desktop -release --app-id com.kshare.desktop
```

The desktop app can also be built with plain `go build` for a quick compile check.

#### Terminal UI (TUI)
```powershell
cd desktop-app
go build -tags tui -trimpath -ldflags="-s -w" -o k-share-tui.exe main_tui.go
```
The TUI is built using [Bubbletea](https://github.com/charmbracelet/bubbletea) and excludes all GUI dependencies for a lightweight, fast binary.

### Android app

Open [`android-app`](./android-app) in Android Studio and build the `app` module normally.

## TUI Keybindings (k-share-tui)

- **Tab**: Cycle between views (History ➔ Files ➔ Clipboard ➔ Settings)
- **q / Ctrl+C**: Quit application
- **r**: Refresh data (History or Files)
- **Enter**: Perform primary action (Open link, Download file, Select field)
- **Ctrl+S**: (In Clipboard Tab) Push manual text to server
- **u**: (In Files Tab) Open local file picker for upload
- **d**: (In Files Tab) Delete remote file
- **o**: (In Files Tab) Open local download folder

## Configuration

### Server

The server stores its config in the OS user config directory under `K-Share/config.json`.

Key fields:

- `port`
- `shared_dir`
- `admin_code`
- `guest_code`

### Desktop app

The desktop app stores its settings in the OS user config directory under `K-Share/settings.json`.

## First Run

1. Start the server.
2. Note the admin code from the server config.
3. Open the desktop or Android client.
4. Enter the server IP and pairing code.
5. Accept the trust prompt on first connection.

## Platform Notes

- The server runs cross-platform.
- The desktop app runs cross-platform.
- Some integrations, such as native tray or context-menu actions, remain OS-specific adapters.

## License

MIT. See [`LICENSE`](./LICENSE).
