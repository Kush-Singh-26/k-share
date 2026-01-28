package ui

import (
	"archive/zip"
	"fmt"
	"io"
	"k-share-client/api"
	"k-share-client/config"
	"k-share-client/crypto"
	"k-share-client/discovery"
	"net"
	"strings"

	"log"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/atotto/clipboard"
	"github.com/sqweek/dialog"
)

// Connection operations
func (a *App) connect() {
	if ip, _ := a.serverIP.Get(); ip == "" {
		a.autoDiscover()
		return
	}

	// Debounce: prevent multiple connection attempts
	if a.isConnecting {
		return
	}
	a.isConnecting = true
	fyne.Do(func() {
		if a.connectBtn != nil {
			a.connectBtn.Disable()
		}
	})

	a.statusText.Set("🟡 Connecting...")

	go func() {
		defer func() {
			a.isConnecting = false
			fyne.Do(func() {
				if a.connectBtn != nil {
					a.connectBtn.Enable()
				}
			})
		}()

		role, err := a.apiClient.Ping()

		if err != nil {
			log.Printf("Connection failed: %v", err)
			a.statusText.Set("🔴 Failed: " + err.Error())
			return
		}

		// TOFU Certificate Verification
		certInfo := crypto.Manager.GetLastSeenCert()
		if certInfo != nil && !crypto.Manager.IsTrusted(certInfo.Hash) {
			serverIP, _ := a.serverIP.Get()
			crypto.Manager.SetPendingCert(certInfo, serverIP)

			// Show trust dialog on main thread
			fyne.Do(func() {
				a.showTrustDialog(certInfo, serverIP, role)
			})
			return // Wait for user decision
		}

		a.completeConnection(role)
	}()
}

// completeConnection finishes the connection after trust is established
func (a *App) completeConnection(role string) {
	a.statusText.Set("🟢 Connected")

	// Handle Role
	if role == "guest" {
		a.isGuest = true
		a.clipboardChannel = "guest"
		fyne.Do(func() {
			a.clipChannelSelect.Hide()
			a.clipGuestLabel.Show()
			if a.historyBtn != nil {
				a.historyBtn.Hide()
			}
		})
	} else {
		a.isGuest = false
		fyne.Do(func() {
			a.clipChannelSelect.Show()
			a.clipGuestLabel.Hide()
			if a.historyBtn != nil {
				a.historyBtn.Show()
			}
		})
	}

	// Connect WebSocket
	a.wsClient.Connect()

	// Initial data fetch
	a.fetchClipboard()
	a.loadFiles()
	a.loadHistory()

	// Save to SavedNetworks if it's a valid LAN IP
	fullAddr, _ := a.serverIP.Get()
	host, _, err := net.SplitHostPort(fullAddr)
	if err == nil && host != "localhost" && host != "127.0.0.1" {
		parts := strings.Split(host, ".")
		if len(parts) == 4 {
			subnet := fmt.Sprintf("%s.%s.%s", parts[0], parts[1], parts[2])
			config.Current.SavedNetworks[subnet] = host
			config.Save()
		}
	}
}

