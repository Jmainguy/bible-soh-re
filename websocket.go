package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// BroadcastMessage represents a message to be sent to clients
type BroadcastMessage struct {
	Scope   socialScope            `json:"-"`
	Type    string                 `json:"type"`   // "verse_comment", "note_comment", "reaction"
	Action  string                 `json:"action"` // "create", "update", "delete"
	Data    map[string]interface{} `json:"data"`   // Additional data
	Book    string                 `json:"book,omitempty"`
	Chapter int                    `json:"chapter,omitempty"`
	Verse   int                    `json:"verse,omitempty"`
	NoteID  int                    `json:"note_id,omitempty"`
}

// Client represents a connected WebSocket client
type Client struct {
	hub       *Hub
	db        *Database
	sessionID string
	userID    int64
	conn      *websocket.Conn
	send      chan []byte
}

// Hub maintains connected clients and broadcasts messages
type Hub struct {
	stop       chan struct{}
	clients    map[*Client]bool
	broadcast  chan BroadcastMessage
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

var hub = &Hub{
	clients:    make(map[*Client]bool),
	broadcast:  make(chan BroadcastMessage, 256),
	register:   make(chan *Client),
	unregister: make(chan *Client),
}

// Run starts the hub
func (h *Hub) Run() {
	for {
		select {
		case <-h.stop:
			return
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
		case message := <-h.broadcast:
			data, err := json.Marshal(message)
			if err != nil {
				continue
			}
			h.mu.Lock()
			for client := range h.clients {
				session, err := client.db.GetSession(client.sessionID)
				if err != nil || session.UserID != client.userID || !client.db.canAccessScope(message.Scope, client.userID) {
					continue
				}
				select {
				case client.send <- data:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.Unlock()
		}
	}
}

// BroadcastUpdate sends an update to all connected clients
func BroadcastUpdate(msg BroadcastMessage) {
	if msg.Scope.GroupID.Valid {
		if msg.Data == nil {
			msg.Data = map[string]interface{}{}
		}
		msg.Data["group_id"] = msg.Scope.GroupID.Int64
	}
	if publishLive != nil {
		publishLive(msg)
		return
	}
	queueLiveUpdate(msg)
}

func queueLiveUpdate(msg BroadcastMessage) {
	select {
	case hub.broadcast <- msg:
	default:
		log.Print("Live update queue full; clients can refresh")
	}
}

// readPump reads messages from the WebSocket connection
func (c *Client) readPump() {
	defer func() {
		select {
		case c.hub.unregister <- c:
		case <-c.hub.stop:
		}
		if err := c.conn.Close(); err != nil {
			log.Printf("Failed to close WebSocket connection: %v", err)
		}
	}()

	c.conn.SetReadLimit(4096)
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
	}
}

// writePump writes messages to the WebSocket connection
func (c *Client) writePump() {
	defer func() {
		if err := c.conn.Close(); err != nil {
			log.Printf("Failed to close WebSocket connection: %v", err)
		}
	}()

	for message := range c.send {
		_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		err := c.conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			break
		}
	}
}

// HandleWebSocket handles WebSocket connections
func (h *AuthHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	user := h.requireUser(w, r)
	if user == nil {
		return
	}
	cookie, _ := r.Cookie(sessionCookieName)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		conn:      conn,
		hub:       hub,
		db:        h.db,
		userID:    user.ID,
		sessionID: cookie.Value,
		send:      make(chan []byte, 256),
	}

	hub.register <- client

	go client.writePump()
	go client.readPump()
}
