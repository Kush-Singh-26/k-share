package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCreatesDefaultConfig(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempRoot, "config"))
	t.Setenv("HOME", filepath.Join(tempRoot, "profile"))
	t.Setenv("APPDATA", filepath.Join(tempRoot, "appdata"))
	t.Setenv("USERPROFILE", filepath.Join(tempRoot, "profile"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Port != "26260" {
		t.Fatalf("Load() Port = %q, want 26260", cfg.Port)
	}
	if len(cfg.GuestCode) != DefaultCodeLength {
		t.Fatalf("Load() GuestCode = %q, want length %d", cfg.GuestCode, DefaultCodeLength)
	}
	if cfg.AdminCode == "" {
		t.Fatal("Load() AdminCode is empty")
	}

	if _, err := os.Stat(Path()); err != nil {
		t.Fatalf("expected config file to be created: %v", err)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempRoot, "config"))
	t.Setenv("HOME", filepath.Join(tempRoot, "profile"))
	t.Setenv("APPDATA", filepath.Join(tempRoot, "appdata"))
	t.Setenv("USERPROFILE", filepath.Join(tempRoot, "profile"))

	want := Config{
		Port:      "12345",
		SharedDir: filepath.Join(tempRoot, "share"),
		AdminCode: "admin123",
		GuestCode: "guest123",
	}
	if err := Save(want); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}
