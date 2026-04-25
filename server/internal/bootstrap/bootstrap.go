package bootstrap

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/getlantern/systray"
	"github.com/grandcat/zeroconf"

	serverauth "github.com/Kush-Singh-26/k-share/server/internal/auth"
	serverclipboard "github.com/Kush-Singh-26/k-share/server/internal/clipboardstore"
	serverconfig "github.com/Kush-Singh-26/k-share/server/internal/config"
	serverdashboard "github.com/Kush-Singh-26/k-share/server/internal/dashboard"
	serverhistory "github.com/Kush-Singh-26/k-share/server/internal/history"
	serverhttpapi "github.com/Kush-Singh-26/k-share/server/internal/httpapi"
	serverplatform "github.com/Kush-Singh-26/k-share/server/internal/platform"
	serverrealtime "github.com/Kush-Singh-26/k-share/server/internal/realtime"
	serversearch "github.com/Kush-Singh-26/k-share/server/internal/search"
	serverthumbnail "github.com/Kush-Singh-26/k-share/server/internal/thumbnail"
)

	type App struct {
		Config         *serverconfig.Config
		Hub            *serverrealtime.Hub
		ipMu           sync.RWMutex
		lastKnownIP    string
		Clipboard      *serverclipboard.Store
		History        *serverhistory.Store
		Thumbnail      *serverthumbnail.Store
		AppDir         string
		ThumbnailLimit int64
		logFile        *os.File
		stopCh         chan struct{}
		mdnsRestartCh  chan struct{}
		mdnsMu         sync.Mutex
		mIP            *systray.MenuItem
		server         *http.Server
		StartTime      time.Time
		ConfigMu       sync.RWMutex
		logMu          sync.Mutex
		RateLimiter    *rateLimiter
		indexMu        sync.RWMutex
		adminIndex     *serversearch.Index
		guestIndex     *serversearch.Index
	}

	func (a *App) getAdminIndex() *serversearch.Index {
		a.indexMu.RLock()
		defer a.indexMu.RUnlock()
		return a.adminIndex
	}

	func (a *App) getGuestIndex() *serversearch.Index {
		a.indexMu.RLock()
		defer a.indexMu.RUnlock()
		return a.guestIndex
	}

	func (a *App) setIndices(adminIdx, guestIdx *serversearch.Index) {
		a.indexMu.Lock()
		defer a.indexMu.Unlock()
		a.adminIndex = adminIdx
		a.guestIndex = guestIdx
	}

func Run(args []string) {
	if len(args) > 1 {
		switch args[1] {
		case "-install":
			serverplatform.InstallContextMenu()
			os.Exit(0)
		case "-uninstall":
			serverplatform.UninstallContextMenu()
			os.Exit(0)
		case "-send":
			if len(args) > 2 {
				SendToPhone(args[2])
			}
			os.Exit(0)
		}
	}

	app := New()
	app.Run()
}

func New() *App {
	appDir := serverconfig.AppDir()
	cfg, err := serverconfig.Load()
	if err != nil {
		log.Fatal("❌ Could not load config.json: ", err)
	}

	return &App{
		Config:         &cfg,
		Hub:            serverrealtime.NewHub(),
		Clipboard:      serverclipboard.New(appDir),
		History:        serverhistory.New(appDir),
		Thumbnail:      serverthumbnail.New(),
		AppDir:         appDir,
		ThumbnailLimit: 100 * 1024 * 1024,
		stopCh:         make(chan struct{}),
		mdnsRestartCh:  make(chan struct{}),
		StartTime:      time.Now(),
		RateLimiter:    newRateLimiter(),
	}
}

