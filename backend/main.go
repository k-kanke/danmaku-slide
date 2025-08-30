package main

import (
    "log"
    "net/http"
    "os"
    "time"

    "slideflow/internal/app"
    "slideflow/internal/util"
)

func main() {
    s := app.NewServer()

    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    srv := &http.Server{
        Addr:         ":" + port,
        Handler:      util.Logging(s.Handler()),
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 10 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    log.Printf("SlideFlow backend listening on :%s", port)
    log.Fatal(srv.ListenAndServe())
}

