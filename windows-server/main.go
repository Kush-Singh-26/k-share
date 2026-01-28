package main

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/systray"
	"github.com/gorilla/websocket"
	"github.com/nfnt/resize"
	"golang.org/x/sys/windows/registry"
)

type Config struct {
	Port      string `json:"port"`
	SharedDir string `json:"shared_dir"`
	AdminCode string `json:"admin_code"`
	GuestCode string `json:"guest_code"`
}

type FileInfo struct {
	Name        string `json:"name"`
	IsDirectory bool   `json:"isDirectory"`
	Size        int64  `json:"size"`
	ModTime     string `json:"modTime"`
}

type HistoryItem struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

const (
	chunkSize = 64 * 1024
)

var (
	appConfig      Config
	clipboardMutex sync.Mutex
	hub            *Hub
	lastKnownIP    string
	cacheDir       string
)

func init() {
	// Setup cache directory
	exe, err := os.Executable()
	if err != nil {
		cacheDir = "cache"
	} else {
		cacheDir = filepath.Join(filepath.Dir(exe), ".thumbnails")
	}
	os.MkdirAll(cacheDir, 0755)
}

// Hub manages WebSocket connections for real-time updates
type Hub struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan []byte
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mutex      sync.Mutex
}

func newHub() *Hub {
	return &Hub{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mutex.Lock()
			h.clients[client] = true
			h.mutex.Unlock()
		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.Close()
			}
			h.mutex.Unlock()
		case message := <-h.broadcast:
			h.mutex.Lock()
			for client := range h.clients {
				err := client.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					client.Close()
					delete(h.clients, client)
				}
			}
			h.mutex.Unlock()
		}
	}
}

func notifyChange(changeType string) {
	msg, _ := json.Marshal(map[string]string{"type": changeType})
	hub.broadcast <- msg
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	hub.register <- conn
}

// Utilities
func getAppDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func setupLogging() {
	logFile := filepath.Join(getAppDir(), "server_log.txt")
	if info, err := os.Stat(logFile); err == nil && info.Size() > 10*1024*1024 {
		os.Remove(logFile)
	}
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("❌ Failed to open log file: %v\n", err)
		return
	}
	multi := io.MultiWriter(f, os.Stdout)
	log.SetOutput(multi)
	log.Println("🚀 --- K-Share Secure Server Started (HTTPS) ---")
}

func loadConfig() {
	file, err := os.Open(filepath.Join(getAppDir(), "config.json"))
	if err != nil {
		log.Fatal("❌ Could not load config.json: ", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&appConfig)
	if err != nil {
		log.Fatal("❌ Invalid config.json format: ", err)
	}
}

// Auth & Role-based access control
func getRole(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "none"
	}
	code := strings.TrimPrefix(auth, "Bearer ")
	if code == appConfig.AdminCode {
		return "admin"
	}
	if code == appConfig.GuestCode {
		return "guest"
	}
	return "none"
}

func getEffectiveRoot(r *http.Request) (string, error) {
	role := getRole(r)
	if role == "admin" {
		return appConfig.SharedDir, nil
	}
	if role == "guest" {
		p := filepath.Join(appConfig.SharedDir, "Public")
		os.MkdirAll(p, 0755)
		return p, nil
	}
	return "", fmt.Errorf("unauthorized")
}

func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if getRole(r) == "none" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	role := getRole(r) // "admin", "guest", or "none" (if no auth header)

	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"name":   "K-Share Server",
		"proto":  "https",
		"role":   role,
	})
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Auth Check
	rootDir, err := getEffectiveRoot(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	filename := r.URL.Query().Get("name")
	if filename == "" {
		filename = "upload_" + time.Now().Format("20060102_150405")
	}

	destPath := filepath.Join(rootDir, filename)
	// Ensure unique filename to prevent overwrite
	destPath = getUniqueFilenameServer(destPath)

	// Create parent directories if they don't exist
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		http.Error(w, "Failed to create directory", http.StatusInternalServerError)
		return
	}

	destFile, err := os.Create(destPath)
	if err != nil {
		http.Error(w, "Save failed", http.StatusInternalServerError)
		return
	}
	defer destFile.Close()
	log.Printf("📥 Receiving Stream: %s -> %s\n", filename, rootDir)

	// Stream request body directly to file
	_, err = io.Copy(destFile, r.Body)
	if err != nil {
		log.Printf("❌ Upload failed: %v\n", err)
		http.Error(w, "Upload failed", http.StatusInternalServerError)
		return
	}
	log.Printf("✅ Received: %s\n", filename)
	notifyChange("files")
}

