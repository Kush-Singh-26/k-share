package ui

import (
	"bytes"
	"fmt"
	"io"
	"k-share-client/api"
	"k-share-client/config"
	"k-share-client/crypto"
	"k-share-client/discovery"
	"log"
	"os"
	"path/filepath"

	"github.com/atotto/clipboard"
	"github.com/sqweek/dialog"
)

// Connection operations
func (a *App) connect() {
	if ip, _ := a.serverIP.Get(); ip == "" {
		a.autoDiscover()
		return
	}

	a.statusText.Set("🟡 Connecting...")

	go func() {
		err := a.apiClient.Ping()

		if err != nil {
			log.Printf("Connection failed: %v", err)
			a.statusText.Set("🔴 Failed: " + err.Error())
			return
		}

		a.statusText.Set("🟢 Connected")

		// Connect WebSocket
		a.wsClient.Connect()

		// Initial data fetch
		a.fetchClipboard()
		a.loadFiles()
		a.loadHistory()
	}()
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
	text, err := a.apiClient.GetClipboard()
	if err != nil {
		log.Printf("Fetch clipboard failed: %v", err)
		return
	}
	a.clipboardText.Set(text)
}

func (a *App) pushClipboard() {
	text, _ := a.clipboardText.Get()
	err := a.apiClient.PushClipboard(text)
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
	files, err := a.apiClient.ListFromPhoneFiles()
	if err != nil {
		log.Printf("Load files failed: %v", err)
		return
	}
	a.files = files
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
	downloadPath := getUniqueFilename(basePath)
	os.MkdirAll(filepath.Dir(downloadPath), 0755)

	a.statusText.Set("📥 Downloading...")

	go func() {
		encReader, err := a.apiClient.DownloadFile(file.Name, "fromphone")
		if err != nil {
			log.Printf("Download failed: %v", err)
			a.statusText.Set("🔴 Download failed")
			return
		}
		defer encReader.Close()

		if filepath.Ext(file.Name) == ".zip" {
			tempZip := downloadPath + ".temp"
			tempFile, err := os.Create(tempZip)
			if err != nil {
				log.Printf("Create temp file failed: %v", err)
				a.statusText.Set("🔴 Download failed")
				return
			}

			if err := crypto.DecryptStream(tempFile, encReader, config.Current.PairingCode); err != nil {
				tempFile.Close()
				os.Remove(tempZip)
				log.Printf("Decryption failed: %v", err)
				a.statusText.Set("🔴 Decryption failed")
				return
			}
			tempFile.Close()
			os.Rename(tempZip, downloadPath)
		} else {
			destFile, err := os.Create(downloadPath)
			if err != nil {
				log.Printf("Create file failed: %v", err)
				a.statusText.Set("🔴 Download failed")
				return
			}
			defer destFile.Close()

			if err := crypto.DecryptStream(destFile, encReader, config.Current.PairingCode); err != nil {
				log.Printf("Decryption failed: %v", err)
				a.statusText.Set("🔴 Decryption failed")
				return
			}
		}

		a.statusText.Set("✅ Downloaded: " + file.Name)
	}()
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
		fileData, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			log.Printf("Read file failed: %v", err)
			a.statusText.Set("🔴 Upload failed")
			return
		}

		baseName := filepath.Base(filename)
		if err := a.apiClient.UploadFile(baseName, bytes.NewReader(fileData)); err != nil {
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
