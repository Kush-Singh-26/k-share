package fileops

import (
	"desktop-app/api"
	"os"
	"path/filepath"
	"testing"
)

func TestPlanDownloadTargetsFile(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "sample.txt")
	gotPath, gotExtract := DownloadPathForTest(base, api.FileInfo{Name: "sample.txt"})
	if gotPath != base {
		t.Fatalf("expected %q, got %q", base, gotPath)
	}
	if gotExtract != "" {
		t.Fatalf("expected empty extract folder, got %q", gotExtract)
	}
}

func TestPlanDownloadTargetsDirectory(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "folder")

	if err := os.WriteFile(base, []byte("existing"), 0o600); err != nil {
		t.Fatalf("write base: %v", err)
	}

	gotPath, gotExtract := DownloadPathForTest(base, api.FileInfo{Name: "folder", IsDirectory: true})
	if gotPath != filepath.Join(dir, "folder (1).zip") {
		t.Fatalf("unexpected path: %q", gotPath)
	}
	if gotExtract != filepath.Join(dir, "folder (1)") {
		t.Fatalf("unexpected extract folder: %q", gotExtract)
	}
}
