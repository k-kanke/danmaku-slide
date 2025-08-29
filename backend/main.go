package main

import (
    "crypto/rand"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "strings"
    "time"

    qrcode "github.com/skip2/go-qrcode"
    "github.com/gorilla/websocket"
)

type room struct {
    ID string
}

type server struct {
    mux   *http.ServeMux
    rooms map[string]*room
}

func newServer() *server {
    s := &server{
        mux:   http.NewServeMux(),
        rooms: make(map[string]*room),
    }

    // Routes
    s.mux.HandleFunc("/health", s.handleHealth)
    s.mux.HandleFunc("/rooms", s.handleCreateRoom)
    s.mux.HandleFunc("/ws/", s.handleWS)

    return s
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/plain; charset=utf-8")
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write([]byte("ok"))
}

// POST /rooms -> { roomId, overlayUrl, postUrl, qrPngBase64 }
func (s *server) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }

    id := newRoomID(10)
    h := newHub()
    go h.run()
    s.rooms[id] = &room{ID: id, Hub: h}

    base := baseURL(r)
    overlayURL := fmt.Sprintf("%s/overlay/%s", base, id)
    postURL := fmt.Sprintf("%s/post/%s", base, id)

    // Generate QR for post URL
    png, err := qrcode.Encode(postURL, qrcode.Medium, 256)
    if err != nil {
        http.Error(w, "failed to generate QR", http.StatusInternalServerError)
        return
    }
    qrB64 := base64.StdEncoding.EncodeToString(png)

    resp := map[string]string{
        "roomId":      id,
        "overlayUrl":  overlayURL,
        "postUrl":     postURL,
        "qrPngBase64": qrB64,
    }

    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    json.NewEncoder(w).Encode(resp)
}

// GET /ws/:roomId
func (s *server) handleWS(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    roomID := strings.TrimPrefix(r.URL.Path, "/ws/")
    rm, ok := s.rooms[roomID]
    if !ok {
        http.Error(w, "room not found", http.StatusNotFound)
        return
    }

    upgrader := websocket.Upgrader{
        ReadBufferSize:  1024,
        WriteBufferSize: 1024,
        CheckOrigin: func(r *http.Request) bool { return true },
    }

    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("ws upgrade error: %v", err)
        return
    }

    client := &Client{hub: rm.Hub, conn: conn, send: make(chan []byte, 256)}
    rm.Hub.register <- client

    go client.writePump()
    go client.readPump()
}

func main() {
    s := newServer()

    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    srv := &http.Server{
        Addr:         ":" + port,
        Handler:      logging(s.mux),
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 10 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    log.Printf("SlideFlow backend listening on :%s", port)
    log.Fatal(srv.ListenAndServe())
}

func logging(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next.ServeHTTP(w, r)
        dur := time.Since(start)
        log.Printf("%s %s %s %s", r.RemoteAddr, r.Method, r.URL.Path, dur)
    })
}

func newRoomID(n int) string {
    const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
    b := make([]byte, n)
    if _, err := rand.Read(b); err != nil {
        // fallback to time-based
        return fmt.Sprintf("%d", time.Now().UnixNano())
    }
    for i := range b {
        b[i] = letters[int(b[i])%len(letters)]
    }
    return string(b)
}

func baseURL(r *http.Request) string {
    // Prefer headers when behind proxy
    scheme := r.Header.Get("X-Forwarded-Proto")
    if scheme == "" {
        if r.TLS != nil {
            scheme = "https"
        } else {
            scheme = "http"
        }
    }
    host := r.Header.Get("X-Forwarded-Host")
    if host == "" {
        host = r.Host
    }
    // Trim possible trailing slash
    return strings.TrimRight(fmt.Sprintf("%s://%s", scheme, host), "/")
}

// --- WebSocket Hub ---

type Hub struct {
    clients    map[*Client]bool
    broadcast  chan []byte
    register   chan *Client
    unregister chan *Client
}

func newHub() *Hub {
    return &Hub{
        clients:    make(map[*Client]bool),
        broadcast:  make(chan []byte, 256),
        register:   make(chan *Client),
        unregister: make(chan *Client),
    }
}

func (h *Hub) run() {
    for {
        select {
        case c := <-h.register:
            h.clients[c] = true
        case c := <-h.unregister:
            if _, ok := h.clients[c]; ok {
                delete(h.clients, c)
                close(c.send)
            }
        case msg := <-h.broadcast:
            for c := range h.clients {
                select {
                case c.send <- msg:
                default:
                    close(c.send)
                    delete(h.clients, c)
                }
            }
        }
    }
}

type Client struct {
    hub  *Hub
    conn *websocket.Conn
    send chan []byte
}

func (c *Client) readPump() {
    defer func() {
        c.hub.unregister <- c
        c.conn.Close()
    }()
    c.conn.SetReadLimit(1024)
    c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
    c.conn.SetPongHandler(func(string) error {
        c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
        return nil
    })
    for {
        _, message, err := c.conn.ReadMessage()
        if err != nil {
            break
        }
        // Relay any client message to the hub.
        c.hub.broadcast <- message
    }
}

func (c *Client) writePump() {
    ticker := time.NewTicker(50 * time.Second)
    defer func() {
        ticker.Stop()
        c.conn.Close()
    }()
    for {
        select {
        case message, ok := <-c.send:
            c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            if !ok {
                // Hub closed the channel.
                c.conn.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }
            if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
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
