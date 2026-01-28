package ui

import (
	"bytes"
	"time"

	"golang.design/x/clipboard"
)

// pasteAndSend reads clipboard and uploads content (screenshot or files)
func (a *App) pasteAndSend() {
	// Try image first (screenshots from Win+Shift+S, Print Screen, etc.)
	imgData := clipboard.Read(clipboard.FmtImage)
	if imgData != nil && len(imgData) > 0 {
		a.uploadClipboardImage(imgData)
		return
	}

	a.statusText.Set("📋 No image in clipboard")
}

// uploadClipboardImage saves clipboard image data and uploads it
func (a *App) uploadClipboardImage(data []byte) {
	// Generate filename: screenshot_2026-01-28_19-15-00.png
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := "screenshot_" + timestamp + ".png"

	a.statusText.Set("📤 Uploading screenshot...")

	go func() {
		err := a.apiClient.UploadFile(filename, bytes.NewReader(data))
		if err != nil {
			a.statusText.Set("🔴 Upload failed: " + err.Error())
			return
		}
		a.statusText.Set("✅ Uploaded: " + filename)
	}()
}
