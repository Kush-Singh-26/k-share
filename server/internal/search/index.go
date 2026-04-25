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
		if strings.HasPrefix(name, ".") {
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
	count := 0
	for _, e := range idx.entries {
		if strings.Contains(strings.ToLower(e.Name), q) {
			results = append(results, e)
			count++
			if count >= 100 {
				break
			}
		}
	}
	return results
}

// LastScan returns the time of the last successful index build.
func (idx *Index) LastScan() time.Time {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.lastScan
}
