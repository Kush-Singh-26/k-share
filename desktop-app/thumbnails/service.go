package thumbnails

import (
	"bytes"
	"context"
	"container/list"
	"crypto/sha256"
	"desktop-app/api"
	"desktop-app/presentation"
	"encoding/hex"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

const (
	maxCacheSize   = 100
	workerPoolSize = 4
)

type cacheEntry struct {
	key   string
	image image.Image
}

type lruCache struct {
	capacity int
	cache    map[string]*list.Element
	order    *list.List
	mu       sync.Mutex
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

type Job struct {
	Filename  string
	ImgWidget *canvas.Image
}

type Service struct {
	client         *api.Client
	cache          *lruCache
	cacheDir       string
	jobQueue       chan Job
	workersStarted bool
	workerMu       sync.Mutex
	targets        map[*canvas.Image]string
	targetMu       sync.Mutex
}

func New(client *api.Client) *Service {
	cacheDir, err := os.UserCacheDir()
	if err != nil || cacheDir == "" {
		cacheDir = filepath.Join(".", "K-Share", "cache")
	} else {
		cacheDir = filepath.Join(cacheDir, "K-Share")
	}
	cacheDir = filepath.Join(cacheDir, "thumbnails")
	_ = os.MkdirAll(cacheDir, 0o755)

	return &Service{
		client:   client,
		cache:    newLRUCache(maxCacheSize),
		cacheDir: cacheDir,
		jobQueue: make(chan Job, 50),
		targets:  make(map[*canvas.Image]string),
	}
}

func (s *Service) StartWorkers() {
	s.workerMu.Lock()
	defer s.workerMu.Unlock()

	if s.workersStarted {
		return
	}
	s.workersStarted = true

	for i := 0; i < workerPoolSize; i++ {
		go s.worker()
	}
}

func (s *Service) SetTarget(widget *canvas.Image, filename string) {
	s.targetMu.Lock()
	defer s.targetMu.Unlock()
	s.targets[widget] = filename
}

func (s *Service) Request(filename string, imgWidget *canvas.Image) {
	s.StartWorkers()

	if img, ok := s.cache.Get(filename); ok {
		s.applyImage(imgWidget, filename, img)
		return
	}

	select {
	case s.jobQueue <- Job{Filename: filename, ImgWidget: imgWidget}:
	default:
	}
}

func (s *Service) IsImageFile(filename string) bool {
	return presentation.IsImageFile(filename)
}

func (s *Service) worker() {
	for job := range s.jobQueue {
		s.loadSync(job.Filename, job.ImgWidget)
	}
}

func (s *Service) loadSync(filename string, imgWidget *canvas.Image) {
	if img, ok := s.cache.Get(filename); ok {
		s.applyImage(imgWidget, filename, img)
		return
	}

	hashedName := hashFilename(filename)
	cachePath := filepath.Join(s.cacheDir, hashedName+".png")

	if _, err := os.Stat(cachePath); err == nil {
		file, err := os.Open(cachePath)
		if err == nil {
			img, _, err := image.Decode(file)
			_ = file.Close()
			if err == nil {
				s.cache.Put(filename, img)
				s.applyImage(imgWidget, filename, img)
				return
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	data, err := s.client.GetThumbnail(ctx, filename)
	if err != nil {
		return
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return
	}

	go func(img image.Image, path string) {
		out, err := os.Create(path)
		if err != nil {
			return
		}
		defer out.Close()
		_ = png.Encode(out, img)
	}(img, cachePath)

	s.cache.Put(filename, img)
	s.applyImage(imgWidget, filename, img)
}

func (s *Service) applyImage(imgWidget *canvas.Image, filename string, img image.Image) {
	fyne.Do(func() {
		s.targetMu.Lock()
		target := s.targets[imgWidget]
		s.targetMu.Unlock()

		if target == filename {
			imgWidget.Resource = nil
			imgWidget.Image = img
			imgWidget.Refresh()
		}
	})
}

func hashFilename(s string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func HashFilenameForTest(s string) string {
	return hashFilename(s)
}
