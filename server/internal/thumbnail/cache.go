package thumbnail

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	serverstorage "github.com/Kush-Singh-26/k-share/server/internal/storage"

	"github.com/nfnt/resize"
)

type Store struct {
	cacheDir string
	queue    chan genRequest
	stopCh   chan struct{}
	mu       sync.Mutex
}

type genRequest struct {
	rootDir string
	relPath string
}

func New() *Store {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		exe, _ := os.Executable()
		cacheDir = filepath.Join(filepath.Dir(exe), ".thumbnails")
	} else {
		cacheDir = filepath.Join(cacheDir, "K-Share", "thumbnails")
	}
	return NewWithCacheDir(cacheDir)
}

func NewWithCacheDir(cacheDir string) *Store {
	_ = os.MkdirAll(cacheDir, 0o755)
	s := &Store{
		cacheDir: cacheDir,
		queue:    make(chan genRequest, 64),
		stopCh:   make(chan struct{}),
	}
	// Start a pool of workers for concurrent thumbnail generation
	for i := 0; i < 4; i++ {
		go s.ProcessQueue()
	}
	return s
}

func (s *Store) CacheDir() string {
	return s.cacheDir
}

func (s *Store) CachePath(relPath string, modTime time.Time, size int64) string {
	hash := sha256.New()
	hash.Write([]byte(relPath))
	hash.Write([]byte(fmt.Sprintf("%d", modTime.UnixNano())))
	hash.Write([]byte(fmt.Sprintf("%d", size)))
	return filepath.Join(s.cacheDir, hex.EncodeToString(hash.Sum(nil))+".jpg")
}

func (s *Store) Serve(rootDir, relPath string, w http.ResponseWriter, r *http.Request) error {
	filePath, err := serverstorage.ResolveWithinRoot(rootDir, relPath)
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return nil
	}

	info, err := os.Stat(filePath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return nil
	}

	relFromRoot, _ := filepath.Rel(rootDir, filePath)
	if relFromRoot == "" {
		relFromRoot = relPath
	}

	cachePath := s.CachePath(relFromRoot, info.ModTime(), info.Size())
	if _, err := os.Stat(cachePath); err == nil {
		// Update access time for LRU eviction
		_ = os.Chtimes(cachePath, time.Now(), time.Now())
		w.Header().Set("Content-Type", "image/jpeg")
		http.ServeFile(w, r, cachePath)
		return nil
	}

	// Queue async generation; for now return a placeholder response so the request doesn't block.
	s.QueueGeneration(rootDir, relPath)
	http.Error(w, "Thumbnail not ready", http.StatusAccepted)
	return nil
}

func (s *Store) generate(rootDir, relPath string) {
	filePath, err := serverstorage.ResolveWithinRoot(rootDir, relPath)
	if err != nil {
		return
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return
	}
	// DoS Safety: Don't try to decode images larger than 25MB
	if info.Size() > 25*1024*1024 {
		return
	}
	relFromRoot, _ := filepath.Rel(rootDir, filePath)
	if relFromRoot == "" {
		relFromRoot = relPath
	}
	cachePath := s.CachePath(relFromRoot, info.ModTime(), info.Size())
	if _, err := os.Stat(cachePath); err == nil {
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(relPath))
	var img image.Image
	if ext == ".jpg" || ext == ".jpeg" {
		img, _ = jpeg.Decode(file)
	} else if ext == ".png" {
		img, _ = png.Decode(file)
	}
	if img == nil {
		return
	}

	thumb := resize.Thumbnail(128, 128, img, resize.Lanczos3)
	if outFile, err := os.Create(cachePath); err == nil {
		_ = jpeg.Encode(outFile, thumb, nil)
		_ = outFile.Close()
	}
}

// QueueGeneration adds a thumbnail generation job to the async queue.
func (s *Store) QueueGeneration(rootDir, relPath string) {
	select {
	case <-s.stopCh:
		return
	case s.queue <- genRequest{rootDir: rootDir, relPath: relPath}:
	default:
		// Queue is full; drop the request silently.
	}
}

// ProcessQueue runs in a background goroutine and processes generation jobs.
func (s *Store) ProcessQueue() {
	for {
		select {
		case req, ok := <-s.queue:
			if !ok {
				return
			}
			s.generate(req.rootDir, req.relPath)
		case <-s.stopCh:
			return
		}
	}
}

func (s *Store) Stop() {
	close(s.stopCh)
}

func (s *Store) Evict(maxSize int64) {
	var totalSize int64
	entries, err := os.ReadDir(s.cacheDir)
	if err != nil {
		return
	}

	type fileInfo struct {
		name string
		size int64
		time time.Time
	}
	var files []fileInfo

	for _, entry := range entries {
		info, err := entry.Info()
		if err == nil && !info.IsDir() {
			files = append(files, fileInfo{entry.Name(), info.Size(), info.ModTime()})
			totalSize += info.Size()
		}
	}

	if totalSize <= maxSize {
		return
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].time.Before(files[j].time)
	})

	for _, file := range files {
		if totalSize <= maxSize {
			break
		}
		if err := os.Remove(filepath.Join(s.cacheDir, file.name)); err == nil {
			totalSize -= file.size
		}
	}
}

func (s *Store) StartEviction(maxSize int64, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.Evict(maxSize)
		case <-s.stopCh:
			return
		}
	}
}