func handleListFiles(w http.ResponseWriter, r *http.Request) {
	// Auth Check
	rootDir, err := getEffectiveRoot(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	role := getRole(r)
	var files []FileInfo

	// Helper to read a directory
	readDir := func(path string, prefix string) {
		entries, err := os.ReadDir(path)
		if err != nil {
			return
		}
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			// Skip valid items
			name := info.Name()
			if name == "Public" && prefix == "" {
				continue
			} // Skip Public folder itself in root listing

			files = append(files, FileInfo{
				Name:        prefix + name,
				IsDirectory: info.IsDir(),
				Size:        info.Size(),
				ModTime:     info.ModTime().Format("2006-01-02 15:04:05"),
			})
		}
	}

	// 1. Read Root
	readDir(rootDir, "")

	// 2. If Admin, also read Public and prefix with "Public/"
	if role == "admin" {
		publicDir := filepath.Join(rootDir, "Public")
		readDir(publicDir, "Public/")
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func handleClipboard(w http.ResponseWriter, r *http.Request) {
	role := getRole(r)
	channel := r.URL.Query().Get("channel") // "guest" or empty (default)

	// Determine target file
	var targetFile string

	if role == "guest" {
		// Guest is forced to use guest clipboard
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

	clipboardPath := filepath.Join(getAppDir(), targetFile)

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

	if r.Method == http.MethodOptions {
		return
	}
	clipboardMutex.Lock()
	defer clipboardMutex.Unlock()

	if r.Method == http.MethodGet {
		data, _ := os.ReadFile(clipboardPath)
		w.Write(data)
	} else if r.Method == http.MethodPost {
		body, _ := io.ReadAll(r.Body)
		data := body // Plain text over TLS

		mode := r.URL.Query().Get("mode")
		var finalData []byte
		if mode == "append" {
			currentData, _ := os.ReadFile(clipboardPath)
			if len(currentData) > 0 {
				finalData = append(currentData, []byte("\n")...)
				finalData = append(finalData, data...)
			} else {
				finalData = data
			}
		} else {
			finalData = data
		}
		os.WriteFile(clipboardPath, finalData, 0644)
		if targetFile == "clipboard.txt" {
			addToHistory(string(data))
		}
		log.Printf("📋 Clipboard updated (%s)", targetFile)
		w.WriteHeader(http.StatusOK)

		// Notify based on channel
		if targetFile == "guest_clipboard.txt" {
			notifyChange("clip_guest")
		} else {
			notifyChange("clip")
			notifyChange("history")
		}
	}
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, DELETE, OPTIONS")
	if r.Method == http.MethodOptions {
		return
	}

	// SECURITY: Clipboard history is admin-only
	if getRole(r) != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	historyPath := filepath.Join(getAppDir(), "clipboard_history.json")

	if r.Method == http.MethodDelete {
		id := r.URL.Query().Get("id")
		var items []HistoryItem
		data, err := os.ReadFile(historyPath)
		if err == nil {
			json.Unmarshal(data, &items)
			newItems := []HistoryItem{}
			for _, item := range items {
				if item.ID != id {
					newItems = append(newItems, item)
				}
			}
			data, _ = json.Marshal(newItems)
			os.WriteFile(historyPath, data, 0644)
			log.Printf("🗑️ Deleted history item: %s\n", id)
			notifyChange("history")
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	history, _ := os.ReadFile(historyPath)
	w.Header().Set("Content-Type", "application/json")
	w.Write(history)
}

func handleThumbnail(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	folder := r.URL.Query().Get("folder")

	// Auth Check
	dir, err := getEffectiveRoot(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Sanitize path to prevent directory traversal
	relPath := filepath.Join(folder, name)
	if strings.Contains(relPath, "..") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	filePath := filepath.Join(dir, relPath)

	// 1. Check if source file exists
	info, err := os.Stat(filePath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// 2. Generate Cache Filename (Hash of full path + modtime)
	h := sha256.New()
	h.Write([]byte(filePath))
	h.Write([]byte(info.ModTime().String()))
	hash := hex.EncodeToString(h.Sum(nil))
	cachePath := filepath.Join(cacheDir, hash+".jpg")

	// 3. Serve from Cache if exists
	if _, err := os.Stat(cachePath); err == nil {
		w.Header().Set("Content-Type", "image/jpeg")
		http.ServeFile(w, r, cachePath)
		return
	}

	// 4. Generate Thumbnail
	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "Read error", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(name))
	var img image.Image
	if ext == ".jpg" || ext == ".jpeg" {
		img, _ = jpeg.Decode(file)
	} else if ext == ".png" {
		img, _ = png.Decode(file)
	}

	if img == nil {
		http.Error(w, "Unsupported format", http.StatusBadRequest)
		return
	}

	// Resize
	m := resize.Thumbnail(128, 128, img, resize.Lanczos3)

	// 5. Save to Cache
	outFile, err := os.Create(cachePath)
	if err == nil {
		jpeg.Encode(outFile, m, nil)
		outFile.Close()
	}

	// 6. Serve Generated
	w.Header().Set("Content-Type", "image/jpeg")
	jpeg.Encode(w, m, nil)
}

func handleOpen(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method != http.MethodPost {
		return
	}
	body, _ := io.ReadAll(r.Body)
	url := string(body)
	log.Printf("🌐 Opening URL on PC: %s\n", url)
	exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	w.WriteHeader(http.StatusOK)
}

func addToHistory(text string) {
	historyPath := filepath.Join(getAppDir(), "clipboard_history.json")
	var items []HistoryItem
	data, err := os.ReadFile(historyPath)
	if err == nil {
		json.Unmarshal(data, &items)
	}
	newItem := HistoryItem{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Text:      text,
		Timestamp: time.Now(),
	}
	if len(items) > 0 && items[0].Text == text {
		return
	}
	items = append([]HistoryItem{newItem}, items...)
	if len(items) > 20 {
		items = items[:20]
	}
	data, _ = json.Marshal(items)
	os.WriteFile(historyPath, data, 0644)
}

func getOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		return conn.LocalAddr().(*net.UDPAddr).IP
	}

	// Fallback: Scan interfaces for a valid IPv4 address
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return net.IPv4(127, 0, 0, 1)
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP
			}
		}
	}
	return net.IPv4(127, 0, 0, 1)
}

func startIPMonitor() {
	for {
		time.Sleep(30 * time.Second)
		currentIP := getOutboundIP().String()
		if currentIP != lastKnownIP && currentIP != "127.0.0.1" {
			log.Printf("🔄 IP Changed: %s → %s\n", lastKnownIP, currentIP)
			lastKnownIP = currentIP
		}
	}
}

func generateSelfSignedCert() {
	if _, err := os.Stat("cert.pem"); err == nil {
		if _, err := os.Stat("key.pem"); err == nil {
			return // Already exists
		}
	}
	log.Println("🔐 Generating self-signed TLS certificate...")

	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"K-Share Self-Signed"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Add all IPs to SANs
	addrs, _ := net.InterfaceAddrs()
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			template.IPAddresses = append(template.IPAddresses, ipnet.IP)
		}
	}
	template.IPAddresses = append(template.IPAddresses, net.ParseIP("127.0.0.1"))

	derBytes, _ := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)

	certOut, _ := os.Create("cert.pem")
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()

	keyOut, _ := os.Create("key.pem")
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	keyOut.Close()
	log.Println("✅ Certificate generated.")
}

