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
	for {
		select {
		case client := <-h.register:
			h.mutex.Lock()
			h.clients[client] = true
			clientCount := len(h.clients)
			h.mutex.Unlock()

			log.Printf("Client connected. Total clients: %d", clientCount)

			go func(c *Client, count int) {
				time.Sleep(100 * time.Millisecond)

				h.mutex.RLock()
				_, stillConnected := h.clients[c]
				currentCount := len(h.clients)
				h.mutex.RUnlock()

				if !stillConnected {
					return
				}

				message := Message{
					Type: "client_count_update",
					Data: map[string]interface{}{
						"count":     currentCount,
						"timestamp": time.Now().Unix(),
					},
				}

				jsonData, err := json.Marshal(message)
				if err != nil {
					return
				}

				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("Recovered from panic sending to client: %v", r)
						}
					}()

					select {
					case c.send <- jsonData:
					default:
					}
				}()
			}(client, clientCount)

			h.broadcastClientCount(clientCount)

		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("Recovered from panic closing client channel: %v", r)
						}
					}()
					close(client.send)
				}()
			}
			clientCount := len(h.clients)
			h.mutex.Unlock()

			log.Printf("Client disconnected. Total clients: %d", clientCount)
			h.broadcastClientCount(clientCount)

		case message := <-h.broadcast:
			h.mutex.RLock()
			clientCount := len(h.clients)
			h.mutex.RUnlock()

			if clientCount == 0 {
				continue
			}

			h.mutex.Lock()
			for client := range h.clients {
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("Recovered from panic broadcasting to client: %v", r)
							delete(h.clients, client)
						}
					}()

					select {
					case client.send <- message:
					default:
						delete(h.clients, client)
						func() {
							defer func() {
								if r := recover(); r != nil {
								}
							}()
							close(client.send)
						}()
					}
				}()
			}
			h.mutex.Unlock()
		}
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
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		var msg Message
		if err := json.Unmarshal(message, &msg); err == nil {
			switch msg.Type {
			case "ping":
				response := Message{Type: "pong", Data: nil}
				if data, err := json.Marshal(response); err == nil {
					select {
					case c.send <- data:
					default:
						return
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
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
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
			w.Write(message)

			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
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
