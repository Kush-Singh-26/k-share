package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png" // Explicit import for png.Encode
	"k-share-client/config"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// Thumbnail cache
var thumbnailCache = make(map[string]image.Image)
var cacheMutex sync.RWMutex

// Widget target map to safely handle list recycling
// Maps pointer of Image widget -> Filename it SHOULD display
var widgetTargets = make(map[*canvas.Image]string)
var targetMutex sync.Mutex

var cacheDir string

func init() {
	// Setup cache directory
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = "."
	}
	cacheDir = filepath.Join(appData, "k-share-client", "cache")
	os.MkdirAll(cacheDir, 0755)
}

func isImageFile(filename string) bool {
	lower := strings.ToLower(filename)
	return strings.HasSuffix(lower, ".jpg") ||
		strings.HasSuffix(lower, ".jpeg") ||
		strings.HasSuffix(lower, ".png") ||
		strings.HasSuffix(lower, ".gif") ||
		strings.HasSuffix(lower, ".webp") ||
		strings.HasSuffix(lower, ".bmp")
}

// getFileIcon returns an emoji icon based on file type
func getFileIcon(filename string, isDirectory bool) string {
	if isDirectory {
		return "📁"
	}

	lower := strings.ToLower(filename)

	// Documents
	if strings.HasSuffix(lower, ".pdf") {
		return "📕"
	}
	if strings.HasSuffix(lower, ".doc") || strings.HasSuffix(lower, ".docx") {
		return "📄"
	}
	if strings.HasSuffix(lower, ".xls") || strings.HasSuffix(lower, ".xlsx") {
		return "📊"
	}
	if strings.HasSuffix(lower, ".ppt") || strings.HasSuffix(lower, ".pptx") {
		return "📽️"
	}
	if strings.HasSuffix(lower, ".txt") || strings.HasSuffix(lower, ".md") {
		return "📝"
	}

	// Media
	if strings.HasSuffix(lower, ".mp3") || strings.HasSuffix(lower, ".wav") || strings.HasSuffix(lower, ".flac") {
		return "🎵"
	}
	if strings.HasSuffix(lower, ".mp4") || strings.HasSuffix(lower, ".avi") || strings.HasSuffix(lower, ".mkv") || strings.HasSuffix(lower, ".mov") {
		return "🎬"
	}

	// Archives
	if strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".rar") || strings.HasSuffix(lower, ".7z") || strings.HasSuffix(lower, ".tar") {
		return "📦"
	}

	// Code
	if strings.HasSuffix(lower, ".go") || strings.HasSuffix(lower, ".py") || strings.HasSuffix(lower, ".js") || strings.HasSuffix(lower, ".java") || strings.HasSuffix(lower, ".cpp") {
		return "💻"
	}

	// Images (will show thumbnail)
	if isImageFile(filename) {
		return "🖼️"
	}

	return "📄"
}

// setThumbnailTarget registers which file this widget is supposed to show
func (a *App) setThumbnailTarget(imgWidget *canvas.Image, filename string) {
	targetMutex.Lock()
	defer targetMutex.Unlock()
	widgetTargets[imgWidget] = filename
}

func (a *App) loadThumbnail(filename string, imgWidget *canvas.Image) {
	// 1. Check Memory Cache
	cacheMutex.RLock()
	cached, ok := thumbnailCache[filename]
	cacheMutex.RUnlock()

	if ok {
		applyImage(a, imgWidget, filename, cached)
		return
	}

	// 2. Check Disk Cache
	hashedName := hashFilename(filename)
	cachePath := filepath.Join(cacheDir, hashedName+".png")

	if _, err := os.Stat(cachePath); err == nil {
		// Found on disk
		file, err := os.Open(cachePath)
		if err == nil {
			img, _, err := image.Decode(file)
			file.Close()
			if err == nil {
				// Update memory cache
				cacheMutex.Lock()
				thumbnailCache[filename] = img
				cacheMutex.Unlock()

				applyImage(a, imgWidget, filename, img)
				return
			}
		}
	}

	// 3. Download
	thumbURL := fmt.Sprintf("%s/thumbnail?folder=fromphone&name=%s",
		"http://"+config.Current.ServerIP,
		url.QueryEscape(filename))

	resp, err := http.Get(thumbURL)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return
	}

	// Decode
	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return
	}

	// 4. Save to Disk (Async)
	go func(img image.Image, path string) {
		out, err := os.Create(path)
		if err != nil {
			return
		}
		defer out.Close()
		png.Encode(out, img)
	}(img, cachePath)

	// Update memory cache
	cacheMutex.Lock()
	thumbnailCache[filename] = img
	cacheMutex.Unlock()

	applyImage(a, imgWidget, filename, img)
}

func applyImage(a *App, imgWidget *canvas.Image, filename string, img image.Image) {
	fyne.Do(func() {
		targetMutex.Lock()
		target := widgetTargets[imgWidget]
		targetMutex.Unlock()

		if target == filename {
			imgWidget.Image = img
			imgWidget.Refresh()
		}
	})
}

func hashFilename(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