func (a *App) Run() {
	a.setupLogging()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Recovered from panic in Hub.Run: %v", r)
			}
		}()
		a.Hub.Run()
	}()

	api := &serverhttpapi.Handlers{
		Config:           a.Config,
		ConfigMu:         &a.ConfigMu,
		Hub:              a.Hub,
		Clipboard:        a.Clipboard,
		History:          a.History,
		Thumbnail:        a.Thumbnail,
		GetRole:          a.getRole,
		GetEffectiveRoot: a.getEffectiveRoot,
		AppDir:           a.getAppDir,
		OpenURL:          serverplatform.OpenURL,
		GetAdminIndex:    a.getAdminIndex,
		GetGuestIndex:    a.getGuestIndex,
	}

	// Initial build
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Recovered from panic in initial search index build: %v", r)
			}
		}()
		a.ConfigMu.RLock()
		sharedDir := a.Config.SharedDir
		a.ConfigMu.RUnlock()

		adminIdx := serversearch.NewIndex(sharedDir)
		_ = adminIdx.Build()

		guestDir := filepath.Join(sharedDir, "Public")
		_ = os.MkdirAll(guestDir, 0o755)
		guestIdx := serversearch.NewIndex(guestDir)
		_ = guestIdx.Build()
		a.setIndices(adminIdx, guestIdx)
		log.Println("🔍 Search indices built")
	}()

	// Index Worker (Debounced)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Recovered from panic in search index worker: %v", r)
			}
		}()
		
		timer := time.NewTimer(1 * time.Hour) // Initial long duration
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		var pending bool

		for {
			select {
			case msg := <-a.Hub.NotifyCh:
				if msg == "files" {
					pending = true
					timer.Reset(5 * time.Second)
				}
			case <-timer.C:
				if pending {
					pending = false
					log.Println("🔄 Refreshing search indices (debounced)...")
					a.ConfigMu.RLock()
					sharedDir := a.Config.SharedDir
					a.ConfigMu.RUnlock()

					newAdminIndex := serversearch.NewIndex(sharedDir)
					_ = newAdminIndex.Build()

					guestDir := filepath.Join(sharedDir, "Public")
					_ = os.MkdirAll(guestDir, 0o755)
					newGuestIndex := serversearch.NewIndex(guestDir)
					_ = newGuestIndex.Build()

					a.setIndices(newAdminIndex, newGuestIndex)
					log.Println("✅ Search indices refreshed")
				}
			case <-a.stopCh:
				timer.Stop()
				return
			}
		}
	}()

	if err := os.MkdirAll(a.Config.SharedDir, 0755); err != nil {
		log.Printf("❌ Failed to create SharedDir: %v", err)
	}

	localIP := a.getOutboundIP()
	a.setLastKnownIP(localIP.String())
	log.Printf("📡 Local IP: %s\n", localIP)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Recovered from panic in startIPMonitor: %v", r)
			}
		}()
		a.startIPMonitor()
	}()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Recovered from panic in StartEviction: %v", r)
			}
		}()
		a.Thumbnail.StartEviction(a.ThumbnailLimit, time.Hour)
	}()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Recovered from panic in startMDNS: %v", r)
			}
		}()
		a.startMDNS()
	}()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Recovered from panic in startLogRotationChecker: %v", r)
			}
		}()
		a.startLogRotationChecker()
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", api.HandleWS)
	mux.HandleFunc("/ping", api.HandlePing)
	mux.HandleFunc("/upload", a.requireAuth(api.HandleUpload))
	mux.HandleFunc("/open", a.requireAuth(api.HandleOpen))
	mux.HandleFunc("/thumbnail", a.requireAuth(api.HandleThumbnail))
	mux.HandleFunc("/clipboard", a.requireAuth(api.HandleClipboard))
	mux.HandleFunc("/clipboard/image", a.requireAuth(api.HandleClipboardImage))
	mux.HandleFunc("/clipboard/history", a.requireAuth(api.HandleHistory))
	mux.HandleFunc("/files", a.requireAuth(api.HandleListFiles))
	mux.HandleFunc("/search", a.requireAuth(api.HandleSearch))
	mux.HandleFunc("/delete", a.requireAuth(api.HandleDelete))
	mux.HandleFunc("/download/", a.requireAuth(api.HandleDownload))

	dash := serverdashboard.Handlers{
		Config:    a.Config,
		ConfigMu:  &a.ConfigMu,
		Clipboard: a.Clipboard,
		History:   a.History,
		Hub:       a.Hub,
		AppDir:    a.AppDir,
		LogFile:   a.logFile,
	}
	mux.HandleFunc("/dashboard/", dash.ServeDashboard().ServeHTTP)
	mux.HandleFunc("/api/admin/status", a.requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		dash.HandleStatus(w, r, a.getLastKnownIP(), a.StartTime)
	}))
	mux.HandleFunc("/api/admin/config", a.requireAdmin(dash.HandleConfig))
	mux.HandleFunc("/api/admin/rotate-codes", a.requireAdmin(dash.HandleRotateCodes))
	mux.HandleFunc("/api/admin/logs", a.requireAdmin(dash.HandleLogs))
	mux.HandleFunc("/api/admin/files", a.requireAdmin(dash.HandleFiles))
	mux.HandleFunc("/api/admin/clear-trash", a.requireAdmin(dash.HandleClearTrash))

	if err := a.generateSelfSignedCert(); err != nil {
		log.Fatalf("❌ TLS cert generation failed: %v", err)
	}

	a.ConfigMu.RLock()
	port := a.Config.Port
	a.ConfigMu.RUnlock()

	a.server = &http.Server{
		Addr:           ":" + port,
		Handler:        a.withMiddleware(mux),
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
		},
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Recovered from panic in ListenAndServeTLS: %v", r)
			}
		}()
		log.Printf("🔒 Server listening on https://%s:%s", "0.0.0.0", a.Config.Port)
		if err := a.server.ListenAndServeTLS(filepath.Join(a.getAppDir(), "cert.pem"), filepath.Join(a.getAppDir(), "key.pem")); err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	systray.Run(a.onReady, a.onExit)

	// Graceful shutdown after systray exits
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
}

