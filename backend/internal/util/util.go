package util

import (
    "crypto/rand"
    "fmt"
    "log"
    "net/http"
    "strings"
    "time"
)

func Logging(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next.ServeHTTP(w, r)
        dur := time.Since(start)
        log.Printf("%s %s %s %s", r.RemoteAddr, r.Method, r.URL.Path, dur)
    })
}

func NewRoomID(n int) string {
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

func BaseURL(r *http.Request) string {
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

func ClientIdentity(r *http.Request, handle string) string {
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

