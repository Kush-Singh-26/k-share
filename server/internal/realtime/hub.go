package realtime

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 54 * time.Second
	maxMessageSize = 512
)

type client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

func (c *client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
	}
}

type Hub struct {
	clients    map[*client]bool
	broadcast  chan []byte
	register   chan *client
	unregister chan *client
	mutex      sync.RWMutex
	NotifyCh   chan string
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *client),
		unregister: make(chan *client),
		NotifyCh:   make(chan string, 10),
	}
}

func (h *Hub) Run() {
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
				close(client.send)
			}
			h.mutex.Unlock()
		case message := <-h.broadcast:
			h.mutex.Lock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mutex.Unlock()
		}
	}
}

func (h *Hub) Notify(changeType string) {
	msg, _ := json.Marshal(map[string]string{"type": changeType})
	select {
	case h.broadcast <- msg:
	default:
		// Broadcast channel is full; drop the message
	}
	// Internal Notify
	select {
	case h.NotifyCh <- changeType:
	default:
	}
}

var Upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow if origin is same host or localhost
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		host, _, _ := net.SplitHostPort(r.Host)
		if host == "" {
			host = r.Host
		}
		return u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == host
	},
}

func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	c := &client{hub: h, conn: conn, send: make(chan []byte, 64)}
	h.register <- c
	go c.writePump()
	go c.readPump()
}

func (h *Hub) ClientCount() int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return len(h.clients)
}