func (a *App) getAppDir() string {
	return a.AppDir
}

func (a *App) getLastKnownIP() string {
	a.ipMu.RLock()
	defer a.ipMu.RUnlock()
	return a.lastKnownIP
}

func (a *App) setLastKnownIP(ip string) {
	a.ipMu.Lock()
	defer a.ipMu.Unlock()
	a.lastKnownIP = ip
}

func (a *App) setupLogging() {
	logFile := filepath.Join(a.getAppDir(), "server_log.txt")
	if info, err := os.Stat(logFile); err == nil && info.Size() > 10*1024*1024 {
		a.rotateLogs(logFile)
	}
	
	a.logMu.Lock()
	defer a.logMu.Unlock()
	
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("❌ Failed to open log file: %v\n", err)
		return
	}
	a.logFile = f
	multi := io.MultiWriter(f, os.Stdout)
	log.SetOutput(multi)
	log.Println("🚀 --- K-Share Secure Server Started (HTTPS) ---")
}

func (a *App) getRole(r *http.Request) string {
	a.ConfigMu.RLock()
	defer a.ConfigMu.RUnlock()
	return serverauth.Role(r, a.Config)
}

func (a *App) getEffectiveRoot(r *http.Request) (string, error) {
	a.ConfigMu.RLock()
	defer a.ConfigMu.RUnlock()
	return serverauth.EffectiveRoot(r, a.Config)
}

func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.getRole(r) == "none" {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			if ip == "" {
				ip = r.RemoteAddr
			}
			if a.RateLimiter != nil {
				a.RateLimiter.recordAuthFailure(ip)
			}
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (a *App) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.getRole(r) != "admin" {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			if ip == "" {
				ip = r.RemoteAddr
			}
			if a.RateLimiter != nil {
				a.RateLimiter.recordAuthFailure(ip)
			}
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (a *App) getOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		if udpAddr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			return udpAddr.IP
		}
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Printf("⚠️ Failed to get interface addresses: %v", err)
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

func (a *App) startIPMonitor() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			currentIP := a.getOutboundIP().String()
			if currentIP != a.getLastKnownIP() {
				log.Printf("🔄 IP Changed: %s → %s\n", a.getLastKnownIP(), currentIP)
				a.setLastKnownIP(currentIP)
				a.updateSystrayIP(currentIP)
				a.restartMDNS()
				a.Hub.Notify("ip_change")
			}
		case <-a.stopCh:
			return
		}
	}
}

func (a *App) updateSystrayIP(ip string) {
	if a.mIP != nil {
		a.mIP.SetTitle("IP: " + ip)
	}
}

func (a *App) restartMDNS() {
	a.mdnsMu.Lock()
	defer a.mdnsMu.Unlock()
	select {
	case <-a.mdnsRestartCh:
		// Already closed
	default:
		close(a.mdnsRestartCh)
	}
	a.mdnsRestartCh = make(chan struct{})
}

func (a *App) generateSelfSignedCert() error {
	certPath := filepath.Join(a.getAppDir(), "cert.pem")
	keyPath := filepath.Join(a.getAppDir(), "key.pem")

	// Check if existing cert is present and not expiring soon
	if data, err := os.ReadFile(certPath); err == nil {
		if block, _ := pem.Decode(data); block != nil {
			if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
				if time.Until(cert.NotAfter) > 30*24*time.Hour {
					if _, err := os.Stat(keyPath); err == nil {
						return nil
					}
				}
			}
		}
	}
	log.Println("🔐 Generating self-signed TLS certificate...")

	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return fmt.Errorf("keygen: %w", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, _ := rand.Int(rand.Reader, serialNumberLimit)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"K-Share Self-Signed"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(90 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Printf("⚠️ Failed to get interface addresses for cert: %v", err)
	} else {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				template.IPAddresses = append(template.IPAddresses, ipnet.IP)
			}
		}
	}
	template.IPAddresses = append(template.IPAddresses, net.ParseIP("127.0.0.1"))

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("create cert: %w", err)
	}

	certOut, err := os.Create(certPath)
	if err != nil {
		return fmt.Errorf("create cert file: %w", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		certOut.Close()
		return fmt.Errorf("encode cert: %w", err)
	}
	certOut.Close()

	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create key file: %w", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		keyOut.Close()
		return fmt.Errorf("encode key: %w", err)
	}
	keyOut.Close()

	log.Println("✅ Certificate generated.")
	return nil
}

