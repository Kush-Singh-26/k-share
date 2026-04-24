package clipboardstore

import (
	"os"
	"testing"
)

func TestReadMissingTextReturnsEmpty(t *testing.T) {
	store := New(t.TempDir())
	data, err := store.ReadText("clipboard.txt")
	if err != nil {
		t.Fatalf("ReadText() returned error: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("ReadText() len = %d, want 0", len(data))
	}
}

func TestWriteTextAppend(t *testing.T) {
	store := New(t.TempDir())
	if err := store.WriteText("clipboard.txt", []byte("hello"), false); err != nil {
		t.Fatalf("WriteText() returned error: %v", err)
	}
	if err := store.WriteText("clipboard.txt", []byte("world"), true); err != nil {
		t.Fatalf("WriteText() append returned error: %v", err)
	}
	data, err := os.ReadFile(store.TextPath("clipboard.txt"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != "hello\nworld" {
		t.Fatalf("WriteText() append = %q, want %q", string(data), "hello\nworld")
	}
}

func TestWriteAndReadImage(t *testing.T) {
	store := New(t.TempDir())
	if err := store.WriteImage([]byte{1, 2, 3}); err != nil {
		t.Fatalf("WriteImage() returned error: %v", err)
	}
	data, err := store.ReadImage()
	if err != nil {
		t.Fatalf("ReadImage() returned error: %v", err)
	}
	if len(data) != 3 {
		t.Fatalf("ReadImage() len = %d, want 3", len(data))
	}
}
