package handlers

import (
	"net/http"

	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type WebSocketHandler struct {
	hub *stream.Hub
}

func NewWebSocketHandler(hub *stream.Hub) *WebSocketHandler {
	return &WebSocketHandler{
		hub: hub,
	}
}

func (h *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.hub.ServeWS(w, r)
}