// showTrustDialog displays a dialog for TOFU certificate verification
func (a *App) showTrustDialog(certInfo *crypto.CertInfo, serverIP string, role string) {
	trustWindow := a.fyneApp.NewWindow("Trust This Server?")
	trustWindow.Resize(fyne.NewSize(450, 300))

	// Build certificate info display
	infoText := fmt.Sprintf(
		"A new server certificate was detected.\n\n"+
			"Server: %s\n"+
			"Fingerprint:\n%s\n\n"+
			"Valid: %s to %s\n\n"+
			"Do you want to trust this server?",
		serverIP,
		certInfo.Fingerprint,
		certInfo.NotBefore,
		certInfo.NotAfter,
	)

	infoLabel := widget.NewLabel(infoText)
	infoLabel.Wrapping = fyne.TextWrapWord

	trustBtn := widget.NewButton("✅ Trust", func() {
		code, _ := a.pairingCode.Get()
		crypto.Manager.TrustCertificate(certInfo.Hash, serverIP, "K-Share Server", code)
		trustWindow.Close()
		go a.completeConnection(role)
	})
	trustBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButton("❌ Cancel", func() {
		crypto.Manager.ClearPending()
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
		port := 26260 // Default port
		// If user has a custom port in the entry (e.g. :9999), we might want to parse it,
		// but typically discovery implies standard port. We'll stick to default for now
		// or parse if the user typed ":9999" but no IP.
		// For robustness, let's assume default port for auto-discovery.

		code, _ := a.pairingCode.Get()

		// Update UI with scan status
		onStatus := func(msg string) {
			a.statusText.Set("🔍 " + msg)
		}

		ip := discovery.FindServer(port, code, onStatus)

		if ip != "" {
			address := fmt.Sprintf("%s:%d", ip, port)
			a.serverIP.Set(address)
			config.Current.ServerIP = address

			// Save to SavedNetworks
			// We need a helper to get the subnet, but for now we can rely on
			// successful connection to save it later or just save it here.
			// Let's rely on config.Save() being called in SetServerIP or manually.
			// Ideally we want to map Subnet -> IP.
			// For now, simpler: just update config and connect.
			config.Save()

			// Save to network cache
			// (We'll do this in a fire-and-forget manner or let the next discovery use it)
			// Ideally access discovery helpers to get subnet.
			// For now, we update the UI which triggers config save in OnChanged.

			a.statusText.Set("✅ Found: " + ip)
			a.connect()
		} else {
			a.statusText.Set("🔴 Server not found")
		}
	}()
}

// Clipboard operations
func (a *App) fetchClipboard() {
	text, err := a.apiClient.GetClipboard(a.clipboardChannel)
	if err != nil {
		log.Printf("Fetch clipboard failed: %v", err)
		return
	}
	a.clipboardText.Set(text)
}

func (a *App) pushClipboard() {
	text, _ := a.clipboardText.Get()
	err := a.apiClient.PushClipboard(text, a.clipboardChannel)
	if err != nil {
		log.Printf("Push clipboard failed: %v", err)
		return
	}
}

func (a *App) copyToSystemClipboard() {
	text, _ := a.clipboardText.Get()
	err := clipboard.WriteAll(text)
	if err != nil {
		log.Printf("Copy to clipboard failed: %v", err)
		return
	}
}

// File operations
func (a *App) loadFiles() {
	files, err := a.apiClient.ListFiles()
	if err != nil {
		log.Printf("Load files failed: %v", err)
		a.statusText.Set("🔴 Load failed: " + err.Error())
		return
	}
	// Convert to []interface{} for UntypedList binding
	data := make([]interface{}, len(files))
	for i, f := range files {
		data[i] = f
	}

	// Update binding (thread safe)
	a.filesBinding.Set(data)
}

// getUniqueFilename returns a unique filename by adding numerical suffix if file exists
func getUniqueFilename(basePath string) string {
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return basePath
	}

	dir := filepath.Dir(basePath)
	ext := filepath.Ext(basePath)
	nameWithoutExt := filepath.Base(basePath)
	nameWithoutExt = nameWithoutExt[:len(nameWithoutExt)-len(ext)]

	counter := 1
	for {
		newPath := filepath.Join(dir, nameWithoutExt+" ("+string(rune('0'+counter))+")"+ext)
		if counter > 9 {
			newPath = filepath.Join(dir, nameWithoutExt+" ("+itoa(counter)+")"+ext)
		}
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
		counter++
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}

func (a *App) downloadFile(file api.FileInfo) {
	basePath := filepath.Join(config.Current.DownloadFolder, file.Name)
	var downloadPath string
	var targetUnzipFolder string

	if file.IsDirectory {
		// Ensure the target folder for extraction is unique (e.g. "Folder (1)")
		targetUnzipFolder = getUniqueFilename(basePath)
		// Use a matching zip name for the temp file
		downloadPath = targetUnzipFolder + ".zip"
	} else {
		downloadPath = getUniqueFilename(basePath)
	}

	os.MkdirAll(filepath.Dir(downloadPath), 0755)

	a.statusText.Set("📥 Downloading...")

	go func() {
		reader, err := a.apiClient.DownloadFile(file.Name, "")
		if err != nil {
			log.Printf("Download failed: %v", err)
			a.statusText.Set("🔴 Download failed")
			return
		}
		defer reader.Close()

		// Save stream to file
		destFile, err := os.Create(downloadPath)
		if err != nil {
			log.Printf("Create file failed: %v", err)
			a.statusText.Set("🔴 Download failed")
			return
		}

		if _, err := io.Copy(destFile, reader); err != nil {
			destFile.Close()
			log.Printf("Download copy failed: %v", err)
			a.statusText.Set("🔴 Download failed")
			return
		}
		destFile.Close()

		// If it was a directory download (zip), extract it and remove zip
		if file.IsDirectory {
			a.statusText.Set("📦 Extracting...")
			// Remove .zip suffix to get target folder path
			folderPath := strings.TrimSuffix(downloadPath, ".zip")

			// Unzip to folderPath
			if err := unzip(downloadPath, folderPath); err != nil {
				log.Printf("Unzip failed: %v", err)
				a.statusText.Set("🔴 Unzip failed: " + err.Error())
				return
			}

			// Delete the zip file
			os.Remove(downloadPath)
			a.statusText.Set("✅ Downloaded: " + file.Name)
		} else {
			a.statusText.Set("✅ Downloaded: " + file.Name)
		}
	}()
}

// Helper to unzip a file
func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	os.MkdirAll(dest, 0755)

	// Closure to address file descriptors issue with defer
	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		path := filepath.Join(dest, f.Name)

		// Check for ZipSlip (Directory traversal)
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", path)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
		} else {
			os.MkdirAll(filepath.Dir(path), f.Mode())
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer f.Close()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *App) uploadFile() {
	// Use native Windows file dialog
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
		if err := a.apiClient.UploadFile(baseName, file); err != nil {
			log.Printf("Upload failed: %v", err)
			a.statusText.Set("🔴 Upload failed")
			return
		}

		a.statusText.Set("✅ Uploaded: " + baseName)
	}()
}

