package main

import (
	"log"
	"net/http"
	"time"

	"github.com/ToxicSozo/GoDraw/internal/httpserver"
	"github.com/ToxicSozo/GoDraw/internal/store"
)

func main() {
	st := store.New()
	handler := httpserver.New(st)

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("starting reviewer service on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server stopped: %v", err)
	}
}
