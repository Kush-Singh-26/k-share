package ui

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"k-share-client/config"
	"k-share-client/crypto"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

const (
	maxCacheSize   = 100 // Maximum thumbnails in memory
	workerPoolSize = 4   // Concurrent thumbnail downloads
)

// LRU Cache for thumbnails
type lruCache struct {
	capacity int
	cache    map[string]*list.Element
	order    *list.List
	mu       sync.Mutex
}

type cacheEntry struct {
	key   string
	image image.Image
}

func newLRUCache(capacity int) *lruCache {
	return &lruCache{
		capacity: capacity,
		cache:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

func (c *lruCache) Get(key string) (image.Image, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[key]; ok {
		c.order.MoveToFront(elem)
		return elem.Value.(*cacheEntry).image, true
	}
	return nil, false
}

func (c *lruCache) Put(key string, img image.Image) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[key]; ok {
		c.order.MoveToFront(elem)
		elem.Value.(*cacheEntry).image = img
		return
	}

	// Evict oldest if at capacity
	if c.order.Len() >= c.capacity {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			delete(c.cache, oldest.Value.(*cacheEntry).key)
		}
	}

	elem := c.order.PushFront(&cacheEntry{key: key, image: img})
	c.cache[key] = elem
}

// Thumbnail worker pool
type thumbnailJob struct {
	filename  string
	imgWidget *canvas.Image
	app       *App
}

var (
	thumbnailCache = newLRUCache(maxCacheSize)
	jobQueue       = make(chan thumbnailJob, 50)
	workersStarted = false
	workerMu       sync.Mutex

	// Widget target map to handle list recycling
	widgetTargets = make(map[*canvas.Image]string)
	targetMutex   sync.Mutex

	cacheDir   string
	httpClient *http.Client
)

func init() {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = "."
	}
	cacheDir = filepath.Join(appData, "k-share-client", "cache")
	os.MkdirAll(cacheDir, 0755)

	// Shared HTTP client with TOFU TLS
	httpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: crypto.CreateTLSConfig(nil),
		},
	}
}

func startWorkers() {
	workerMu.Lock()
	defer workerMu.Unlock()

	if workersStarted {
		return
	}
	workersStarted = true

	for i := 0; i < workerPoolSize; i++ {
		go thumbnailWorker()
	}
}

func thumbnailWorker() {
	for job := range jobQueue {
		loadThumbnailSync(job.app, job.filename, job.imgWidget)
	}
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

func getFileIcon(filename string, isDirectory bool) string {
	if isDirectory {
		return "📁"
	}

	lower := strings.ToLower(filename)

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
	if strings.HasSuffix(lower, ".mp3") || strings.HasSuffix(lower, ".wav") || strings.HasSuffix(lower, ".flac") {
		return "🎵"
	}
	if strings.HasSuffix(lower, ".mp4") || strings.HasSuffix(lower, ".avi") || strings.HasSuffix(lower, ".mkv") || strings.HasSuffix(lower, ".mov") {
		return "🎬"
	}
	if strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".rar") || strings.HasSuffix(lower, ".7z") || strings.HasSuffix(lower, ".tar") {
		return "📦"
	}
	if strings.HasSuffix(lower, ".go") || strings.HasSuffix(lower, ".py") || strings.HasSuffix(lower, ".js") || strings.HasSuffix(lower, ".java") || strings.HasSuffix(lower, ".cpp") {
		return "💻"
	}
	if isImageFile(filename) {
		return "🖼️"
	}

	return "📄"
}

func (a *App) setThumbnailTarget(imgWidget *canvas.Image, filename string) {
	targetMutex.Lock()
	defer targetMutex.Unlock()
	widgetTargets[imgWidget] = filename
}

// loadThumbnail queues a thumbnail load job (non-blocking)
func (a *App) loadThumbnail(filename string, imgWidget *canvas.Image) {
	startWorkers()

	// Check memory cache first (fast path)
	if img, ok := thumbnailCache.Get(filename); ok {
		applyImage(a, imgWidget, filename, img)
		return
	}

	// Queue for async loading
	select {
	case jobQueue <- thumbnailJob{filename: filename, imgWidget: imgWidget, app: a}:
	default:
		// Queue full, skip this thumbnail
	}
}

// loadThumbnailSync performs the actual thumbnail loading (called by workers)
func loadThumbnailSync(a *App, filename string, imgWidget *canvas.Image) {
	// Double-check memory cache
	if img, ok := thumbnailCache.Get(filename); ok {
		applyImage(a, imgWidget, filename, img)
		return
	}

	// Check disk cache
	hashedName := hashFilename(filename)
	cachePath := filepath.Join(cacheDir, hashedName+".png")

	if _, err := os.Stat(cachePath); err == nil {
		file, err := os.Open(cachePath)
		if err == nil {
			img, _, err := image.Decode(file)
			file.Close()
			if err == nil {
				thumbnailCache.Put(filename, img)
				applyImage(a, imgWidget, filename, img)
				return
			}
		}
	}

	// Download from server
	thumbURL := fmt.Sprintf("%s/thumbnail?name=%s",
		"https://"+config.Current.ServerIP,
		url.QueryEscape(filename))

	req, _ := http.NewRequest("GET", thumbURL, nil)
	req.Header.Set("Authorization", "Bearer "+config.Current.PairingCode)

	resp, err := httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return
	}

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return
	}

	// Save to disk cache (async)
	go func(img image.Image, path string) {
		out, err := os.Create(path)
		if err != nil {
			return
		}
		defer out.Close()
		png.Encode(out, img)
	}(img, cachePath)

	thumbnailCache.Put(filename, img)
	applyImage(a, imgWidget, filename, img)
}

func applyImage(a *App, imgWidget *canvas.Image, filename string, img image.Image) {
	fyne.Do(func() {
		targetMutex.Lock()
		target := widgetTargets[imgWidget]
		targetMutex.Unlock()

		if target == filename {
			imgWidget.Resource = nil
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
