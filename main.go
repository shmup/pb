package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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

func (s *store) createSnippet(content string) string {
	s.Lock()
	defer s.Unlock()

	id := strconv.Itoa(s.counter)
	s.snippets[id] = content
	s.counter++
	return id
}

func (s *store) getSnippet(id string) (string, bool) {
	s.Lock()
	defer s.Unlock()

	content, ok := s.snippets[id]
	return content, ok
}

func (s *store) updateSnippet(id, content string) bool {
	s.Lock()
	defer s.Unlock()

	if _, exists := s.snippets[id]; !exists {
		return false
	}
	s.snippets[id] = content
	return true
}

func (s *store) deleteSnippet(id string) bool {
	s.Lock()
	defer s.Unlock()

	if _, exists := s.snippets[id]; !exists {
		return false
	}
	delete(s.snippets, id)
	return true
}

func main() {
	s := newStore()
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
			id := s.createSnippet(string(body))
			url := "http://localhost:8080/" + id
			w.Header().Set("Location", url)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, url)

		case http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}
			if s.updateSnippet(id, string(body)) {
				url := "http://localhost:8080/" + id
				fmt.Fprintln(w, url)
			} else {
				http.NotFound(w, r)
			}

		case http.MethodGet:
			if content, ok := s.getSnippet(id); ok {
				fmt.Fprint(w, content)
			} else {
				http.NotFound(w, r)
			}

		case http.MethodDelete:
			if s.deleteSnippet(id) {
				fmt.Fprintf(w, "Deleted %s", id)
			} else {
				http.NotFound(w, r)
			}

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

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