//go:embed Icon.png
var iconData []byte

func loadIcon() []byte {
	if len(iconData) == 0 {
		return generateIcon()
	}

	// Windows systray requires ICO format. We need to wrap the PNG bytes in an ICO header.
	// 1. Decode PNG to get dimensions
	cfg, _, err := image.DecodeConfig(bytes.NewReader(iconData))
	if err != nil {
		return generateIcon()
	}

	// 2. Construct ICO Header
	ico := new(bytes.Buffer)
	binary.Write(ico, binary.LittleEndian, uint16(0)) // Reserved
	binary.Write(ico, binary.LittleEndian, uint16(1)) // Type (1=ICO)
	binary.Write(ico, binary.LittleEndian, uint16(1)) // Count (1 image)

	// 3. Directory Entry
	w := cfg.Width
	h := cfg.Height
	if w >= 256 {
		w = 0
	}
	if h >= 256 {
		h = 0
	}
	binary.Write(ico, binary.LittleEndian, uint8(w))
	binary.Write(ico, binary.LittleEndian, uint8(h))
	binary.Write(ico, binary.LittleEndian, uint8(0))              // Palette count
	binary.Write(ico, binary.LittleEndian, uint8(0))              // Reserved
	binary.Write(ico, binary.LittleEndian, uint16(1))             // Color planes
	binary.Write(ico, binary.LittleEndian, uint16(32))            // Bits per pixel
	binary.Write(ico, binary.LittleEndian, uint32(len(iconData))) // Size of image data
	binary.Write(ico, binary.LittleEndian, uint32(22))            // Offset of image data (6+16=22)

	// 4. Image Data
	ico.Write(iconData)

	return ico.Bytes()
}

