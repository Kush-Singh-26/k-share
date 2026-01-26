package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	ServerIP          string            `json:"server_ip"`
	PairingCode       string            `json:"pairing_code"`
	DownloadFolder    string            `json:"download_folder"`
	AutoSyncClipboard bool              `json:"auto_sync_clipboard"`
	SavedNetworks     map[string]string `json:"saved_networks"`
}

var (
	configPath string
	Current    = &Config{
		ServerIP:          "localhost:9823",
		DownloadFolder:    getDefaultDownloadFolder(),
		AutoSyncClipboard: true,
		SavedNetworks:     make(map[string]string),
	}
)

func init() {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = "."
	}
	configDir := filepath.Join(appData, "k-share-client")
	os.MkdirAll(configDir, 0755)
	configPath = filepath.Join(configDir, "settings.json")
}

func getDefaultDownloadFolder() string {
	userProfile := os.Getenv("USERPROFILE")
	if userProfile == "" {
		return "."
	}
	downloadFolder := filepath.Join(userProfile, "Downloads", "K-Share")
	os.MkdirAll(downloadFolder, 0755)
	return downloadFolder
}

func Load() error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		// If file doesn't exist, use defaults and save
		if os.IsNotExist(err) {
			return Save()
		}
		return err
	}
	return json.Unmarshal(data, Current)
}

func Save() error {
	data, err := json.Marshal(Current)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

func GetConfigPath() string {
	return configPath
}
