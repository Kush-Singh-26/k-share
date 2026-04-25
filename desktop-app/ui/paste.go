package ui

import (
	"context"
	"time"
)

// pasteAndSend reads clipboard and uploads content (screenshot or files)
func (a *App) pasteAndSend() {
	imgData := a.clipOps.ReadSystemImageClipboard()
	if imgData != nil && len(imgData) > 0 {
		if _, err := a.clipOps.UploadScreenshotWithPrefix(context.Background(), imgData, a.currentPath); err != nil {
			a.setStatus("🔴 Upload failed: "+err.Error(), 5*time.Second)
			return
		}
		a.setStatus("✅ Uploaded screenshot", 3*time.Second)
		a.loadFiles()
		return
	}

	text, _ := a.clipboardText.Get()
	if text != "" {
		a.pushClipboard()
		a.setStatus("✅ Text pushed", 3*time.Second)
		return
	}

	a.setStatus("📋 Nothing to paste", 3*time.Second)
}
