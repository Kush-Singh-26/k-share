package dashboard

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Kush-Singh-26/k-share/server/internal/clipboardstore"
	"github.com/Kush-Singh-26/k-share/server/internal/config"
	"github.com/Kush-Singh-26/k-share/server/internal/history"
	"github.com/Kush-Singh-26/k-share/server/internal/realtime"
)

//go:embed static/*
var staticFiles embed.FS

type Handlers struct {
	Config    *config.Config
	ConfigMu  *sync.RWMutex
	Clipboard *clipboardstore.Store
	History   *history.Store
	Hub       *realtime.Hub
	AppDir    string
	LogFile   *os.File
}

func (h *Handlers) ServeDashboard() http.Handler {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/dashboard/", http.FileServer(http.FS(sub)))
}

func (h *Handlers) HandleStatus(w http.ResponseWriter, r *http.Request, ip string, startTime time.Time) {
	h.ConfigMu.RLock()
	port := h.Config.Port
	sharedDir := h.Config.SharedDir
	h.ConfigMu.RUnlock()

	status := map[string]interface{}{
		"uptime":      time.Since(startTime).String(),
		"ip":          ip,
		"port":        port,
		"sharedDir":   sharedDir,
		"clientCount": h.Hub.ClientCount(),
	}
	h.writeJSON(w, status)
}

func validateConfig(cfg config.Config) error {
	if cfg.Port == "" {
		return fmt.Errorf("port cannot be empty")
	}
	if p, err := strconv.Atoi(cfg.Port); err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("invalid port: %s", cfg.Port)
	}
	if cfg.SharedDir == "" {
		return fmt.Errorf("shared_dir cannot be empty")
	}
	if cfg.AdminCode == "" || len(cfg.AdminCode) < 6 {
		return fmt.Errorf("admin_code must be at least 6 characters")
	}
	if cfg.GuestCode == "" || len(cfg.GuestCode) < 6 {
		return fmt.Errorf("guest_code must be at least 6 characters")
	}
	return nil
}

func (h *Handlers) HandleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.ConfigMu.RLock()
		defer h.ConfigMu.RUnlock()
		h.writeJSON(w, h.Config)
		return
	}
	if r.Method == http.MethodPost {
		var newCfg config.Config
		if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := validateConfig(newCfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		h.ConfigMu.Lock()
		*h.Config = newCfg
		h.ConfigMu.Unlock()

		if err := config.Save(newCfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		h.writeJSON(w, map[string]string{"status": "ok"})
		h.Hub.Notify("files") // Trigger index rebuild if shared_dir changed
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (h *Handlers) HandleRotateCodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	role := r.URL.Query().Get("role")
	h.ConfigMu.Lock()
	if role == "admin" {
		h.Config.AdminCode = config.RandomCode(config.DefaultCodeLength)
	} else if role == "guest" {
		h.Config.GuestCode = config.RandomCode(config.DefaultCodeLength)
	} else {
		h.Config.AdminCode = config.RandomCode(config.DefaultCodeLength)
		h.Config.GuestCode = config.RandomCode(config.DefaultCodeLength)
	}
	currentCfg := *h.Config
	h.ConfigMu.Unlock()

	if err := config.Save(currentCfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.writeJSON(w, &currentCfg)
}

func (h *Handlers) HandleLogs(w http.ResponseWriter, r *http.Request) {
	logPath := filepath.Join(h.AppDir, "server_log.txt")
	data, err := os.ReadFile(logPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	lines := strings.Split(string(data), "\n")
	start := len(lines) - 200
	if start < 0 {
		start = 0
	}
	h.writeJSON(w, lines[start:])
}

func (h *Handlers) HandleFiles(w http.ResponseWriter, r *http.Request) {
	var files []map[string]interface{}
	h.ConfigMu.RLock()
	sharedDir := h.Config.SharedDir
	h.ConfigMu.RUnlock()

	err := filepath.Walk(sharedDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if path == sharedDir {
			return nil
		}
		rel, _ := filepath.Rel(sharedDir, path)
		files = append(files, map[string]interface{}{
			"name":  rel,
			"dir":   info.IsDir(),
			"size":  info.Size(),
			"mtime": info.ModTime(),
		})
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.writeJSON(w, files)
}

func (h *Handlers) HandleClearTrash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.ConfigMu.RLock()
	sharedDir := h.Config.SharedDir
	h.ConfigMu.RUnlock()

	trashPath := filepath.Join(sharedDir, ".trash")
	if err := os.RemoveAll(trashPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	os.MkdirAll(trashPath, 0o755)
	h.Hub.Notify("files")
	h.writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handlers) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
