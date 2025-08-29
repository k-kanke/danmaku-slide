package main

import (
    "crypto/rand"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "strings"
    "time"

    qrcode "github.com/skip2/go-qrcode"
    "github.com/gorilla/websocket"
    "sync"
)

type room struct {
    ID       string
    Hub      *Hub
    Paused   bool
    SlowMode time.Duration
}

type server struct {
    mux   *http.ServeMux
    rooms map[string]*room
    mu    sync.Mutex
    // rate[roomID][identity] = lastPostTime
    rate map[string]map[string]time.Time
    // NG words (lowercased)
    ngWords []string
}

func newServer() *server {
    s := &server{
        mux:   http.NewServeMux(),
        rooms: make(map[string]*room),
        rate:  make(map[string]map[string]time.Time),
    }

    // Routes
    s.mux.HandleFunc("/health", s.handleHealth)
    s.mux.HandleFunc("/rooms", s.handleCreateRoom)
    s.mux.HandleFunc("/ws/", s.handleWS)
    s.mux.HandleFunc("/overlay/", s.handleOverlay)
    s.mux.HandleFunc("/rooms/", s.handleRoomSubroutes)

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
    s.mu.Lock()
    s.rooms[id] = &room{ID: id, Hub: h}
    s.mu.Unlock()

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

    client := &Client{hub: rm.Hub, conn: conn, send: make(chan []byte, 256)}
    rm.Hub.register <- client

    go client.writePump()
    go client.readPump()
}

// GET /overlay/:roomId -> HTML + JS overlay (transparent canvas)
func (s *server) handleOverlay(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    roomID := strings.TrimPrefix(r.URL.Path, "/overlay/")
    s.mu.Lock()
    _, ok := s.rooms[roomID]
    s.mu.Unlock()
    if !ok {
        http.Error(w, "room not found", http.StatusNotFound)
        return
    }
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    // Minimal HTML with transparent background and full-screen canvas
    // JS connects to WS and renders scrolling messages with lane control.
    fmt.Fprintf(w, `<!doctype html>
<html lang="ja">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>SlideFlow Overlay - %s</title>
  <style>
    html, body { margin:0; padding:0; background:transparent; height:100%%; overflow:hidden; }
    canvas { display:block; width:100vw; height:100vh; background:transparent; pointer-events:none; }
  </style>
</head>
<body>
  <canvas id="overlay"></canvas>
  <script>
  (function(){
    const roomId = %q;
    const canvas = document.getElementById('overlay');
    const ctx = canvas.getContext('2d');
    let dpr = window.devicePixelRatio || 1;
    let width = 0, height = 0;
    let lastTime = performance.now();
    const fontSize = 36; // px
    const lineHeight = Math.round(fontSize * 1.2);
    const speed = 160; // px per second
    const maxMessages = 200;

    function resize(){
      dpr = window.devicePixelRatio || 1;
      width = Math.floor(window.innerWidth);
      height = Math.floor(window.innerHeight);
      canvas.width = Math.floor(width * dpr);
      canvas.height = Math.floor(height * dpr);
      canvas.style.width = width + 'px';
      canvas.style.height = height + 'px';
      ctx.setTransform(dpr,0,0,dpr,0,0);
      ctx.font = 'bold ' + fontSize + "px -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Noto Sans JP', 'Hiragino Kaku Gothic ProN', Meiryo, Arial, sans-serif";
      ctx.textBaseline = 'top';
    }
    window.addEventListener('resize', resize);
    resize();

    // Lane management
    function laneCount(){ return Math.max(1, Math.floor(height / lineHeight)); }
    function laneY(l){ return Math.round(l * lineHeight + (lineHeight - fontSize) / 2); }
    const bullets = []; // active bullets
    const inbox = [];   // pending messages

    function rightmostXOfLane(l){
      let maxX = -Infinity;
      for (const b of bullets){ if (b.lane === l) { maxX = Math.max(maxX, b.x + b.w); } }
      return maxX === -Infinity ? -1 : maxX;
    }

    function tryPlace(text, color){
      if (!text) return false;
      const w = Math.ceil(ctx.measureText(text).width);
      const L = laneCount();
      for (let l=0; l<L; l++){
        const rightmost = rightmostXOfLane(l);
        if (rightmost < width - 140){ // small gap to avoid overlap
          bullets.push({text, x: width, y: laneY(l), w, lane:l, speed, color});
          return true;
        }
      }
      return false;
    }

    function draw(){
      const now = performance.now();
      const dt = Math.min(0.05, (now - lastTime) / 1000);
      lastTime = now;

      ctx.clearRect(0,0,width,height);
      // move and draw bullets
      for (let i=bullets.length-1; i>=0; i--){
        const b = bullets[i];
        b.x -= b.speed * dt;
        if (b.x + b.w < 0){ bullets.splice(i,1); continue; }
        ctx.save();
        ctx.shadowColor = 'rgba(0,0,0,0.7)';
        ctx.shadowBlur = 4; ctx.shadowOffsetX = 2; ctx.shadowOffsetY = 2;
        ctx.fillStyle = b.color || '#fff';
        ctx.fillText(b.text, Math.round(b.x), b.y);
        ctx.restore();
      }

      // try to place pending messages
      for (let i=0; i<inbox.length && bullets.length < maxMessages; ){
        if (tryPlace(inbox[i].text, inbox[i].color)){
          inbox.splice(i,1);
        } else {
          i++;
        }
      }

      requestAnimationFrame(draw);
    }
    requestAnimationFrame(draw);

    // WebSocket connection
    const wsProto = (location.protocol === 'https:') ? 'wss' : 'ws';
    const wsUrl = wsProto + '://' + location.host + '/ws/' + roomId;
    let ws;
    function connect(){
      ws = new WebSocket(wsUrl);
      ws.addEventListener('message', (ev)=>{
        try {
          const msg = JSON.parse(ev.data);
          if (msg && msg.type === 'chat'){
            const txt = String(msg.text || '').slice(0, 200);
            const handle = (msg.handle ? String(msg.handle) : '').trim();
            const text = handle ? '【' + handle + '】 ' + txt : txt;
            inbox.push({ text, color: '#ffffff' });
          } else if (msg && msg.type === 'clear'){
            bullets.length = 0; inbox.length = 0;
          }
        } catch(e){
          // non-JSON -> treat as raw text
          const t = String(ev.data || '');
          inbox.push({ text: t, color: '#ffffff' });
        }
      });
      ws.addEventListener('close', ()=> setTimeout(connect, 1000));
      ws.addEventListener('error', ()=> { try{ ws.close(); }catch{} });
    }
    connect();
  })();
  </script>
</body>
</html>`, roomID, roomID)
}

