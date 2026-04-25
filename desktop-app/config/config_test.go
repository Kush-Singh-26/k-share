package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func resetConfigForTest(t *testing.T) string {
	t.Helper()

	oldPath := configPath
	mu.Lock()
	oldCurrent := current
	mu.Unlock()

	dir := t.TempDir()
	configPath = filepath.Join(dir, "settings.json")
	
	mu.Lock()
	current = &Config{
		ServerIP:          "localhost:9823",
		DownloadFolder:    ".",
		AutoSyncClipboard: true,
		SavedNetworks:     make(map[string]string),
		KnownServers:      make(map[string]ServerIdentity),
	}
	mu.Unlock()

	t.Cleanup(func() {
		configPath = oldPath
		mu.Lock()
		current = oldCurrent
		mu.Unlock()
	})

	return configPath
}

func TestLoadFromPathAppliesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"server_ip":"10.0.0.5:26260"}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := &Config{}
	if err := loadFromPath(path, cfg); err != nil {
		t.Fatalf("loadFromPath: %v", err)
	}

	if cfg.ServerIP != "10.0.0.5:26260" {
		t.Fatalf("unexpected server ip: %q", cfg.ServerIP)
	}
	if cfg.SavedNetworks == nil || cfg.KnownServers == nil {
		t.Fatal("expected maps to be initialized")
	}
	if cfg.DownloadFolder == "" {
		t.Fatal("expected download folder to be defaulted")
	}
}

func TestKnownServerPersistence(t *testing.T) {
	path := resetConfigForTest(t)

	mu.Lock()
	current.KnownServers = nil
	current.SavedNetworks = nil
	mu.Unlock()

	identity := ServerIdentity{
		CertHash:    "abc123",
		AuthCode:    "422974",
		LastIP:      "192.168.1.6",
		DisplayName: "desk",
	}

	if err := SetKnownServer(identity.CertHash, identity); err != nil {
		t.Fatalf("SetKnownServer: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got, ok := cfg.KnownServers[identity.CertHash]; !ok || got != identity {
		t.Fatalf("expected known server to persist, got %#v", cfg.KnownServers)
	}

	if err := RemoveKnownServer(identity.CertHash); err != nil {
		t.Fatalf("RemoveKnownServer: %v", err)
	}

	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config after remove: %v", err)
	}
	cfg = Config{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal after remove: %v", err)
	}
	if len(cfg.KnownServers) != 0 {
		t.Fatalf("expected no known servers after remove, got %#v", cfg.KnownServers)
	}
}
