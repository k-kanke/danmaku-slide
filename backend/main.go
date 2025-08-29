package main

import (
    "log"
    "net/http"
    "os"
    "time"
)

func main() {
    mux := http.NewServeMux()

    // Health check endpoint
    mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/plain; charset=utf-8")
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("ok"))
    })

    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    srv := &http.Server{
        Addr:         ":" + port,
        Handler:      logging(mux),
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