// Handle room subroutes like /rooms/:roomId/messages
func (s *server) handleRoomSubroutes(w http.ResponseWriter, r *http.Request) {
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
    default:
        http.NotFound(w, r)
        return
    }
}

type postMessageReq struct {
    Text   string `json:"text"`
    Handle string `json:"handle"`
}

func (s *server) handlePostMessage(w http.ResponseWriter, r *http.Request, rm *room, roomID string) {
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

    // Rate limit: 1 post per 2s per identity per room
    identity := clientIdentity(r, req.Handle)
    now := time.Now()
    s.mu.Lock()
    if s.rate[roomID] == nil {
        s.rate[roomID] = make(map[string]time.Time)
    }
    last := s.rate[roomID][identity]
    if now.Sub(last) < 2*time.Second {
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
    rm.Hub.broadcast <- b

    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.WriteHeader(http.StatusAccepted)
    json.NewEncoder(w).Encode(map[string]any{"ok": true})
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

func clientIdentity(r *http.Request, handle string) string {
    ip := r.Header.Get("X-Forwarded-For")
    if ip == "" {
        ip = r.RemoteAddr
    } else {
        // XFF may contain multiple IPs, take the first
        if i := strings.Index(ip, ","); i >= 0 {
            ip = ip[:i]
        }
    }
    // strip port if present
    if i := strings.LastIndex(ip, ":"); i > -1 {
        ip = ip[:i]
    }
    handle = strings.ToLower(strings.TrimSpace(handle))
    return ip + "|" + handle
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
