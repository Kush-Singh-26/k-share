package history

import (
	"os"
	"testing"
)

func TestAddAndList(t *testing.T) {
	appDir := t.TempDir()
	s := New(appDir)

	if err := s.Add("hello"); err != nil {
		t.Fatalf("Add() returned error: %v", err)
	}

	items, err := s.List()
	if err != nil {
		t.Fatalf("List() returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("List() len = %d, want 1", len(items))
	}
	if items[0].Text != "hello" {
		t.Fatalf("List() text = %q, want hello", items[0].Text)
	}
}

func TestDelete(t *testing.T) {
	appDir := t.TempDir()
	s := New(appDir)

	if err := s.Add("hello"); err != nil {
		t.Fatalf("Add() returned error: %v", err)
	}
	items, _ := s.List()
	if len(items) != 1 {
		t.Fatalf("expected one history item, got %d", len(items))
	}

	if err := s.Delete(items[0].ID); err != nil {
		t.Fatalf("Delete() returned error: %v", err)
	}

	items, err := s.List()
	if err != nil {
		t.Fatalf("List() returned error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no history items, got %d", len(items))
	}
}

func TestListMissingFile(t *testing.T) {
	appDir := t.TempDir()
	s := New(appDir)

	items, err := s.List()
	if err != nil {
		t.Fatalf("List() returned error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty list for missing file, got %d items", len(items))
	}
}

func TestSaveWritesFile(t *testing.T) {
	appDir := t.TempDir()
	s := New(appDir)

	err := s.save() // Initial save works via internal logic
	if err != nil {
		t.Fatalf("save() returned error: %v", err)
	}
	if _, err := os.Stat(s.Path()); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}
