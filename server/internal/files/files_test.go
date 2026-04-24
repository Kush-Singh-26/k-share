package files

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"net/http/httptest"
)

func TestListSkipsHiddenFolders(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Public"), 0o755); err != nil {
		t.Fatalf("failed to create public dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".trash"), 0o755); err != nil {
		t.Fatalf("failed to create trash dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "visible.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "Public", "shared.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to create public file: %v", err)
	}

	entries, err := List(root, "")
	if err != nil {
		t.Fatalf("List() returned error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
}

func TestDeleteMovesFileToTrash(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(source, []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	if err := Delete(root, "sample.txt"); err != nil {
		t.Fatalf("Delete() returned error: %v", err)
	}

	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("expected source file to be moved, got err=%v", err)
	}

	entries, err := os.ReadDir(filepath.Join(root, ".trash"))
	if err != nil {
		t.Fatalf("failed to read trash dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("trash entries = %d, want 1", len(entries))
	}
}

func TestUploadCreatesFile(t *testing.T) {
	root := t.TempDir()
	if err := Upload(root, "upload.txt", bytes.NewBufferString("hello")); err != nil {
		t.Fatalf("Upload() returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "upload.txt"))
	if err != nil {
		t.Fatalf("failed to read uploaded file: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("uploaded contents = %q, want hello", string(data))
	}
}

func TestServeDownloadFile(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(source, []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "https://example.com/download/sample.txt", nil)
	if err := ServeDownload(root, "sample.txt", rec, req); err != nil {
		t.Fatalf("ServeDownload() returned error: %v", err)
	}

	if rec.Code != 200 {
		t.Fatalf("ServeDownload() status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Disposition"); got == "" {
		t.Fatal("ServeDownload() missing content-disposition header")
	}
	if rec.Body.Len() == 0 {
		t.Fatal("ServeDownload() returned empty body")
	}
}

func TestServeDownloadFolder(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "folder")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatalf("failed to create folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "https://example.com/download/folder", nil)
	if err := ServeDownload(root, "folder", rec, req); err != nil {
		t.Fatalf("ServeDownload() returned error: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("expected zip output, got error: %v", err)
	}
	if len(zr.File) == 0 {
		t.Fatal("expected zip archive to contain files")
	}
}
