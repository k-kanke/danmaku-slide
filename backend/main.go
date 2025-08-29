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
    s.rooms[id] = &room{ID: id}

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
