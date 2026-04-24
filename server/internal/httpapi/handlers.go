package httpapi

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	serverclipboard "github.com/Kush-Singh-26/k-share/server/internal/clipboardstore"
	serverconfig "github.com/Kush-Singh-26/k-share/server/internal/config"
	"github.com/Kush-Singh-26/k-share/server/internal/domain"
	serverfiles "github.com/Kush-Singh-26/k-share/server/internal/files"
	serverhistory "github.com/Kush-Singh-26/k-share/server/internal/history"
	serverrealtime "github.com/Kush-Singh-26/k-share/server/internal/realtime"
	serverthumbnail "github.com/Kush-Singh-26/k-share/server/internal/thumbnail"
)

const maxUploadSize = 500 << 20 // 500 MB

var allowedOrigins = map[string]bool{
	"https://localhost": true,
	"https://127.0.0.1": true,
}

func setCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if allowedOrigins[origin] || strings.HasPrefix(origin, "https://localhost:") {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
}

type Handlers struct {
	Config           *serverconfig.Config
	Hub              *serverrealtime.Hub
	Clipboard        *serverclipboard.Store
	History          *serverhistory.Store
	Thumbnail        *serverthumbnail.Store
	GetRole          func(*http.Request) string
	GetEffectiveRoot func(*http.Request) (string, error)
	AppDir           func() string
	OpenURL          func(string) error
}

func writeJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("⚠️ JSON encode error: %v", err)
	}
}

func writeJSONError(w http.ResponseWriter, code, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(domain.AppError{Code: code, Message: message})
}

func (h Handlers) HandlePing(w http.ResponseWriter, r *http.Request) {
	setCORS(w, r)
	w.Header().Set("Content-Type", "application/json")

	role := h.GetRole(r)
	writeJSON(w, map[string]string{
		"status": "ok",
		"name":   "K-Share Server",
		"role":   role,
	})
}

func (h Handlers) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rootDir, err := h.GetEffectiveRoot(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	filename := r.URL.Query().Get("name")
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	log.Printf("📥 Receiving Stream: %s -> %s\n", filename, rootDir)
	if err := serverfiles.Upload(rootDir, filename, r.Body); err != nil {
		log.Printf("❌ Upload failed: %v\n", err)
		http.Error(w, "Upload failed", http.StatusInternalServerError)
		return
	}
	log.Printf("✅ Received: %s\n", filename)
	h.Hub.Notify("files")
}

func (h Handlers) HandleListFiles(w http.ResponseWriter, r *http.Request) {
	rootDir, err := h.GetEffectiveRoot(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	pathParam := r.URL.Query().Get("path")
	files, err := serverfiles.List(rootDir, pathParam)
	if err != nil {
		http.Error(w, "List failed", http.StatusInternalServerError)
		return
	}

	setCORS(w, r)
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, files)
}

func (h Handlers) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.GetRole(r) != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	filename := r.URL.Query().Get("name")
	if filename == "" {
		http.Error(w, "Missing filename", http.StatusBadRequest)
		return
	}

	rootDir, err := h.GetEffectiveRoot(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := serverfiles.Delete(rootDir, filename); err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		log.Printf("❌ Failed to move to trash: %v", err)
		http.Error(w, "Delete failed", http.StatusInternalServerError)
		return
	}

	log.Printf("🗑️ Delete request: name='%s'", filename)
	log.Printf("🗑️ Moved to trash: %s", filename)
	w.WriteHeader(http.StatusOK)
	h.Hub.Notify("files")
}

