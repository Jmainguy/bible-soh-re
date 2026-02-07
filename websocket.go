package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

// BroadcastMessage represents a message to be sent to clients
type BroadcastMessage struct {
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
	conn *websocket.Conn
	send chan []byte
}

// Hub maintains connected clients and broadcasts messages
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

var hub = &Hub{
	clients:    make(map[*Client]bool),
	broadcast:  make(chan []byte, 256),
	register:   make(chan *Client),
	unregister: make(chan *Client),
}

// Run starts the hub
func (h *Hub) Run() {
	for {
		select {
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
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// BroadcastUpdate sends an update to all connected clients
func BroadcastUpdate(msg BroadcastMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling broadcast message: %v", err)
		return
	}
	hub.broadcast <- data
}

// readPump reads messages from the WebSocket connection
func (c *Client) readPump() {
	defer func() {
		hub.unregister <- c
		if err := c.conn.Close(); err != nil {
			log.Printf("Failed to close WebSocket connection: %v", err)
		}
	}()

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
		err := c.conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			break
		}
	}
}

// HandleWebSocket handles WebSocket connections
func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		conn: conn,
		send: make(chan []byte, 256),
	}

	hub.register <- client

	go client.writePump()
	go client.readPump()
}
