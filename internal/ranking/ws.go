package ranking

import (
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type WSHub struct {
	mu       sync.RWMutex
	rooms    map[string]map[*websocket.Conn]bool
	upgrader websocket.Upgrader
}

func NewWSHub() *WSHub {
	return &WSHub{
		rooms: make(map[string]map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (h *WSHub) Register(slug string, conn *websocket.Conn) {
	h.mu.Lock()
	if h.rooms[slug] == nil {
		h.rooms[slug] = make(map[*websocket.Conn]bool)
	}
	h.rooms[slug][conn] = true
	h.mu.Unlock()
}

func (h *WSHub) Unregister(slug string, conn *websocket.Conn) {
	h.mu.Lock()
	delete(h.rooms[slug], conn)
	if len(h.rooms[slug]) == 0 {
		delete(h.rooms, slug)
	}
	h.mu.Unlock()
	conn.Close()
}

func (h *WSHub) Broadcast(slug string, data []byte, exclude *websocket.Conn) {
	h.mu.RLock()
	conns := make([]*websocket.Conn, 0, len(h.rooms[slug]))
	for c := range h.rooms[slug] {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	for _, conn := range conns {
		if conn == exclude {
			continue
		}
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[Ranking WS] write error: %v", err)
			h.Unregister(slug, conn)
		}
	}
}

func (h *WSHub) ServeWS(w http.ResponseWriter, r *http.Request, slug string) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Ranking WS] upgrade error: %v", err)
		return
	}

	h.Register(slug, conn)
	log.Printf("[Ranking WS] client connected to %s", slug)

	defer h.Unregister(slug, conn)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		h.Broadcast(slug, msg, conn)
	}
}
