package search

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIndexBuildAndQuery(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "hello.txt"), []byte("hello"), 0o644)
	_ = os.WriteFile(filepath.Join(tmpDir, "world.png"), []byte("world"), 0o644)
	sub := filepath.Join(tmpDir, "subdir")
	_ = os.MkdirAll(sub, 0o755)
	_ = os.WriteFile(filepath.Join(sub, "nested.go"), []byte("code"), 0o644)

	idx := NewIndex(tmpDir)
	if err := idx.Build(); err != nil {
		t.Fatalf("build failed: %v", err)
	}

	results := idx.Query("hello")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'hello', got %d", len(results))
	}
	if results[0].Name != "hello.txt" {
		t.Errorf("expected hello.txt, got %s", results[0].Name)
	}

	results = idx.Query(".go")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for '.go', got %d", len(results))
	}
	if results[0].Name != "nested.go" {
		t.Errorf("expected nested.go, got %s", results[0].Name)
	}

	results = idx.Query("missing")
	if len(results) != 0 {
		t.Fatalf("expected 0 results for 'missing', got %d", len(results))
	}

	if idx.LastScan().IsZero() {
		t.Error("expected LastScan to be set")
	}
}

func TestIndexQueryEmpty(t *testing.T) {
	idx := NewIndex(t.TempDir())
	_ = idx.Build()
	results := idx.Query("")
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty query, got %d", len(results))
	}
}

func TestIndexCaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "Upper.TXT"), []byte("up"), 0o644)
	idx := NewIndex(tmpDir)
	_ = idx.Build()
	results := idx.Query("upper")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}