func generateIcon() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	bgColor := color.RGBA{63, 81, 181, 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)
	white := color.RGBA{255, 255, 255, 255}
	for y := 6; y < 26; y++ {
		for x := 8; x < 11; x++ {
			img.Set(x, y, white)
		}
	}
	for i := 0; i < 12; i++ {
		img.Set(11+i, 18-i, white)
		img.Set(11+i, 19-i, white)
	}
	for i := 0; i < 12; i++ {
		img.Set(11+i, 14+i, white)
		img.Set(11+i, 15+i, white)
	}
	buf := new(bytes.Buffer)
	png.Encode(buf, img)
	pngBytes := buf.Bytes()
	ico := new(bytes.Buffer)
	binary.Write(ico, binary.LittleEndian, uint16(0))
	binary.Write(ico, binary.LittleEndian, uint16(1))
	binary.Write(ico, binary.LittleEndian, uint16(1))
	binary.Write(ico, binary.LittleEndian, uint8(32))
	binary.Write(ico, binary.LittleEndian, uint8(32))
	binary.Write(ico, binary.LittleEndian, uint8(0))
	binary.Write(ico, binary.LittleEndian, uint8(0))
	binary.Write(ico, binary.LittleEndian, uint16(1))
	binary.Write(ico, binary.LittleEndian, uint16(32))
	binary.Write(ico, binary.LittleEndian, uint32(len(pngBytes)))
	binary.Write(ico, binary.LittleEndian, uint32(22))
	ico.Write(pngBytes)
	return ico.Bytes()
}

func getUniqueFilenameServer(basePath string) string {
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return basePath
	}

	dir := filepath.Dir(basePath)
	ext := filepath.Ext(basePath)
	nameWithoutExt := filepath.Base(basePath)
	if len(ext) > 0 {
		nameWithoutExt = nameWithoutExt[:len(nameWithoutExt)-len(ext)]
	}

	counter := 1
	for {
		newPath := filepath.Join(dir, nameWithoutExt+" ("+itoa(counter)+")"+ext)
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
		counter++
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}

func onReady() {
	systray.SetTitle("K-Share")
	systray.SetTooltip("K-Share-Server: Encrypted Local Sharing")
	iconBytes := loadIcon()
	systray.SetIcon(iconBytes)

	// Auto-install context menu (HKCU, no admin needed)
	go installContextMenu()

	mIP := systray.AddMenuItem("IP: "+getOutboundIP().String(), "")
	mIP.Disable()

	systray.AddSeparator()

	mRefreshIP := systray.AddMenuItem("Refresh IP", "Check for IP changes")
	mOpenShared := systray.AddMenuItem("Open Shared Folder", "Open storage location")

	systray.AddSeparator()
	mExit := systray.AddMenuItem("Exit", "Quit K-Share")

	go func() {
		for {
			select {
			case <-mOpenShared.ClickedCh:
				exec.Command("explorer", appConfig.SharedDir).Start()

			case <-mRefreshIP.ClickedCh:
				newIP := getOutboundIP()
				mIP.SetTitle("IP: " + newIP.String())
				log.Printf("🔄 Manual IP Refresh: %s\n", newIP.String())

			case <-mExit.ClickedCh:
				systray.Quit()
				os.Exit(0)
			}
		}
	}()
}

func onExit() {}

