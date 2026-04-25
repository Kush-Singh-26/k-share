package config

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

const (
	DefaultPort       = "26260"
	DefaultCodeLength = 8
	CodeCharset       = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

type Config struct {
	Port      string `json:"port"`
	SharedDir string `json:"shared_dir"`
	AdminCode string `json:"admin_code"`
	GuestCode string `json:"guest_code"`
}

func AppDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		exe, exeErr := os.Executable()
		if exeErr != nil {
			return "."
		}
		appDir := filepath.Join(filepath.Dir(exe), "K-Share")
		_ = os.MkdirAll(appDir, 0o755)
		return appDir
	}

	appDir := filepath.Join(configDir, "K-Share")
	_ = os.MkdirAll(appDir, 0o755)
	return appDir
}

func Path() string {
	// Check local directory first
	if _, err := os.Stat("config.json"); err == nil {
		return "config.json"
	}
	return filepath.Join(AppDir(), "config.json")
}

func Default() Config {
	home, _ := os.UserHomeDir()
	return Config{
		Port:      DefaultPort,
		AdminCode: RandomCode(DefaultCodeLength),
		GuestCode: RandomCode(DefaultCodeLength),
		SharedDir: filepath.Join(home, "Documents", "K-Share-Files"),
	}
}

func Load() (Config, error) {
	path := Path()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := Default()
			if err := Save(cfg); err != nil {
				return Config{}, err
			}
			return cfg, nil
		}
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		// Backup corrupted config and create a default one
		backupPath := path + ".corrupted." + time.Now().Format("20060102_150405")
		_ = os.WriteFile(backupPath, data, 0o644)
		cfg = Default()
		if saveErr := Save(cfg); saveErr != nil {
			return Config{}, fmt.Errorf("config corrupted and failed to recreate default: %w", saveErr)
		}
		return cfg, nil
	}
	return cfg, nil
}

func Save(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	path := Path()
	tmpPath := path + ".tmp"
	// Write to temporary file first
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	// Atomic rename to target path
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func RandomCode(length int) string {
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(CodeCharset))))
		if err != nil {
			// Fallback to a deterministic string to avoid panic
			return "aB3dE7fG"
		}
		b[i] = CodeCharset[n.Int64()]
	}
	return string(b)
}

func Reset() (Config, error) {
	_ = os.Remove(Path())
	cfg := Default()
	if err := Save(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