func (a *App) loadIcon() []byte {
	exe, _ := os.Executable()
	iconPath := filepath.Join(filepath.Dir(exe), "Icon.png")
	iconData, err := os.ReadFile(iconPath)
	if err != nil || len(iconData) == 0 {
		return generateIcon()
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(iconData))
	if err != nil {
		return generateIcon()
	}

	ico := new(bytes.Buffer)
	mustWrite := func(v any) {
		if err := binary.Write(ico, binary.LittleEndian, v); err != nil {
			log.Printf("Error writing icon data: %v", err)
		}
	}

	mustWrite(uint16(0))
	mustWrite(uint16(1))
	mustWrite(uint16(1))

	w := cfg.Width
	h := cfg.Height
	if w >= 256 {
		w = 0
	}
	if h >= 256 {
		h = 0
	}
	mustWrite(uint8(w))
	mustWrite(uint8(h))
	mustWrite(uint8(0))
	mustWrite(uint8(0))
	mustWrite(uint16(1))
	mustWrite(uint16(32))
	mustWrite(uint32(len(iconData)))
	mustWrite(uint32(22))
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
	_ = png.Encode(buf, img)
	pngBytes := buf.Bytes()
	ico := new(bytes.Buffer)
	mustWrite := func(v any) {
		if err := binary.Write(ico, binary.LittleEndian, v); err != nil {
			log.Printf("Error writing generated icon data: %v", err)
		}
	}

	mustWrite(uint16(0))
	mustWrite(uint16(1))
	mustWrite(uint16(1))
	mustWrite(uint8(32))
	mustWrite(uint8(32))
	mustWrite(uint8(0))
	mustWrite(uint8(0))
	mustWrite(uint16(1))
	mustWrite(uint16(32))
	mustWrite(uint32(len(pngBytes)))
	mustWrite(uint32(22))
	ico.Write(pngBytes)
	return ico.Bytes()
}

func (a *App) onReady() {
	systray.SetTitle("K-Share")
	systray.SetTooltip("K-Share-Server: Encrypted Local Sharing")
	systray.SetIcon(a.loadIcon())

	sentinel := filepath.Join(a.getAppDir(), ".context_menu_installed")
	if _, err := os.Stat(sentinel); os.IsNotExist(err) {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Recovered from panic in ContextMenu install: %v", r)
				}
			}()
			serverplatform.InstallContextMenu()
			_ = os.WriteFile(sentinel, []byte("1"), 0o644)
		}()
	}

	a.mIP = systray.AddMenuItem("IP: "+a.getOutboundIP().String(), "")
	a.mIP.Disable()

	systray.AddSeparator()
	mRefreshIP := systray.AddMenuItem("Refresh IP", "Check for IP changes")
	mOpenShared := systray.AddMenuItem("Open Shared Folder", "Open storage location")
	mOpenDash := systray.AddMenuItem("Open Admin Dashboard", "Manage server via web")
	systray.AddSeparator()
	mExit := systray.AddMenuItem("Exit", "Quit K-Share")

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Recovered from panic in systray loop: %v", r)
			}
		}()
		for {
			select {
			case <-mOpenShared.ClickedCh:
				_ = serverplatform.OpenFolder(a.Config.SharedDir)
			case <-mOpenDash.ClickedCh:
				url := fmt.Sprintf("https://localhost:%s/dashboard", a.Config.Port)
				_ = serverplatform.OpenURL(url)
			case <-mRefreshIP.ClickedCh:
				newIP := a.getOutboundIP()
				a.mIP.SetTitle("IP: " + newIP.String())
				log.Printf("🔄 Manual IP Refresh: %s\n", newIP.String())
			case <-mExit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

func (a *App) onExit() {
	close(a.stopCh)
	if a.Thumbnail != nil {
		a.Thumbnail.Stop()
	}
	if a.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.server.Shutdown(ctx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	}
	
	a.logMu.Lock()
	defer a.logMu.Unlock()
	if a.logFile != nil {
		_ = a.logFile.Close()
	}
}

func (a *App) rotateLogs(logFile string) {
	const generations = 5
	for i := generations - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", logFile, i)
		newPath := fmt.Sprintf("%s.%d", logFile, i+1)
		_ = os.Rename(oldPath, newPath)
	}
	_ = os.Rename(logFile, logFile+".1")
}

func (a *App) startLogRotationChecker() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			logFile := filepath.Join(a.getAppDir(), "server_log.txt")
			if info, err := os.Stat(logFile); err == nil && info.Size() > 10*1024*1024 {
				log.Println("🗑️ Rotating log file")
				
				a.logMu.Lock()
				if a.logFile != nil {
					_ = a.logFile.Close()
				}
				a.rotateLogs(logFile)
				f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err == nil {
					a.logFile = f
					multi := io.MultiWriter(f, os.Stdout)
					log.SetOutput(multi)
				} else {
					fmt.Printf("❌ Failed to reopen log file after rotation: %v\n", err)
					log.SetOutput(os.Stdout)
				}
				a.logMu.Unlock()
			}
		case <-a.stopCh:
			return
		}
	}
}