func (h Handlers) HandleClipboard(w http.ResponseWriter, r *http.Request) {
	role := h.GetRole(r)
	channel := r.URL.Query().Get("channel")

	var targetFile string
	if role == "guest" {
		targetFile = "guest_clipboard.txt"
	} else if role == "admin" {
		if channel == "guest" {
			targetFile = "guest_clipboard.txt"
		} else {
			targetFile = "clipboard.txt"
		}
	} else {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	setCORS(w, r)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	if r.Method == http.MethodOptions {
		return
	}

	if r.Method == http.MethodGet {
		data, _ := h.Clipboard.ReadText(targetFile)
		_, _ = w.Write(data)
		return
	}

	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Request too large", http.StatusRequestEntityTooLarge)
			return
		}
		appendMode := r.URL.Query().Get("mode") == "append"
		_ = h.Clipboard.WriteText(targetFile, body, appendMode)
		if targetFile == "clipboard.txt" {
			_ = h.History.Add(string(body))
		}
		log.Printf("📋 Clipboard updated (%s)", targetFile)
		w.WriteHeader(http.StatusOK)
		if targetFile == "guest_clipboard.txt" {
			h.Hub.Notify("clip_guest")
		} else {
			h.Hub.Notify("clip")
			h.Hub.Notify("history")
		}
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (h Handlers) HandleHistory(w http.ResponseWriter, r *http.Request) {
	setCORS(w, r)
	w.Header().Set("Access-Control-Allow-Methods", "GET, DELETE, OPTIONS")
	if r.Method == http.MethodOptions {
		return
	}

	if h.GetRole(r) != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if r.Method == http.MethodDelete {
		id := r.URL.Query().Get("id")
		if err := h.History.Delete(id); err != nil {
			log.Printf("❌ Failed to delete history item %s: %v", id, err)
			http.Error(w, "Delete failed", http.StatusInternalServerError)
			return
		}
		log.Printf("🗑️ Deleted history item: %s\n", id)
		h.Hub.Notify("history")
		w.WriteHeader(http.StatusOK)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	history, _ := h.History.List()
	writeJSON(w, history)
}

func (h Handlers) HandleThumbnail(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	folder := r.URL.Query().Get("folder")

	dir, err := h.GetEffectiveRoot(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := h.Thumbnail.Serve(dir, name, folder, w, r); err != nil {
		http.Error(w, "Thumbnail failed", http.StatusInternalServerError)
	}
}

func (h Handlers) HandleOpen(w http.ResponseWriter, r *http.Request) {
	setCORS(w, r)
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.GetRole(r) != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Read error", http.StatusInternalServerError)
		return
	}

	rawURL := strings.TrimSpace(string(body))
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" {
		log.Printf("⚠️ Blocked invalid or unsafe URL: %s\n", rawURL)
		http.Error(w, "Only valid HTTPS URLs are allowed", http.StatusBadRequest)
		return
	}

	log.Printf("🌐 Opening URL on PC: %s\n", rawURL)
	if h.OpenURL != nil {
		_ = h.OpenURL(rawURL)
	}
	w.WriteHeader(http.StatusOK)
}

func (h Handlers) HandleWS(w http.ResponseWriter, r *http.Request) {
	if h.GetRole(r) == "none" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	h.Hub.HandleWS(w, r)
}

func (h Handlers) HandleClipboardImage(w http.ResponseWriter, r *http.Request) {
	setCORS(w, r)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

	if r.Method == http.MethodOptions {
		return
	}

	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "image/png")
		data, _ := h.Clipboard.ReadImage()
		_, _ = w.Write(data)
		return
	}

	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10 MB
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Request too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err := h.Clipboard.WriteImage(data); err != nil {
			http.Error(w, "Failed to save image", http.StatusInternalServerError)
			return
		}

		log.Printf("📋 Image Clipboard updated")
		w.WriteHeader(http.StatusOK)
		h.Hub.Notify("clip_image")
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

const downloadPrefix = "/download/"

func (h Handlers) HandleDownload(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, downloadPrefix)
	rootDir, err := h.GetEffectiveRoot(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if err := serverfiles.ServeDownload(rootDir, relPath, w, r); err != nil {
		http.Error(w, "Download failed", http.StatusInternalServerError)
	}
}
