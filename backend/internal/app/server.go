package app

import (
    "encoding/base64"
    "encoding/json"
    "io"
    "net/http"
    "os"
    "strings"
    "sync"
    "time"

    qrcode "github.com/skip2/go-qrcode"

    "slideflow/internal/hub"
    "slideflow/internal/util"
)

type room struct {
    ID       string
    Hub      *hub.Hub
    Paused   bool
    SlowMode time.Duration
}

type Server struct {
    mux   *http.ServeMux
    rooms map[string]*room
    mu    sync.Mutex
    // rate[roomID][identity] = lastPostTime
    rate map[string]map[string]time.Time
    // NG words (lowercased)
    ngWords []string
}

func NewServer() *Server {
    s := &Server{
        mux:   http.NewServeMux(),
        rooms: make(map[string]*room),
        rate:  make(map[string]map[string]time.Time),
    }

    // Routes
    s.mux.HandleFunc("/health", s.handleHealth)
    s.mux.HandleFunc("/rooms", s.handleCreateRoom)
    s.mux.HandleFunc("/ws/", s.handleWS)
    s.mux.HandleFunc("/overlay/", s.handleOverlay)
    s.mux.HandleFunc("/post/", s.handlePostForm)
    s.mux.HandleFunc("/admin/", s.handleAdmin)
    s.mux.HandleFunc("/rooms/", s.handleRoomSubroutes)
    s.mux.HandleFunc("/present", s.handlePresent)

    // Load NG words from env (comma-separated), fallback to a small default
    if v := strings.TrimSpace(os.Getenv("NG_WORDS")); v != "" {
        parts := strings.Split(v, ",")
        for _, p := range parts {
            p = strings.ToLower(strings.TrimSpace(p))
            if p != "" {
                s.ngWords = append(s.ngWords, p)
            }
        }
    } else {
        s.ngWords = []string{"死ね", "fuck", "shit"}
    }
    return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/plain; charset=utf-8")
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write([]byte("ok"))
}

// POST /rooms -> { roomId, overlayUrl, postUrl, qrPngBase64 }
func (s *Server) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }

    id := util.NewRoomID(10)
    h := hub.NewHub()
    go h.Run()
    s.mu.Lock()
    s.rooms[id] = &room{ID: id, Hub: h}
    s.mu.Unlock()

    base := util.BaseURL(r)
    overlayURL := base + "/overlay/" + id
    postURL := base + "/post/" + id

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

// --- Room subroutes ---
func (s *Server) handleRoomSubroutes(w http.ResponseWriter, r *http.Request) {
    // Expect /rooms/{id}/...
    rest := strings.TrimPrefix(r.URL.Path, "/rooms/")
    parts := strings.Split(rest, "/")
    if len(parts) < 2 {
        http.NotFound(w, r)
        return
    }
    roomID, tail := parts[0], parts[1]
    s.mu.Lock()
    rm, ok := s.rooms[roomID]
    s.mu.Unlock()
    if !ok {
        http.Error(w, "room not found", http.StatusNotFound)
        return
    }

    switch tail {
    case "messages":
        if r.Method != http.MethodPost {
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }
        s.handlePostMessage(w, r, rm, roomID)
        return
    case "pause":
        if r.Method != http.MethodPost { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
        s.mu.Lock(); rm.Paused = true; s.mu.Unlock()
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        json.NewEncoder(w).Encode(map[string]any{"ok": true, "paused": true})
        return
    case "resume":
        if r.Method != http.MethodPost { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
        s.mu.Lock(); rm.Paused = false; s.mu.Unlock()
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        json.NewEncoder(w).Encode(map[string]any{"ok": true, "paused": false})
        return
    case "clear":
        if r.Method != http.MethodPost { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
        rm.Hub.Broadcast([]byte(`{"type":"clear"}`))
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        json.NewEncoder(w).Encode(map[string]any{"ok": true})
        return
    case "slowmode":
        if r.Method != http.MethodPost { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
        var body struct{ Ms int `json:"ms"` }
        if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil { http.Error(w, "invalid json", http.StatusBadRequest); return }
        if body.Ms < 0 { body.Ms = 0 }
        s.mu.Lock(); rm.SlowMode = time.Duration(body.Ms) * time.Millisecond; s.mu.Unlock()
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        json.NewEncoder(w).Encode(map[string]any{"ok": true, "slowModeMs": body.Ms})
        return
    default:
        http.NotFound(w, r)
        return
    }
}

type postMessageReq struct {
    Text   string `json:"text"`
    Handle string `json:"handle"`
}

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request, rm *room, roomID string) {
    body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20)) // 4MB cap
    if err != nil {
        http.Error(w, "invalid body", http.StatusBadRequest)
        return
    }
    var req postMessageReq
    if err := json.Unmarshal(body, &req); err != nil {
        http.Error(w, "invalid json", http.StatusBadRequest)
        return
    }
    req.Text = strings.TrimSpace(req.Text)
    req.Handle = strings.TrimSpace(req.Handle)
    if req.Text == "" {
        http.Error(w, "text required", http.StatusBadRequest)
        return
    }
    if len([]rune(req.Text)) > 200 {
        http.Error(w, "text too long", http.StatusBadRequest)
        return
    }
    if len([]rune(req.Handle)) > 32 {
        http.Error(w, "handle too long", http.StatusBadRequest)
        return
    }

    // NG word check
    lower := strings.ToLower(req.Text)
    for _, ng := range s.ngWords {
        if ng == "" { continue }
        if strings.Contains(lower, strings.ToLower(ng)) {
            http.Error(w, "ng word detected", http.StatusForbidden)
            return
        }
    }

    // Check paused and apply slow mode as cooldown
    s.mu.Lock()
    paused := rm.Paused
    slow := rm.SlowMode
    s.mu.Unlock()
    if paused {
        http.Error(w, "paused", http.StatusLocked)
        return
    }

    cooldown := 2 * time.Second
    if slow > 0 {
        cooldown = slow
    }
    identity := util.ClientIdentity(r, req.Handle)
    now := time.Now()
    s.mu.Lock()
    if s.rate[roomID] == nil {
        s.rate[roomID] = make(map[string]time.Time)
    }
    last := s.rate[roomID][identity]
    if now.Sub(last) < cooldown {
        s.mu.Unlock()
        http.Error(w, "rate limited", http.StatusTooManyRequests)
        return
    }
    s.rate[roomID][identity] = now
    s.mu.Unlock()

    // Broadcast payload
    payload := map[string]string{
        "type":   "chat",
        "text":   req.Text,
        "handle": req.Handle,
    }
    b, _ := json.Marshal(payload)
    rm.Hub.Broadcast(b)

    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.WriteHeader(http.StatusAccepted)
    json.NewEncoder(w).Encode(map[string]any{"ok": true})
}
