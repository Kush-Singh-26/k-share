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
	"sync/atomic"

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
			a.statusText.Set("🔴 Failed: " + err.Error())
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
		a.statusText.Set("🔴 Connection cancelled")
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
			a.statusText.Set("✅ Found: " + ip)
			a.connect()
		} else {
			a.statusText.Set("🔴 Server not found")
		}
	}()
}

// Clipboard operations
func (a *App) fetchClipboard() {
	text, err := a.clipOps.FetchText(context.Background(), a.clipboardChannel)
	if err != nil {
		log.Printf("Fetch clipboard failed: %v", err)
		a.statusText.Set("🔴 Fetch failed")
		return
	}
	a.clipboardText.Set(text)
	a.statusText.Set("📋 Clipboard fetched")
}

func (a *App) pushClipboard() {
	text, _ := a.clipboardText.Get()
	err := a.clipOps.PushText(context.Background(), text, a.clipboardChannel)
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
	data, err := a.clipOps.FetchImage(context.Background())
	if err != nil {
		log.Printf("Fetch clipboard image failed: %v", err)
		return
	}

	a.lastImageHash = a.clipOps.SystemImageHash(data)
	a.clipOps.WriteImageToSystemClipboard(data)
	a.statusText.Set("📋 Image copied to clipboard")
}

// File operations
func (a *App) loadFiles() {
	files, err := a.fileOps.ListFiles(context.Background(), "")
	if err != nil {
		log.Printf("Load files failed: %v", err)
		a.statusText.Set("🔴 Load failed: " + err.Error())
		return
	}

	data := make([]interface{}, len(files))
	for i, f := range files {
		data[i] = f
	}
	a.filesBinding.Set(data)
}

func (a *App) downloadFile(file api.FileInfo) {
	a.statusText.Set("📥 Downloading...")

	go func() {
		if err := a.fileOps.Download(context.Background(), file); err != nil {
			log.Printf("Download failed: %v", err)
			a.statusText.Set("🔴 Download failed: " + err.Error())
			return
		}
		a.statusText.Set("✅ Downloaded: " + file.Name)
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
		file, err := os.Open(filename)
		if err != nil {
			log.Printf("Open file failed: %v", err)
			a.statusText.Set("🔴 Upload failed")
			return
		}
		defer file.Close()

		baseName := filepath.Base(filename)
		if err := a.fileOps.UploadFile(context.Background(), baseName, file); err != nil {
			log.Printf("Upload failed: %v", err)
			a.statusText.Set("🔴 Upload failed")
			return
		}

		a.statusText.Set("✅ Uploaded: " + baseName)
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
		if err := a.fileOps.UploadFolder(context.Background(), folderPath); err != nil {
			log.Printf("Failed to upload folder %s: %v", folderName, err)
			a.statusText.Set("🔴 Upload failed")
			return
		}
		a.statusText.Set("✅ Uploaded: " + folderName)
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
	a.statusText.Set("✅ Folder set")
}

// History operations
func (a *App) loadHistory() {
	if a.isGuest {
		return
	}
	items, err := a.historyOps.Load(context.Background())
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
	if err := a.historyOps.Delete(context.Background(), id); err != nil {
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
		if err := a.fileOps.DeleteFile(context.Background(), filename); err != nil {
			log.Printf("Delete failed: %v", err)
			a.statusText.Set("🔴 Delete failed: " + err.Error())
			return
		}
		a.statusText.Set("✅ Deleted: " + filename)
		a.loadFiles()
	}()
}
