package thumbnail

import (
	"crypto/sha256"
	"encoding/hex"
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
	mu       sync.Mutex
}

type genRequest struct {
	rootDir string
	name    string
	folder  string
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
	}
	go s.ProcessQueue()
	return s
}

func (s *Store) CacheDir() string {
	return s.cacheDir
}

func (s *Store) CachePath(relFilePath string, modTime time.Time) string {
	h := sha256.New()
	h.Write([]byte(relFilePath))
	h.Write([]byte(modTime.String()))
	hash := hex.EncodeToString(h.Sum(nil))
	return filepath.Join(s.cacheDir, hash+".jpg")
}

func (s *Store) Serve(rootDir, name, folder string, w http.ResponseWriter, r *http.Request) error {
	relPath := filepath.Join(folder, name)

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
		relFromRoot = name
	}

	cachePath := s.CachePath(relFromRoot, info.ModTime())
	if _, err := os.Stat(cachePath); err == nil {
		w.Header().Set("Content-Type", "image/jpeg")
		http.ServeFile(w, r, cachePath)
		return nil
	}

	// Queue async generation; for now return a placeholder response so the request doesn't block.
	s.QueueGeneration(rootDir, name, folder)
	http.Error(w, "Thumbnail not ready", http.StatusAccepted)
	return nil
}

func (s *Store) generate(rootDir, name, folder string) {
	relPath := filepath.Join(folder, name)
	filePath, err := serverstorage.ResolveWithinRoot(rootDir, relPath)
	if err != nil {
		return
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return
	}
	relFromRoot, _ := filepath.Rel(rootDir, filePath)
	if relFromRoot == "" {
		relFromRoot = name
	}
	cachePath := s.CachePath(relFromRoot, info.ModTime())
	if _, err := os.Stat(cachePath); err == nil {
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(name))
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
func (s *Store) QueueGeneration(rootDir, name, folder string) {
	select {
	case s.queue <- genRequest{rootDir: rootDir, name: name, folder: folder}:
	default:
		// Queue is full; drop the request silently.
	}
}

// ProcessQueue runs in a background goroutine and processes generation jobs.
func (s *Store) ProcessQueue() {
	for req := range s.queue {
		s.generate(req.rootDir, req.name, req.folder)
	}
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
	for {
		time.Sleep(interval)
		s.Evict(maxSize)
	}
}