func main() {
	// 1. Handle Context Menu CLI Args
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-install":
			installContextMenu()
			os.Exit(0)
		case "-uninstall":
			uninstallContextMenu()
			os.Exit(0)
		case "-send":
			if len(os.Args) > 2 {
				sendToPhone(os.Args[2])
			}
			os.Exit(0)
		}
	}

	setupLogging()
	loadConfig()
	hub = newHub()
	go hub.run()

	// Create Shared Dir
	if err := os.MkdirAll(appConfig.SharedDir, 0755); err != nil {
		log.Printf("❌ Failed to create SharedDir: %v", err)
	}

	localIP := getOutboundIP()
	lastKnownIP = localIP.String()
	log.Printf("📡 Local IP: %s\n", localIP)
	go startIPMonitor()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handleWS) // WS handles its own auth inside
	mux.HandleFunc("/ping", handlePing)
	mux.HandleFunc("/upload", requireAuth(handleUpload))
	mux.HandleFunc("/open", requireAuth(handleOpen))
	mux.HandleFunc("/thumbnail", requireAuth(handleThumbnail))
	mux.HandleFunc("/clipboard", requireAuth(handleClipboard))
	mux.HandleFunc("/clipboard/history", requireAuth(handleHistory))
	mux.HandleFunc("/files", requireAuth(handleListFiles))

	mux.HandleFunc("/download/", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// /download/filename.ext or /download/subfolder/filename.ext
		relPath := strings.TrimPrefix(path, "/download/")

		rootDir, err := getEffectiveRoot(r)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Secure path validation: join first, then validate canonical path
		fullPath := filepath.Join(rootDir, filepath.Clean(relPath))
		absPath, err := filepath.Abs(fullPath)
		if err != nil {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}
		absRoot, _ := filepath.Abs(rootDir)
		// Ensure the resolved path is within the allowed root directory
		if !strings.HasPrefix(absPath, absRoot+string(filepath.Separator)) && absPath != absRoot {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}

		info, err := os.Stat(fullPath)
		if err != nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}

		if info.IsDir() {
			// Zip folder
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.zip\"", filepath.Base(fullPath)))
			zw := zip.NewWriter(w)
			defer zw.Close()
			filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				rel, _ := filepath.Rel(fullPath, path)
				if rel == "." {
					return nil
				}
				// Ensure ZIP uses forward slashes (standard) even on Windows
				rel = filepath.ToSlash(rel)
				if info.IsDir() {
					_, err = zw.Create(rel + "/")
					return err
				}
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				defer f.Close()
				dest, err := zw.Create(rel)
				if err != nil {
					return err
				}
				_, err = io.Copy(dest, f)
				return err
			})
		} else {
			// Serve File
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(fullPath)))
			http.ServeFile(w, r, fullPath)
		}
	}))

	// Generate certs if needed
	generateSelfSignedCert()

	go func() {
		// Use HTTPS
		server := &http.Server{
			Addr:    ":" + appConfig.Port,
			Handler: mux,
		}
		log.Printf("🔒 Server listening on https://%s:%s", "0.0.0.0", appConfig.Port)
		// Enable TLS 1.3 explicitly if needed, but default is good.
		log.Fatal(server.ListenAndServeTLS("cert.pem", "key.pem"))
	}()

	systray.Run(onReady, onExit)
}

// --- 🔧 CONTEXT MENU HELPERS ---

func installContextMenu() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("❌ Failed to get executable path: %v", err)
	}

	// 1. Create main key in HKCU (Software\Classes)
	k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\*\shell\KShareSend`, registry.ALL_ACCESS)
	if err != nil {
		log.Printf("❌ Failed to create registry key in HKCU: %v\n", err)
		return
	}
	defer k.Close()

	if err := k.SetStringValue("", "Send to Phone (K-Share)"); err != nil {
		log.Fatal(err)
	}
	if err := k.SetStringValue("Icon", exe); err != nil {
		log.Fatal(err)
	}

	// 2. Create command key
	ck, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\*\shell\KShareSend\command`, registry.ALL_ACCESS)
	if err != nil {
		log.Fatal(err)
	}
	defer ck.Close()

	cmd := fmt.Sprintf(`"%s" -send "%%1"`, exe)
	if err := ck.SetStringValue("", cmd); err != nil {
		log.Fatal(err)
	}

	fmt.Println("✅ Context menu installed successfully!")
	time.Sleep(2 * time.Second)
}

func uninstallContextMenu() {
	err := registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\*\shell\KShareSend\command`)
	if err != nil {
		log.Printf("⚠️ Failed to delete command key: %v", err)
	}
	err = registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\*\shell\KShareSend`)
	if err != nil {
		log.Printf("⚠️ Failed to delete main key: %v", err)
	} else {
		fmt.Println("✅ Context menu uninstalled successfully!")
	}
	time.Sleep(2 * time.Second)
}

func sendToPhone(filePath string) {
	fmt.Printf("🚀 Sending to phone: %s\n", filePath)

	// Load config to get port
	loadConfig()

	// Detect if it's a file or folder
	_, err := os.Stat(filePath)
	if err != nil {
		log.Fatalf("❌ File check failed: %v", err)
	}

	url := fmt.Sprintf("https://localhost:%s/upload?name=%s", appConfig.Port, filepath.Base(filePath))

	// Skip verification for local CLI
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	file, err := os.Open(filePath)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	req, err := http.NewRequest("POST", url, file)
	if err != nil {
		log.Fatalf("❌ Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+appConfig.AdminCode)

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("❌ Upload failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Println("✅ Sent successfully!")
	} else {
		fmt.Printf("❌ Server returned: %s\n", resp.Status)
	}
	time.Sleep(2 * time.Second)
}
