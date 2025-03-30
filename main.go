// simple in-memory snippet store with CRUD operations exposed over HTTP
// - POST to create a new snippet
// - GET to retrieve an existing snippet by ID
// - PUT to update an existing snippet by ID
// - DELETE to remove an existing snippet by ID
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
)

func main() {
	ps := newStore()
	readCounter := newReadCounter()

	mux := setupRoutes(ps, readCounter)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	go func() {
		log.Println("Server is running on http://localhost:8080")
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// handle graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	log.Println("Shutting down...")
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Fatalf("Shutdown error: %v", err)
	}
}

func setupRoutes(ps *Store, rc *ReadCounter) *http.ServeMux {
	mux := http.NewServeMux()

	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	mux.HandleFunc("/", createMainHandler(ps, rc))

	return mux
}
