package app

import (
    "log"
    "net/http"
    "strings"

    "github.com/gorilla/websocket"
    "slideflow/internal/hub"
)

// GET /ws/:roomId
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    roomID := strings.TrimPrefix(r.URL.Path, "/ws/")
    s.mu.Lock()
    rm, ok := s.rooms[roomID]
    s.mu.Unlock()
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

    client := hub.NewClient(rm.Hub, conn)
    rm.Hub.RegisterClient(client)
    client.Start()
}
