// Package web provides the HTTP server, WebSocket hub, and REST API handlers
// for the Velkron Pulse dashboard.
package web

import (
	"crypto/subtle"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

const (
	maxWebSocketClients = 32
	maxWebSocketMessage = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     checkWebSocketOrigin,
}

type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	token      string
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (h *Hub) SetAuth(token string) {
	h.token = token
}

func checkWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	host := r.Host
	return origin == "http://"+host || origin == "https://"+host
}

func (h *Hub) authenticateWebSocket(r *http.Request) bool {
	token := extractToken(r)
	if token == "" || h.token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(h.token)) == 1
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("[ws] client connected (%d total)", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("[ws] client disconnected (%d total)", len(h.clients))

		case message := <-h.broadcast:
			h.mu.Lock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) Broadcast(data []byte) {
	h.broadcast <- data
}

func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !h.authenticateWebSocket(r) {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	h.mu.RLock()
	if len(h.clients) >= maxWebSocketClients {
		h.mu.RUnlock()
		writeError(w, http.StatusServiceUnavailable, "Too many WebSocket clients")
		return
	}
	h.mu.RUnlock()

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ws] upgrade error: %v", err)
		return
	}
	conn.SetReadLimit(maxWebSocketMessage)

	client := &Client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 256),
	}
	h.register <- client

	go client.writePump()
	client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[ws] read error: %v", err)
			}
			break
		}
	}
}

func (c *Client) writePump() {
	defer c.conn.Close()

	for message := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			if !strings.Contains(err.Error(), "use of closed network connection") {
				log.Printf("[ws] write error: %v", err)
			}
			return
		}
	}
}
