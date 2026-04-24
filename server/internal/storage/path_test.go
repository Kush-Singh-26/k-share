package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWithinRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "share")
	got, err := ResolveWithinRoot(root, filepath.Join("nested", "file.txt"))
	if err != nil {
		t.Fatalf("ResolveWithinRoot() returned error: %v", err)
	}
	want := filepath.Join(root, "nested", "file.txt")
	if got != want {
		t.Fatalf("ResolveWithinRoot() = %q, want %q", got, want)
	}
}

func TestResolveWithinRootRejectsTraversal(t *testing.T) {
	root := filepath.Join(t.TempDir(), "share")
	if _, err := ResolveWithinRoot(root, filepath.Join("..", "escape.txt")); err == nil {
		t.Fatal("ResolveWithinRoot() accepted traversal path")
	}
}

func TestUniqueFilename(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(first, []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	got := UniqueFilename(first)
	if got == first {
		t.Fatal("UniqueFilename() returned existing path")
	}
}