func (a *App) uploadFolder() {
	// Use simple standard library dialog call
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
		filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			relPath, _ := filepath.Rel(folderPath, path)
			fullRelPath := filepath.Join(folderName, relPath)

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			if err := a.apiClient.UploadFile(fullRelPath, file); err != nil {
				log.Printf("Failed to upload %s: %v", relPath, err)
				return err
			}

			return nil
		})

		a.statusText.Set("✅ Uploaded: " + folderName)
	}()
}

func (a *App) selectDownloadFolder() {
	// Use simple standard library dialog call
	folderPath, err := dialog.Directory().Title("Select Download Folder").Browse()
	if err != nil {
		if err != dialog.ErrCancelled {
			log.Printf("Folder dialog error: %v", err)
		}
		return
	}

	a.downloadFolder.Set(folderPath)
	config.Current.DownloadFolder = folderPath
	config.Save()
	a.statusText.Set("✅ Folder set")
}

// History operations
func (a *App) loadHistory() {
	if a.isGuest {
		return
	}
	items, err := a.apiClient.GetHistory()
	if err != nil {
		log.Printf("Load history failed: %v", err)
		return
	}
	a.historyItems = items
}

func (a *App) deleteHistoryItem(id string) {
	err := a.apiClient.DeleteHistoryItem(id)
	if err != nil {
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
		err := a.apiClient.DeleteFile(filename)
		if err != nil {
			log.Printf("Delete failed: %v", err)
			a.statusText.Set("🔴 Delete failed: " + err.Error())
			return
		}
		a.statusText.Set("✅ Deleted: " + filename)
		a.loadFiles()
	}()
}
