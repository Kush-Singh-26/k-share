package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type ServerIdentity struct {
	CertHash    string `json:"cert_hash"`
	AuthCode    string `json:"auth_code"`
	LastIP      string `json:"last_ip"`
	DisplayName string `json:"display_name"`
}

type Config struct {
	ServerIP          string                    `json:"server_ip"`
	PairingCode       string                    `json:"pairing_code"` // Active authentication code (Admin or Guest)
	DownloadFolder    string                    `json:"download_folder"`
	AutoSyncClipboard bool                      `json:"auto_sync_clipboard"`
	SavedNetworks     map[string]string         `json:"saved_networks"`
	KnownServers      map[string]ServerIdentity `json:"known_servers"`
}

var (
	configPath string
	mu         sync.RWMutex
	current    = &Config{
		ServerIP:          "localhost:26260",
		DownloadFolder:    getDefaultDownloadFolder(),
		AutoSyncClipboard: true,
		SavedNetworks:     make(map[string]string),
		KnownServers:      make(map[string]ServerIdentity),
	}
)

func init() {
	configDir := getConfigDir()
	_ = os.MkdirAll(configDir, 0o755)
	configPath = filepath.Join(configDir, "settings.json")
}

func getDefaultDownloadFolder() string {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return "."
	}
	downloadFolder := filepath.Join(homeDir, "Downloads", "K-Share")
	_ = os.MkdirAll(downloadFolder, 0o755)
	return downloadFolder
}

func getConfigDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		return filepath.Join(".", "K-Share")
	}
	return filepath.Join(configDir, "K-Share")
}

func Load() error {
	mu.Lock()
	defer mu.Unlock()
	return loadFromPath(configPath, current)
}

func Save() error {
	mu.Lock()
	defer mu.Unlock()
	return saveToPath(configPath, current)
}

func Get() Config {
	mu.RLock()
	defer mu.RUnlock()
	
	// Create a shallow copy + deep copies for maps
	cfg := *current
	
	cfg.SavedNetworks = make(map[string]string)
	for k, v := range current.SavedNetworks {
		cfg.SavedNetworks[k] = v
	}
	
	cfg.KnownServers = make(map[string]ServerIdentity)
	for k, v := range current.KnownServers {
		cfg.KnownServers[k] = v
	}
	
	return cfg
}

func GetConfigPath() string {
	return configPath
}

func ensureDefaults(cfg *Config) {
	if cfg.SavedNetworks == nil {
		cfg.SavedNetworks = make(map[string]string)
	}
	if cfg.KnownServers == nil {
		cfg.KnownServers = make(map[string]ServerIdentity)
	}
	if cfg.DownloadFolder == "" {
		cfg.DownloadFolder = getDefaultDownloadFolder()
	}
	if cfg.ServerIP == "" {
		cfg.ServerIP = "localhost:26260"
	}
}

func loadFromPath(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			ensureDefaults(cfg)
			return saveToPath(path, cfg)
		}
		return err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return err
	}
	ensureDefaults(cfg)
	return nil
}

func saveToPath(path string, cfg *Config) error {
	ensureDefaults(cfg)
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func SetServerIP(ip string) error {
	mu.Lock()
	defer mu.Unlock()
	current.ServerIP = ip
	return saveToPath(configPath, current)
}

func SetPairingCode(code string) error {
	mu.Lock()
	defer mu.Unlock()
	current.PairingCode = code
	return saveToPath(configPath, current)
}

func SetDownloadFolder(folder string) error {
	mu.Lock()
	defer mu.Unlock()
	current.DownloadFolder = folder
	return nil
}

func AddSavedNetwork(subnet, ip string) error {
	mu.Lock()
	defer mu.Unlock()
	ensureDefaults(current)
	current.SavedNetworks[subnet] = ip
	return saveToPath(configPath, current)
}

func RemoveSavedNetwork(subnet string) error {
	mu.Lock()
	defer mu.Unlock()
	ensureDefaults(current)
	delete(current.SavedNetworks, subnet)
	return saveToPath(configPath, current)
}

func SetKnownServer(certHash string, identity ServerIdentity) error {
	mu.Lock()
	defer mu.Unlock()
	ensureDefaults(current)
	current.KnownServers[certHash] = identity
	return saveToPath(configPath, current)
}

func RemoveKnownServer(certHash string) error {
	mu.Lock()
	defer mu.Unlock()
	ensureDefaults(current)
	delete(current.KnownServers, certHash)
	return saveToPath(configPath, current)
}

func IsServerKnown(certHash string) bool {
	mu.RLock()
	defer mu.RUnlock()
	if current.KnownServers == nil {
		return false
	}
	_, exists := current.KnownServers[certHash]
	return exists
}
