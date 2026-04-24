package search

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Kush-Singh-26/k-share/server/internal/domain"
)

// Index holds an in-memory index of file names for fast search.
type Index struct {
	mu       sync.RWMutex
	rootDir  string
	entries  []domain.SearchResult
	lastScan time.Time
}

// NewIndex creates a new search index for the given root directory.
func NewIndex(rootDir string) *Index {
	return &Index{rootDir: rootDir, entries: []domain.SearchResult{}}
}

// Build walks the root directory and rebuilds the in-memory index.
func (idx *Index) Build() error {
	var entries []domain.SearchResult
	err := filepath.Walk(idx.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if path == idx.rootDir {
			return nil
		}
		name := info.Name()
		if name == ".trash" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(idx.rootDir, path)
		entries = append(entries, domain.SearchResult{
			Name:        name,
			Path:        filepath.ToSlash(rel),
			IsDirectory: info.IsDir(),
			Size:        info.Size(),
			ModTime:     info.ModTime().Format("2006-01-02 15:04:05"),
		})
		return nil
	})
	if err != nil {
		return err
	}
	idx.mu.Lock()
	idx.entries = entries
	idx.lastScan = time.Now()
	idx.mu.Unlock()
	return nil
}

// Query returns search results matching the given query string.
// Matching is case-insensitive on the file/directory name.
func (idx *Index) Query(q string) []domain.SearchResult {
	q = strings.ToLower(strings.TrimSpace(q))
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if q == "" {
		return []domain.SearchResult{}
	}
	var results []domain.SearchResult
	for _, e := range idx.entries {
		if strings.Contains(strings.ToLower(e.Name), q) {
			results = append(results, e)
		}
	}
	return results
}

// NotifyUpdate triggers a background rebuild of the index.
func (idx *Index) NotifyUpdate() {
	go idx.Build()
}

// LastScan returns the time of the last successful index build.
func (idx *Index) LastScan() time.Time {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.lastScan
}

// Worker periodically rebuilds the index.
type Worker struct {
	idx      *Index
	interval time.Duration
	stopCh   chan struct{}
}

// NewWorker creates a background re-index worker.
func NewWorker(idx *Index, interval time.Duration) *Worker {
	return &Worker{idx: idx, interval: interval, stopCh: make(chan struct{})}
}

// Start begins the periodic re-index loop.
func (w *Worker) Start() {
	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = w.idx.Build()
			case <-w.stopCh:
				return
			}
		}
	}()
}

// Stop halts the worker.
func (w *Worker) Stop() {
	close(w.stopCh)
}
