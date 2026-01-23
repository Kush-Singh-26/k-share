package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"log"
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
)

type Config struct {
	Port        string `json:"port"`
	GHToken     string `json:"github_token"`
	GistID      string `json:"gist_id"`
	GistFile    string `json:"gist_filename"`
	FromPhone   string `json:"from_phone_dir"`
	ToPhone     string `json:"to_phone_dir"`
	PairingCode string `json:"pairing_code"`
}

type FileInfo struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
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
	lastKnownIP    string
	hub            *Hub
)

// --- 🔌 WEBSOCKET HUB ---
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

// --- 🔐 ENCRYPTION HELPERS ---
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
	log.Println("🚀 --- K-Share Encrypted Server Started ---")
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

func getEncryptionKey() []byte {
	hash := sha256.Sum256([]byte(appConfig.PairingCode))
	return hash[:]
}

func encryptData(data []byte) (string, error) {
	key := getEncryptionKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	io.ReadFull(rand.Reader, nonce)

	timestamp := time.Now().Unix()
	payload := map[string]interface{}{
		"t": timestamp,
		"d": base64.StdEncoding.EncodeToString(data),
	}
	jsonPayload, _ := json.Marshal(payload)
	ciphertext := gcm.Seal(nonce, nonce, jsonPayload, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func decryptData(encryptedStr string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedStr)
	if err != nil {
		return nil, err
	}
	key := getEncryptionKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, _ := cipher.NewGCM(block)
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	var payload map[string]interface{}
	json.Unmarshal(plaintext, &payload)
	if t, ok := payload["t"].(float64); ok {
		if time.Now().Unix()-int64(t) > 600 {
			return nil, fmt.Errorf("message expired")
		}
	}
	dataStr, _ := payload["d"].(string)
	return base64.StdEncoding.DecodeString(dataStr)
}

func encryptStream(dst io.Writer, src io.Reader) error {
	key := getEncryptionKey()
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	io.ReadFull(rand.Reader, nonce)
	dst.Write(nonce)

	buf := make([]byte, chunkSize)
	chunkIndex := uint64(0)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			currentNonce := make([]byte, len(nonce))
			copy(currentNonce, nonce)
			for i := 0; i < 8; i++ {
				currentNonce[i] ^= byte(chunkIndex >> (i * 8))
			}
			encrypted := gcm.Seal(nil, currentNonce, buf[:n], nil)
			sizeBuf := make([]byte, 4)
			sizeBuf[0], sizeBuf[1], sizeBuf[2], sizeBuf[3] = byte(len(encrypted)), byte(len(encrypted)>>8), byte(len(encrypted)>>16), byte(len(encrypted)>>24)
			dst.Write(sizeBuf)
			dst.Write(encrypted)
			chunkIndex++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func decryptStream(dst io.Writer, src io.Reader) error {
	key := getEncryptionKey()
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(src, nonce); err != nil {
		return err
	}
	chunkIndex := uint64(0)
	sizeBuf := make([]byte, 4)
	for {
		_, err := io.ReadFull(src, sizeBuf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		length := uint32(sizeBuf[0]) | uint32(sizeBuf[1])<<8 | uint32(sizeBuf[2])<<16 | uint32(sizeBuf[3])<<24
		encrypted := make([]byte, length)
		if _, err := io.ReadFull(src, encrypted); err != nil {
			return err
		}
		currentNonce := make([]byte, len(nonce))
		copy(currentNonce, nonce)
		for i := 0; i < 8; i++ {
			currentNonce[i] ^= byte(chunkIndex >> (i * 8))
		}
		plaintext, err := gcm.Open(nil, currentNonce, encrypted, nil)
		if err != nil {
			return err
		}
		dst.Write(plaintext)
		chunkIndex++
	}
	return nil
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	data, _ := json.Marshal(map[string]string{"status": "ok", "mode": "encrypted"})
	enc, _ := encryptData(data)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write([]byte(enc))
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	filename := r.URL.Query().Get("name")
	if filename == "" {
		filename = "upload_" + time.Now().Format("20060102_150405")
	}
	dir := appConfig.FromPhone
	if r.URL.Query().Get("folder") == "tophone" {
		dir = appConfig.ToPhone
	}
	destPath := filepath.Join(dir, filename)
	destFile, err := os.Create(destPath)
	if err != nil {
		http.Error(w, "Save failed", http.StatusInternalServerError)
		return
	}
	defer destFile.Close()
	log.Printf("📥 Receiving Encrypted: %s -> %s\n", filename, dir)
	err = decryptStream(destFile, r.Body)
	if err != nil {
		log.Printf("❌ Decrypt failed: %v\n", err)
		http.Error(w, "Decrypt failed", http.StatusBadRequest)
		return
	}
	log.Printf("✅ Received & Decrypted: %s\n", filename)
	notifyChange("files")
}

func handleListFiles(w http.ResponseWriter, r *http.Request, dir string) {
	entries, _ := os.ReadDir(dir)
	var files []FileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, _ := entry.Info()
		files = append(files, FileInfo{Name: info.Name(), Size: info.Size(), ModTime: info.ModTime()})
	}
	data, _ := json.Marshal(files)
	enc, _ := encryptData(data)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write([]byte(enc))
}

func handleClipboard(w http.ResponseWriter, r *http.Request) {
	clipboardPath := filepath.Join(getAppDir(), "clipboard.txt")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		return
	}
	clipboardMutex.Lock()
	defer clipboardMutex.Unlock()
	if r.Method == http.MethodGet {
		data, _ := os.ReadFile(clipboardPath)
		enc, _ := encryptData(data)
		w.Write([]byte(enc))
	} else if r.Method == http.MethodPost {
		body, _ := io.ReadAll(r.Body)
		data, err := decryptData(string(body))
		if err != nil {
			http.Error(w, "Decrypt failed", http.StatusForbidden)
			return
		}
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
		addToHistory(string(data))
		log.Println("📋 Clipboard updated")
		w.WriteHeader(http.StatusOK)
		notifyChange("clip")
		notifyChange("history")
	}
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, DELETE, OPTIONS")
	if r.Method == http.MethodOptions {
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
	var items []HistoryItem
	json.Unmarshal(history, &items)

	// Self-healing: Assign IDs to old items if they are 0
	changed := false
	for i := range items {
		if items[i].ID == "" || items[i].ID == "0" {
			items[i].ID = fmt.Sprintf("%d", items[i].Timestamp.UnixNano())
			changed = true
		}
	}
	if changed {
		data, _ := json.Marshal(items)
		os.WriteFile(historyPath, data, 0644)
	}

	data, _ := json.Marshal(items)
	enc, _ := encryptData(data)
	w.Write([]byte(enc))
}

func handleThumbnail(w http.ResponseWriter, r *http.Request) {
	folder := r.URL.Query().Get("folder")
	name := r.URL.Query().Get("name")
	dir := appConfig.ToPhone
	if folder == "fromphone" {
		dir = appConfig.FromPhone
	}
	filePath := filepath.Join(dir, name)
	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
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
	m := resize.Thumbnail(128, 128, img, resize.Lanczos3)
	w.Header().Set("Content-Type", "image/jpeg")
	jpeg.Encode(w, m, nil)
}

func handleOpen(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method != http.MethodPost {
		return
	}
	body, _ := io.ReadAll(r.Body)
	url, err := decryptData(string(body))
	if err != nil {
		http.Error(w, "Decrypt failed", http.StatusForbidden)
		return
	}
	log.Printf("🌐 Opening URL on PC: %s\n", string(url))
	exec.Command("rundll32", "url.dll,FileProtocolHandler", string(url)).Start()
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

func updateSecretGist(ip string) {
	url := fmt.Sprintf("https://api.github.com/gists/%s", appConfig.GistID)
	contentJSON := fmt.Sprintf(`{"ip": "%s"}`, ip)
	payload := map[string]interface{}{"files": map[string]interface{}{appConfig.GistFile: map[string]string{"content": contentJSON}}}
	jsonBytes, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonBytes))
	req.Header.Set("Authorization", "token "+appConfig.GHToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("⚠️  Gist Update Offline: %v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		lastKnownIP = ip
		log.Println("✅ GitHub Gist Updated!")
	}
}

func startDiscoveryMonitor() {
	for {
		localIP := getOutboundIP().String()
		if localIP != lastKnownIP {
			log.Printf("🔄 IP Change Detected: %s -> %s. Updating Gist...\n", lastKnownIP, localIP)
			updateSecretGist(localIP)
		}
		time.Sleep(2 * time.Minute)
	}
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

func onReady() {
	systray.SetTitle("K-Share")
	systray.SetTooltip("K-Share: Encrypted Local Sharing")
	iconBytes := generateIcon()
	systray.SetIcon(iconBytes)
	mIP := systray.AddMenuItem("IP: "+getOutboundIP().String(), "")
	mIP.Disable()
	systray.AddSeparator()
	mOpen := systray.AddMenuItem("Open Dashboard", "Open browser interface")
	mExit := systray.AddMenuItem("Exit", "Quit K-Share")
	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				exec.Command("rundll32", "url.dll,FileProtocolHandler", fmt.Sprintf("http://localhost:%s", appConfig.Port)).Start()
			case <-mExit.ClickedCh:
				systray.Quit()
				os.Exit(0)
			}
		}
	}()
}

