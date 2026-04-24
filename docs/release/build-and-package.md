# Build and Package

## Server

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

