package api

import (
	"desktop-app/crypto"
	"encoding/json"
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
	OnClipUpdate      func()
	OnClipGuestUpdate func()
	OnClipImageUpdate func()
	OnHistoryUpdate   func()
	OnFilesUpdate     func()
	OnStatusChange    func(string)
	stopChan          chan struct{}
	mu                sync.RWMutex
}

func NewWSClient(serverIP, authCode string) *WSClient {
	return &WSClient{
		serverIP: serverIP,
		authCode: authCode,
		stopChan: make(chan struct{}),
	}
}

func (ws *WSClient) SetServerIP(serverIP string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.serverIP = serverIP
}

func (ws *WSClient) SetAuthCode(authCode string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.authCode = authCode
}

func (ws *WSClient) Connect() error {
	ws.mu.Lock()
	serverIP := ws.serverIP
	authCode := ws.authCode
	onStatusChange := ws.OnStatusChange
	
	// Construct WebSocket URL
	wsURL := "wss://" + serverIP + "/ws"

	// Use TOFU certificate pinning
	dialer := *websocket.DefaultDialer
	dialer.TLSClientConfig = crypto.CreateTLSConfig(nil)

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

	// Close old connection if any
	if ws.conn != nil {
		ws.conn.Close()
	}
	ws.conn = conn

	// Ensure stopChan is armed
	select {
	case <-ws.stopChan:
		ws.stopChan = make(chan struct{})
	default:
	}

	if onStatusChange != nil {
		onStatusChange("🟢 Connected")
	}
	ws.mu.Unlock()

	go ws.listen(conn) // Pass local reference
	return nil
}

func (ws *WSClient) listen(conn *websocket.Conn) {
	defer func() {
		conn.Close()
		ws.mu.RLock()
		onStatusChange := ws.OnStatusChange
		ws.mu.RUnlock()
		if onStatusChange != nil {
			onStatusChange("🔴 Disconnected")
		}
	}()

	for {
		select {
		case <-ws.stopChan:
			return
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				// If connection was closed by us, just return
				if strings.Contains(err.Error(), "use of closed") || strings.Contains(err.Error(), "normal") {
					return
				}

				// Reconnect check
				select {
				case <-ws.stopChan:
					return
				default:
					time.Sleep(5 * time.Second)
					_ = ws.Connect()
					return
				}
			}

			var msg WSMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				continue
			}

			// Trigger appropriate callback
			ws.mu.RLock()
			onClip := ws.OnClipUpdate
			onClipGuest := ws.OnClipGuestUpdate
			onClipImg := ws.OnClipImageUpdate
			onHist := ws.OnHistoryUpdate
			onFiles := ws.OnFilesUpdate
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

	select {
	case <-ws.stopChan:
		// already closed
		return
	default:
		close(ws.stopChan)
	}

	if ws.conn != nil {
		ws.conn.Close()
		ws.conn = nil
	}
}

func (ws *WSClient) ReArm() {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	
	select {
	case <-ws.stopChan:
		ws.stopChan = make(chan struct{})
	default:
		// already armed
	}
}
