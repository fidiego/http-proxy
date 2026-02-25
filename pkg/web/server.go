// Package web provides the HTTP-based inspection UI for http-proxy.
package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/fidiego/http-proxy/pkg/proxy"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     func(_ *http.Request) bool { return true },
}

// Server serves the web inspection UI and REST API.
type Server struct {
	engine  *proxy.Engine
	port    int
	server  *http.Server
	hub     *wsHub
}

// New creates a new web Server for the given engine.
func New(engine *proxy.Engine, port int) *Server {
	s := &Server{
		engine: engine,
		port:   port,
		hub:    newWSHub(),
	}
	return s
}

// Start runs the web server until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	go s.hub.run()

	// Subscribe to flow events and broadcast them to WebSocket clients.
	eventCh := s.engine.Store().Subscribe()
	go func() {
		defer s.engine.Store().Unsubscribe(eventCh)
		for {
			select {
			case evt, ok := <-eventCh:
				if !ok {
					return
				}
				data, err := json.Marshal(evt)
				if err == nil {
					s.hub.broadcast <- data
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: corsMiddleware(mux),
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutCtx)
	}()

	log.Printf("web UI: http://localhost:%d", s.port)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("web server: %w", err)
	}
	return nil
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	h := &handlers{engine: s.engine, hub: s.hub}

	// REST API
	mux.HandleFunc("GET /api/flows", h.listFlows)
	mux.HandleFunc("GET /api/flows/{id}", h.getFlow)
	mux.HandleFunc("POST /api/flows/{id}/replay", h.replayFlow)
	mux.HandleFunc("DELETE /api/flows", h.clearFlows)
	mux.HandleFunc("GET /api/config", h.getConfig)

	// WebSocket
	mux.HandleFunc("GET /ws", s.handleWS)

	// Embedded HTML UI (root)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, indexHTML)
	})
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := &wsClient{hub: s.hub, conn: conn, send: make(chan []byte, 256)}
	s.hub.register <- client
	go client.writePump()
	go client.readPump()
}

// corsMiddleware adds permissive CORS headers (dev-only).
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- WebSocket hub ---

type wsHub struct {
	clients    map[*wsClient]bool
	broadcast  chan []byte
	register   chan *wsClient
	unregister chan *wsClient
	mu         sync.Mutex
}

func newWSHub() *wsHub {
	return &wsHub{
		clients:    make(map[*wsClient]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *wsClient),
		unregister: make(chan *wsClient),
	}
}

func (h *wsHub) run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = true
			h.mu.Unlock()
		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			h.mu.Unlock()
		case msg := <-h.broadcast:
			h.mu.Lock()
			for c := range h.clients {
				select {
				case c.send <- msg:
				default:
					delete(h.clients, c)
					close(c.send)
				}
			}
			h.mu.Unlock()
		}
	}
}

type wsClient struct {
	hub  *wsHub
	conn *websocket.Conn
	send chan []byte
}

func (c *wsClient) writePump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

func (c *wsClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(512)
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}
