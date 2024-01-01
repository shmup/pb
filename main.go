// Simple HTTP CRUD API for managing text snippets.
//
// This program creates an in-memory snippet store with CRUD operations exposed over HTTP.
// Supported methods:
// - POST to create a new snippet
// - GET to retrieve an existing snippet by ID
// - PUT to update an existing snippet by ID
// - DELETE to remove an existing snippet by ID
//
// The server starts on port 8080 and responds to the above HTTP methods at the root path.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
)

type store struct {
	sync.Mutex
	snippets map[string]string
	counter  int
}

func newStore() *store {
	return &store{
		snippets: make(map[string]string),
		counter:  1,
	}
}

func main() {
	ps := newPermanentStore()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Path[1:]

		switch r.Method {
		case http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}
			id := ps.createSnippet(string(body))
			url := "http://localhost:8080/" + id
			log.Printf("Created: %s", url)
			w.Header().Set("Location", url)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, url)

		case http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}
			if ps.updateSnippet(id, string(body)) {
				url := "http://localhost:8080/" + id
				fmt.Fprintln(w, url)
				log.Printf("Updated %s", id)
			} else {
				http.NotFound(w, r)
			}

		case http.MethodGet:
			if content, ok := ps.getSnippet(id); ok {
				w.Header().Set("Content-Type", "text/plain")
				fmt.Fprint(w, content)
				log.Printf("Fetched %s", id)
			} else {
				http.NotFound(w, r)
			}

		case http.MethodDelete:
			if ps.deleteSnippet(id) {
				log.Printf("Deleted %s", id)
			} else {
				http.NotFound(w, r)
			}

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	log.Println("Server is running on http://localhost:8080")

	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	log.Println("Shutting down server...")
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Fatalf("Server Shutdown Failed:%+v", err)
	}
	log.Println("Server exited properly")
}
