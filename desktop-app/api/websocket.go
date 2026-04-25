package api

import (
	"context"
	"desktop-app/crypto"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WSMessage struct {
	Type string `json:"type"` // "clip", "clip_guest", "history", "files"
}

	type WSClient struct {
		serverIP          string
		authCode          string
		conn              *websocket.Conn
		
		// P2: Private callbacks with lock-safe setters
		onClipUpdate      func()
		onClipGuestUpdate func()
		onClipImageUpdate func()
		onHistoryUpdate   func()
		onFilesUpdate     func()
		onStatusChange    func(string)
		
		// P1: context-cancel instead of stop channel
		cancel            context.CancelFunc
		mu                sync.RWMutex
		
		// P0: Prevent unbounded goroutine growth on reconnect
		reconnectMu       sync.Mutex
		reconnecting      bool
	}

func NewWSClient(serverIP, authCode string) *WSClient {
	return &WSClient{
		serverIP: normalizeAddress(serverIP),
		authCode: authCode,
	}
}

// Setters for callbacks (P2)
func (ws *WSClient) SetOnClipUpdate(f func())      { ws.mu.Lock(); defer ws.mu.Unlock(); ws.onClipUpdate = f }
func (ws *WSClient) SetOnClipGuestUpdate(f func()) { ws.mu.Lock(); defer ws.mu.Unlock(); ws.onClipGuestUpdate = f }
func (ws *WSClient) SetOnClipImageUpdate(f func()) { ws.mu.Lock(); defer ws.mu.Unlock(); ws.onClipImageUpdate = f }
func (ws *WSClient) SetOnHistoryUpdate(f func())   { ws.mu.Lock(); defer ws.mu.Unlock(); ws.onHistoryUpdate = f }
func (ws *WSClient) SetOnFilesUpdate(f func())     { ws.mu.Lock(); defer ws.mu.Unlock(); ws.onFilesUpdate = f }
func (ws *WSClient) SetOnStatusChange(f func(string)) { ws.mu.Lock(); defer ws.mu.Unlock(); ws.onStatusChange = f }

func (ws *WSClient) SetServerIP(serverIP string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.serverIP = normalizeAddress(serverIP)
}

func (ws *WSClient) SetAuthCode(authCode string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.authCode = authCode
}

func (ws *WSClient) connectInternal() error {
	ws.mu.Lock()
	serverIP := ws.serverIP
	authCode := ws.authCode
	onStatusChange := ws.onStatusChange
	
	// Construct WebSocket URL
	wsURL := "wss://" + serverIP + "/ws"

	// Use TOFU certificate pinning
	host, _, _ := net.SplitHostPort(serverIP)
	if host == "" {
		host = serverIP
	}

	dialer := *websocket.DefaultDialer
	dialer.TLSClientConfig = crypto.CreateTLSConfig(host, nil)

	header := http.Header{}
	header.Add("Authorization", "Bearer "+authCode)

	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		if onStatusChange != nil {
			onStatusChange("🔴 Connection Failed")
		}
		ws.mu.Unlock()
		return err
	}

	// P1: Clean stop of previous connection lifecycle
	if ws.cancel != nil {
		ws.cancel()
	}
	
	// Close old connection if any
	if ws.conn != nil {
		ws.conn.Close()
	}
	ws.conn = conn

	ctx, cancel := context.WithCancel(context.Background())
	ws.cancel = cancel

	if onStatusChange != nil {
		onStatusChange("🟢 Connected")
	}
	ws.mu.Unlock()

	go ws.listen(ctx, conn) // P1: Pass connection context
	return nil
}

func (ws *WSClient) Connect() error {
	return ws.connectInternal()
}

func (ws *WSClient) listen(ctx context.Context, conn *websocket.Conn) {
	defer func() {
		conn.Close()
		ws.mu.RLock()
		onStatusChange := ws.onStatusChange
		ws.mu.RUnlock()
		if onStatusChange != nil {
			onStatusChange("🔴 Disconnected")
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				// If connection was closed or ctx canceled, return
				select {
				case <-ctx.Done():
					return
				default:
				}

				if strings.Contains(err.Error(), "use of closed") || strings.Contains(err.Error(), "normal") {
					return
				}

				// Reconnect with exponential backoff (deduped)
				ws.reconnectMu.Lock()
				if ws.reconnecting {
					ws.reconnectMu.Unlock()
					return
				}
				ws.reconnecting = true
				ws.reconnectMu.Unlock()

				backoff := 1 * time.Second
				for i := 0; i < 5; i++ { // Max 5 retries in this loop
					timer := time.NewTimer(backoff)
					select {
					case <-ctx.Done():
						timer.Stop()
						ws.reconnectMu.Lock()
						ws.reconnecting = false
						ws.reconnectMu.Unlock()
						return
					case <-timer.C:
						if err := ws.connectInternal(); err == nil {
							ws.reconnectMu.Lock()
							ws.reconnecting = false
							ws.reconnectMu.Unlock()
							return
						}
						backoff *= 2
						if backoff > 30*time.Second {
							backoff = 30 * time.Second
						}
					}
				}
				ws.reconnectMu.Lock()
				ws.reconnecting = false
				ws.reconnectMu.Unlock()
				return
			}

			var msg WSMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				continue
			}

			// Trigger appropriate callback
			ws.mu.RLock()
			onClip := ws.onClipUpdate
			onClipGuest := ws.onClipGuestUpdate
			onClipImg := ws.onClipImageUpdate
			onHist := ws.onHistoryUpdate
			onFiles := ws.onFilesUpdate
			ws.mu.RUnlock()

			switch msg.Type {
			case "clip":
				if onClip != nil {
					onClip()
				}
			case "clip_guest":
				if onClipGuest != nil {
					onClipGuest()
				}
			case "clip_image":
				if onClipImg != nil {
					onClipImg()
				}
			case "history":
				if onHist != nil {
					onHist()
				}
			case "files":
				if onFiles != nil {
					onFiles()
				}
			}
		}
	}
}

func (ws *WSClient) Close() {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if ws.cancel != nil {
		ws.cancel()
		ws.cancel = nil
	}

	if ws.conn != nil {
		ws.conn.Close()
		ws.conn = nil
	}
}
