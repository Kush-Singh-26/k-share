package ui

import (
	"context"
	"desktop-app/api"
	"desktop-app/config"
	"desktop-app/crypto"
	"desktop-app/session"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/sqweek/dialog"
)

// Connection operations
func (a *App) connect() {
	if ip, _ := a.serverIP.Get(); ip == "" {
		a.autoDiscover()
		return
	}

	if !atomic.CompareAndSwapInt32(&a.isConnecting, 0, 1) {
		return
	}
	fyne.Do(func() {
		if a.connectBtn != nil {
			a.connectBtn.Disable()
		}
	})

	a.statusText.Set("🟡 Connecting...")
	serverIP, _ := a.serverIP.Get()

	go func() {
		defer func() {
			atomic.StoreInt32(&a.isConnecting, 0)
			fyne.Do(func() {
				if a.connectBtn != nil {
					a.connectBtn.Enable()
				}
			})
		}()

		code, _ := a.pairingCode.Get()
		a.session.SetServerIP(serverIP)
		a.session.SetPairingCode(code)

		role, err := a.session.Connect()
		if err != nil {
			if err == session.ErrTrustRequired {
				certInfo, serverIP, pendingRole, ok := a.session.PendingTrust()
				if ok {
					fyne.Do(func() {
						a.showTrustDialog(certInfo, serverIP, pendingRole)
					})
				}
				return
			}
			log.Printf("Connection failed: %v", err)
			a.setStatus("🔴 Failed: "+err.Error(), 5*time.Second)
			return
		}

		a.completeConnection(role)
	}()
}

func (a *App) completeConnection(role string) {
	a.session.CompleteConnection(role)
	atomic.StoreInt32(&a.isConnected, 1)
	a.statusText.Set("🟢 Connected")

	if a.session.IsGuest() {
		a.isGuest = true
		a.clipboardChannel = a.session.ClipboardChannel()
		fyne.Do(func() {
			a.clipChannelSelect.Hide()
			a.clipGuestLabel.Show()
			if a.historyBtn != nil {
				a.historyBtn.Hide()
			}
		})
	} else {
		a.isGuest = false
		a.clipboardChannel = a.session.ClipboardChannel()
		fyne.Do(func() {
			a.clipChannelSelect.Show()
			a.clipGuestLabel.Hide()
			if a.historyBtn != nil {
				a.historyBtn.Show()
			}
		})
	}

	a.fetchClipboard()
	a.loadFiles()
	a.loadHistory()
}

func (a *App) showTrustDialog(certInfo *crypto.CertInfo, serverIP string, role string) {
	trustWindow := a.fyneApp.NewWindow("Trust This Server?")
	trustWindow.Resize(fyne.NewSize(450, 300))

	fingerprint := certInfo.Fingerprint
	if fingerprint == "" {
		fingerprint = "WARNING: NO FINGERPRINT DETECTED"
	}

	infoText := fmt.Sprintf(
		"A new server certificate was detected.\n\n"+
			"Server: %s\n"+
			"Fingerprint:\n%s\n\n"+
			"Valid: %s to %s\n\n"+
			"Do you want to trust this server?",
		serverIP,
		fingerprint,
		certInfo.NotBefore,
		certInfo.NotAfter,
	)

	infoLabel := widget.NewLabel(infoText)
	infoLabel.Wrapping = fyne.TextWrapWord

	trustBtn := widget.NewButton("✅ Trust", func() {
		a.session.TrustPending()
		trustWindow.Close()
		go a.completeConnection(role)
	})
	trustBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButton("❌ Cancel", func() {
		a.session.CancelPendingTrust()
		a.setStatus("🔴 Connection cancelled", 3*time.Second)
		trustWindow.Close()
	})

	buttons := container.NewHBox(cancelBtn, trustBtn)
	content := container.NewBorder(nil, buttons, nil, nil, container.NewVScroll(infoLabel))

	trustWindow.SetContent(content)
	trustWindow.Show()
}

func (a *App) autoDiscover() {
	a.statusText.Set("🔍 Scanning network...")

	go func() {
		const discoveryPort = 26260
		onStatus := func(msg string) {
			a.statusText.Set("🔍 " + msg)
		}

		ip := a.session.Discover(discoveryPort, onStatus)
		if ip != "" {
			a.serverIP.Set(a.session.ServerIP())
			a.setStatus("✅ Found: "+ip, 3*time.Second)
			a.connect()
		} else {
			a.setStatus("🔴 Server not found", 3*time.Second)
		}
	}()
}

// Clipboard operations
func (a *App) fetchClipboard() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	text, err := a.clipOps.FetchText(ctx, a.clipboardChannel)
	if err != nil {
		log.Printf("Fetch clipboard failed: %v", err)
		a.setStatus("🔴 Fetch failed", 3*time.Second)
		return
	}
	a.clipboardText.Set(text)
	a.setStatus("📋 Clipboard fetched", 3*time.Second)
}

func (a *App) pushClipboard() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	text, _ := a.clipboardText.Get()
	err := a.clipOps.PushText(ctx, text, a.clipboardChannel)
	if err != nil {
		log.Printf("Push clipboard failed: %v", err)
		return
	}
}

func (a *App) copyToSystemClipboard() {
	text, _ := a.clipboardText.Get()
	if err := a.clipOps.CopyToSystemClipboard(text); err != nil {
		log.Printf("Copy to clipboard failed: %v", err)
	}
}

func (a *App) fetchClipboardImage() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	data, err := a.clipOps.FetchImage(ctx)
	if err != nil {
		log.Printf("Fetch clipboard image failed: %v", err)
		return
	}

	a.lastImageHash = a.clipOps.SystemImageHash(data)
	a.clipOps.WriteImageToSystemClipboard(data)
	a.setStatus("📋 Image copied to clipboard", 3*time.Second)
}

