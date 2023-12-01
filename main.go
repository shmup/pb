package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
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

	id := fmt.Sprintf("%d", s.counter)
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
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}
			id := s.createSnippet(string(body))
			fmt.Fprintf(w, "http://localhost:8080/%s\n", id)

		case http.MethodPut:
			id := r.URL.Path[1:]
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}
			if s.updateSnippet(id, string(body)) {
				fmt.Fprintf(w, "Updated snippet at http://localhost:8080/%s\n", id)
			} else {
				http.NotFound(w, r)
			}

		case http.MethodGet:
			id := r.URL.Path[1:]
			if content, ok := s.getSnippet(id); ok {
				fmt.Fprint(w, content)
			} else {
				http.NotFound(w, r)
			}

		case http.MethodDelete:
			id := r.URL.Path[1:]
			if s.deleteSnippet(id) {
				fmt.Fprintf(w, "Deleted snippet %s\n", id)
			} else {
				http.NotFound(w, r)
			}

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