func (a *App) startMDNS() {
	for {
		select {
		case <-a.stopCh:
			return
		default:
		}

		port, _ := strconv.Atoi(a.Config.Port)
		server, err := zeroconf.Register("K-Share Server", "_kshare._tcp", "local.", port, []string{"txtv=0"}, nil)
		if err != nil {
			log.Println("⚠️ Failed to start mDNS:", err)
			select {
			case <-a.stopCh:
				return
			case <-a.mdnsRestartCh:
				continue
			case <-time.After(30 * time.Second):
				continue
			}
		}

		select {
		case <-a.stopCh:
			server.Shutdown()
			return
		case <-a.mdnsRestartCh:
			server.Shutdown()
		}
	}
}

func SendToPhone(filePath string) {
	fmt.Printf("🚀 Sending to phone: %s\n", filePath)

	cfg, err := serverconfig.Load()
	if err != nil {
		log.Fatalf("❌ Could not load config.json: %v", err)
	}

	if _, err := os.Stat(filePath); err != nil {
		log.Fatalf("❌ File check failed: %v", err)
	}

	url := fmt.Sprintf("https://127.0.0.1:%s/upload?path=%s", cfg.Port, filepath.Base(filePath))

	certPath := filepath.Join(serverconfig.AppDir(), "cert.pem")
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		log.Fatalf("❌ Cannot read server cert: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(certPEM)

	tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}
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
	req.Header.Set("Authorization", "Bearer "+cfg.AdminCode)

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
	if runtime.GOOS == "windows" {
		time.Sleep(2 * time.Second)
	}
}

// Middleware

type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*rateLimitEntry
}

type rateLimitEntry struct {
	tokens       int
	authFailures int
	lastSeen     time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{clients: make(map[string]*rateLimitEntry)}
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()

	// Periodic cleanup (simple: 1 in 100 requests)
	if time.Now().UnixNano()%100 == 0 {
		for k, v := range rl.clients {
			if now.Sub(v.lastSeen) > 10*time.Minute {
				delete(rl.clients, k)
			}
		}
	}

	entry, ok := rl.clients[ip]
	if !ok {
		rl.clients[ip] = &rateLimitEntry{tokens: 99, lastSeen: now}
		return true
	}

	if entry.authFailures > 10 {
		if now.Sub(entry.lastSeen) < 1*time.Minute {
			return false
		}
		entry.authFailures = 0 // Reset after timeout
	}

	elapsed := int(now.Sub(entry.lastSeen).Seconds())
	entry.tokens += elapsed
	if entry.tokens > 100 {
		entry.tokens = 100
	}
	entry.lastSeen = now
	if entry.tokens > 0 {
		entry.tokens--
		return true
	}
	return false
}

func (rl *rateLimiter) recordAuthFailure(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if entry, ok := rl.clients[ip]; ok {
		entry.authFailures++
		entry.lastSeen = time.Now()
	} else {
		rl.clients[ip] = &rateLimitEntry{tokens: 100, authFailures: 1, lastSeen: time.Now()}
	}
}

func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if ip == "" {
			ip = r.RemoteAddr
		}
		if !rl.allow(ip) {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

func (a *App) withMiddleware(handler http.Handler) http.Handler {
	// RateLimiter is initialized in New(); do not reset it here
	// to preserve auth failure state across requests.
	if a.RateLimiter == nil {
		a.RateLimiter = newRateLimiter()
	}
	return a.securityHeaders(a.RateLimiter.middleware(handler))
}