// File operations
func (a *App) loadFiles() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	files, err := a.fileOps.ListFiles(ctx, a.currentPath)
	if err != nil {
		log.Printf("Load files failed: %v", err)
		a.setStatus("🔴 Load failed: "+err.Error(), 5*time.Second)
		return
	}

	// Update breadcrumb
	displayPath := "📁 Root"
	if a.currentPath != "" {
		displayPath = "📁 Root / " + strings.ReplaceAll(a.currentPath, "/", " / ")
		if a.backBtn != nil {
			a.backBtn.Enable()
		}
	} else {
		if a.backBtn != nil {
			a.backBtn.Disable()
		}
	}
	a.breadcrumbPath.Set(displayPath)

	data := make([]interface{}, len(files))
	for i, f := range files {
		data[i] = f
	}
	a.filesBinding.Set(data)
}

func (a *App) navigateUp() {
	if a.currentPath == "" {
		return
	}
	parts := strings.Split(a.currentPath, "/")
	if len(parts) <= 1 {
		a.currentPath = ""
	} else {
		a.currentPath = strings.Join(parts[:len(parts)-1], "/")
	}
	a.loadFiles()
}

func (a *App) downloadFile(file api.FileInfo) {
	if file.IsDirectory {
		a.currentPath = file.Name
		a.loadFiles()
		return
	}
	a.forceDownload(file)
}

func (a *App) forceDownload(file api.FileInfo) {
	a.statusText.Set("📥 Downloading...")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := a.fileOps.Download(ctx, file); err != nil {
			log.Printf("Download failed: %v", err)
			a.setStatus("🔴 Download failed: "+err.Error(), 5*time.Second)
			return
		}
		a.setStatus("✅ Downloaded: "+filepath.Base(file.Name), 3*time.Second)
	}()
}

func (a *App) uploadFile() {
	filename, err := dialog.File().Title("Select File to Upload").Load()
	if err != nil {
		if err != dialog.ErrCancelled {
			log.Printf("File dialog error: %v", err)
		}
		return
	}

	a.statusText.Set("📤 Uploading...")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		file, err := os.Open(filename)
		if err != nil {
			log.Printf("Open file failed: %v", err)
			a.setStatus("🔴 Upload failed", 5*time.Second)
			return
		}
		defer file.Close()

		baseName := filepath.Base(filename)
		uploadPath := baseName
		if a.currentPath != "" {
			uploadPath = a.currentPath + "/" + baseName
		}

		if err := a.fileOps.UploadFile(ctx, uploadPath, file); err != nil {
			log.Printf("Upload failed: %v", err)
			a.setStatus("🔴 Upload failed", 5*time.Second)
			return
		}

		a.setStatus("✅ Uploaded: "+baseName, 3*time.Second)
		a.loadFiles()
	}()
}

func (a *App) uploadFolder() {
	folderPath, err := dialog.Directory().Title("Select Folder to Upload").Browse()
	if err != nil {
		if err != dialog.ErrCancelled {
			log.Printf("Folder dialog error: %v", err)
		}
		return
	}

	folderName := filepath.Base(folderPath)
	a.statusText.Set("📤 Uploading folder...")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		
		// If we are in a subfolder, we need the service to know where to put it
		// But UploadFolder currently uses folderName internally.
		// Let's modify UploadFolder or handle it here.
		// Looking at Service.UploadFolder, it uses filepath.Join(folderName, relPath)
		// We should probably pass a target prefix.
		
		prefix := ""
		if a.currentPath != "" {
			prefix = a.currentPath + "/"
		}

		if err := a.fileOps.UploadFolderWithPrefix(ctx, folderPath, prefix); err != nil {
			log.Printf("Failed to upload folder %s: %v", folderName, err)
			a.setStatus("🔴 Upload failed", 5*time.Second)
			return
		}
		a.setStatus("✅ Uploaded: "+folderName, 3*time.Second)
		a.loadFiles()
	}()
}

func (a *App) selectDownloadFolder() {
	folderPath, err := dialog.Directory().Title("Select Download Folder").Browse()
	if err != nil {
		if err != dialog.ErrCancelled {
			log.Printf("Folder dialog error: %v", err)
		}
		return
	}

	a.downloadFolder.Set(folderPath)
	a.fileOps.SetDownloadFolder(folderPath)
	_ = config.SetDownloadFolder(folderPath)
	a.setStatus("✅ Folder set", 3*time.Second)
}

// History operations
func (a *App) loadHistory() {
	if a.isGuest {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	items, err := a.historyOps.Load(ctx)
	if err != nil {
		log.Printf("Load history failed: %v", err)
		return
	}
	a.historyItems = items

	// Refresh popup if open
	if a.historyWindow != nil {
		fyne.Do(func() {
			a.rebuildHistoryUI()
		})
	}
}

func (a *App) deleteHistoryItem(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := a.historyOps.Delete(ctx, id); err != nil {
		log.Printf("Delete history item failed: %v", err)
		return
	}
	a.loadHistory()
}

func (a *App) deleteFile(filename string) {
	if !dialog.Message("Delete %s?\nIt will be moved to the server's trash folder.", filepath.Base(filename)).
		Title("Confirm Delete").
		YesNo() {
		return
	}

	a.statusText.Set("🗑️ Deleting...")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := a.fileOps.DeleteFile(ctx, filename); err != nil {
			log.Printf("Delete failed: %v", err)
			a.setStatus("🔴 Delete failed: "+err.Error(), 5*time.Second)
		} else {
			a.setStatus("✅ Deleted: "+filename, 3*time.Second)
		}
		a.loadFiles()
	}()
}