func onExit() {}

func main() {
	setupLogging()
	loadConfig()
	hub = newHub()
	go hub.run()
	os.MkdirAll(appConfig.FromPhone, 0755)
	os.MkdirAll(appConfig.ToPhone, 0755)
	localIP := getOutboundIP()
	log.Printf("📡 Local IP: %s\n", localIP)
	go startDiscoveryMonitor()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handleWS)
	mux.HandleFunc("/ping", handlePing)
	mux.HandleFunc("/upload", handleUpload)
	mux.HandleFunc("/open", handleOpen)
	mux.HandleFunc("/thumbnail", handleThumbnail)
	mux.HandleFunc("/clipboard/history", handleHistory)
	mux.HandleFunc("/files/tophone", func(w http.ResponseWriter, r *http.Request) { handleListFiles(w, r, appConfig.ToPhone) })
	mux.HandleFunc("/files/fromphone", func(w http.ResponseWriter, r *http.Request) { handleListFiles(w, r, appConfig.FromPhone) })
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		filename := filepath.Base(r.URL.Path)
		dir := appConfig.ToPhone
		if r.URL.Query().Get("folder") == "fromphone" {
			dir = appConfig.FromPhone
		}
		file, err := os.Open(filepath.Join(dir, filename))
		if err != nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		defer file.Close()
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.enc\"", filename))
		log.Printf("📤 Sending Encrypted: %s\n", filename)
		encryptStream(w, file)
	})
	mux.HandleFunc("/clipboard", handleClipboard)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(getAppDir(), "index.html"))
	})

	go func() {
		server := &http.Server{Addr: ":" + appConfig.Port, Handler: mux}
		log.Fatal(server.ListenAndServe())
	}()

	systray.Run(onReady, onExit)
}
