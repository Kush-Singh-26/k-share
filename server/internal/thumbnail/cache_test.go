package thumbnail

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCachePathDeterministic(t *testing.T) {
	store := NewWithCacheDir(t.TempDir())
	modTime := time.Unix(123, 0)
	first := store.CachePath("file.png", modTime)
	second := store.CachePath("file.png", modTime)
	if first != second {
		t.Fatalf("CachePath() = %q and %q, want same value", first, second)
	}
}

func TestServeGeneratesThumbnail(t *testing.T) {
	cacheDir := t.TempDir()
	store := NewWithCacheDir(cacheDir)

	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "image.png")
	if err := writePNG(filePath); err != nil {
		t.Fatalf("failed to create source image: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.com/thumbnail?name=image.png", nil)
	rec := httptest.NewRecorder()
	if err := store.Serve(rootDir, "image.png", "", rec, req); err != nil {
		t.Fatalf("Serve() returned error: %v", err)
	}

	if rec.Code != http.StatusAccepted {
		t.Fatalf("Serve() first call status = %d, want 202", rec.Code)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("failed to stat source image: %v", err)
	}

	// Wait for async generation
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(store.CachePath("image.png", info.ModTime())); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Second call should return 200 OK
	req = httptest.NewRequest(http.MethodGet, "https://example.com/thumbnail?name=image.png", nil)
	rec = httptest.NewRecorder()
	if err := store.Serve(rootDir, "image.png", "", rec, req); err != nil {
		t.Fatalf("Serve() second call returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("Serve() second call status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("Serve() content-type = %q, want image/jpeg", got)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("Serve() returned empty body")
	}

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("failed to read cache dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("cache entries = %d, want 1", len(entries))
	}
}

func TestEvictRemovesOldestFiles(t *testing.T) {
	cacheDir := t.TempDir()
	store := NewWithCacheDir(cacheDir)

	oldFile := filepath.Join(cacheDir, "old.jpg")
	newFile := filepath.Join(cacheDir, "new.jpg")
	if err := os.WriteFile(oldFile, []byte("old"), 0o644); err != nil {
		t.Fatalf("failed to create old file: %v", err)
	}
	if err := os.WriteFile(newFile, []byte("new"), 0o644); err != nil {
		t.Fatalf("failed to create new file: %v", err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now()
	_ = os.Chtimes(oldFile, oldTime, oldTime)
	_ = os.Chtimes(newFile, newTime, newTime)

	store.Evict(1)

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("expected old file to be evicted, got err=%v", err)
	}
	if _, err := os.Stat(newFile); !os.IsNotExist(err) {
		t.Fatalf("expected new file to be evicted as cache exceeded limit, got err=%v", err)
	}
}

func writePNG(path string) error {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
