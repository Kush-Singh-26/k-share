package presentation

import (
	"desktop-app/api"
	"testing"
	"time"
)

func TestBuildFileRow(t *testing.T) {
	row := BuildFileRow(api.FileInfo{
		Name:        "Public/photo.png",
		IsDirectory: false,
		Size:        2048,
		ModTime:     "2026-04-23 12:00:00",
	}, true)

	if row.DisplayName != "photo.png" {
		t.Fatalf("unexpected display name: %q", row.DisplayName)
	}
	if row.GuestLabel != " [Guest]" {
		t.Fatalf("unexpected guest label: %q", row.GuestLabel)
	}
	if !row.IsImage {
		t.Fatal("expected image row")
	}
	if row.Info != "2.0 KB • 2026-04-23 12:00:00" {
		t.Fatalf("unexpected info: %q", row.Info)
	}
}

func TestHistoryTimestamp(t *testing.T) {
	got := HistoryTimestamp(time.Date(2026, 4, 23, 15, 4, 5, 0, time.UTC))
	if got != "2026-04-23 15:04:05" {
		t.Fatalf("unexpected timestamp: %q", got)
	}
}
