package ui

import "context"

// pasteAndSend reads clipboard and uploads content (screenshot or files)
func (a *App) pasteAndSend() {
	imgData := a.clipOps.ReadSystemImageClipboard()
	if imgData != nil && len(imgData) > 0 {
		if _, err := a.clipOps.UploadScreenshot(context.Background(), imgData); err != nil {
			a.statusText.Set("🔴 Upload failed: " + err.Error())
			return
		}
		a.statusText.Set("✅ Uploaded screenshot")
		return
	}

	text, _ := a.clipboardText.Get()
	if text != "" {
		a.pushClipboard()
		a.statusText.Set("✅ Text pushed")
		return
	}

	a.statusText.Set("📋 Nothing to paste")
}
