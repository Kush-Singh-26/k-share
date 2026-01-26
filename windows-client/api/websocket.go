package api

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type WSMessage struct {
	Type string `json:"type"` // "clip", "history", "files"
}

type WSClient struct {
	serverIP        string
	conn            *websocket.Conn
	OnClipUpdate    func()
	OnHistoryUpdate func()
	OnFilesUpdate   func()
	stopChan        chan struct{}
}

func NewWSClient(serverIP string) *WSClient {
	return &WSClient{
		serverIP: serverIP,
		stopChan: make(chan struct{}),
	}
}

func (ws *WSClient) Connect() error {
	// Construct WebSocket URL
	wsURL := "ws://" + ws.serverIP + "/ws"

	var err error
	ws.conn, _, err = websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}

	log.Println("WebSocket connected")
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
