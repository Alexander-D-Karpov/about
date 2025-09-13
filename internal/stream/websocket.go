package stream

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mutex      sync.RWMutex
}

type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	ReadBufferSize:   1024,
	WriteBufferSize:  1024,
	HandshakeTimeout: 10 * time.Second,
}

func New() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 1024),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Hub.Run panic recovered: %v", r)
			go h.Run()
		}
	}()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case client := <-h.register:
			h.mutex.Lock()
			if len(h.clients) >= 1000 {
				h.mutex.Unlock()
				client.conn.Close()
				continue
			}
			h.clients[client] = true
			clientCount := len(h.clients)
			h.mutex.Unlock()

			log.Printf("Client connected. Total clients: %d", clientCount)
			h.broadcastClientCount(clientCount)

		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				select {
				case <-client.send:
				default:
				}
				close(client.send)
			}
			clientCount := len(h.clients)
			h.mutex.Unlock()

			log.Printf("Client disconnected. Total clients: %d", clientCount)
			h.broadcastClientCount(clientCount)

		case message := <-h.broadcast:
			h.mutex.RLock()
			clientCount := len(h.clients)
			if clientCount == 0 {
				h.mutex.RUnlock()
				continue
			}

			clients := make([]*Client, 0, clientCount)
			for client := range h.clients {
				clients = append(clients, client)
			}
			h.mutex.RUnlock()

			deadClients := make([]*Client, 0)
			for _, client := range clients {
				select {
				case client.send <- message:
				default:
					deadClients = append(deadClients, client)
				}
			}

			if len(deadClients) > 0 {
				h.mutex.Lock()
				for _, client := range deadClients {
					if _, ok := h.clients[client]; ok {
						delete(h.clients, client)
						select {
						case <-client.send:
						default:
						}
						close(client.send)
					}
				}
				h.mutex.Unlock()
			}

		case <-ticker.C:
			h.cleanupStaleConnections()
		}
	}
}

func (h *Hub) cleanupStaleConnections() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	deadClients := make([]*Client, 0)
	for client := range h.clients {
		if client.conn != nil {
			client.conn.SetReadDeadline(time.Now().Add(time.Second))
			if _, _, err := client.conn.NextReader(); err != nil {
				deadClients = append(deadClients, client)
			}
		}
	}

	for _, client := range deadClients {
		delete(h.clients, client)
		if client.conn != nil {
			client.conn.Close()
		}
		select {
		case <-client.send:
		default:
		}
		close(client.send)
	}

	if len(deadClients) > 0 {
		log.Printf("Cleaned up %d stale connections", len(deadClients))
	}
}

func (h *Hub) broadcastClientCount(count int) {
	message := Message{
		Type: "client_count_update",
		Data: map[string]interface{}{
			"count":     count,
			"timestamp": time.Now().Unix(),
		},
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling client count message: %v", err)
		return
	}

	select {
	case h.broadcast <- jsonData:
	default:
		log.Printf("Broadcast channel full, dropping client count update")
	}
}

func (h *Hub) Broadcast(msgType string, data interface{}) {
	message := Message{
		Type: msgType,
		Data: data,
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}

	h.mutex.RLock()
	clientCount := len(h.clients)
	h.mutex.RUnlock()

	if clientCount == 0 {
		return
	}

	select {
	case h.broadcast <- jsonData:
	default:
		select {
		case <-h.broadcast:
			select {
			case h.broadcast <- jsonData:
			default:
				log.Printf("Broadcast channel still full after drain, dropping message type: %s", msgType)
			}
		default:
			log.Printf("Broadcast channel full, dropping message type: %s", msgType)
		}
	}
}

func (h *Hub) GetClientCount() int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return len(h.clients)
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 256),
	}

	client.hub.register <- client

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("readPump panic recovered: %v", r)
		}
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			break
		}

		if len(message) > 1024 {
			log.Printf("Message too large: %d bytes", len(message))
			break
		}

		var msg Message
		if err := json.Unmarshal(message, &msg); err == nil {
			c.handleMessage(msg)
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("writePump panic recovered: %v", r)
		}
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			if _, err := w.Write(message); err != nil {
				w.Close()
				return
			}

			n := len(c.send)
			for i := 0; i < n && i < 10; i++ {
				select {
				case msg := <-c.send:
					w.Write([]byte{'\n'})
					w.Write(msg)
				default:
					break
				}
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) handleMessage(msg Message) {
	switch msg.Type {
	case "ping":
		response := Message{Type: "pong", Data: nil}
		if data, err := json.Marshal(response); err == nil {
			select {
			case c.send <- data:
			default:
			}
		}
	case "register":
		log.Printf("Client registered with data: %v", msg.Data)
	case "get_client_count":
		count := c.hub.GetClientCount()
		response := Message{
			Type: "client_count_update",
			Data: map[string]interface{}{
				"count":     count,
				"timestamp": time.Now().Unix(),
			},
		}
		if data, err := json.Marshal(response); err == nil {
			select {
			case c.send <- data:
			default:
			}
		}
	}
}
