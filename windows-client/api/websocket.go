package api

import (
	"encoding/json"
	"k-share-client/crypto"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type WSMessage struct {
	Type string `json:"type"` // "clip", "history", "files"
}

type WSClient struct {
	serverIP        string
	authCode        string
	conn            *websocket.Conn
	OnClipUpdate    func()
	OnHistoryUpdate func()
	OnFilesUpdate   func()
	stopChan        chan struct{}
}

func NewWSClient(serverIP, authCode string) *WSClient {
	return &WSClient{
		serverIP: serverIP,
		authCode: authCode,
		stopChan: make(chan struct{}),
	}
}

func (ws *WSClient) Connect() error {
	// Construct WebSocket URL
	wsURL := "wss://" + ws.serverIP + "/ws"

	// Use TOFU certificate pinning
	dialer := *websocket.DefaultDialer
	dialer.TLSClientConfig = crypto.CreateTLSConfig(nil)

	var err error
	header := http.Header{}
	header.Add("Authorization", "Bearer "+ws.authCode)

	ws.conn, _, err = dialer.Dial(wsURL, header)
	if err != nil {
		return err
	}

	log.Println("WebSocket connected securely")
	go ws.listen()
	return nil
}

func (ws *WSClient) listen() {
	defer func() {
		if ws.conn != nil {
			ws.conn.Close()
		}
	}()

	for {
		select {
		case <-ws.stopChan:
			return
		default:
			_, message, err := ws.conn.ReadMessage()
			if err != nil {
				if !strings.Contains(err.Error(), "use of closed") {
					log.Printf("WebSocket read error: %v", err)
				}
				// Try to reconnect after delay
				time.Sleep(5 * time.Second)
				ws.Connect()
				return
			}

			var msg WSMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				continue
			}

			// Trigger appropriate callback
			switch msg.Type {
			case "clip":
				if ws.OnClipUpdate != nil {
					ws.OnClipUpdate()
				}
			case "history":
				if ws.OnHistoryUpdate != nil {
					ws.OnHistoryUpdate()
				}
			case "files":
				if ws.OnFilesUpdate != nil {
					ws.OnFilesUpdate()
				}
			}
		}
	}
}

func (ws *WSClient) Close() {
	close(ws.stopChan)
	if ws.conn != nil {
		ws.conn.Close()
	}
}
