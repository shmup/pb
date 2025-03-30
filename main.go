// simple in-memory snippet store with CRUD operations exposed over HTTP
// - POST to create a new snippet
// - GET to retrieve an existing snippet by ID
// - PUT to update an existing snippet by ID
// - DELETE to remove an existing snippet by ID
package main

import (
	"context"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
)

type store struct {
	sync.Mutex
	snippets map[string]string
	counter  int
}

func constructURL(r *http.Request, id string) string {
	scheme := "http"

	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	return fmt.Sprintf("%s://%s/%s", scheme, r.Host, id)
}

func authenticateUser(r *http.Request) (string, string, bool) {
	username, password, ok := r.BasicAuth()
	if !ok {
		return "", "", false
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Error getting home directory: %v", err)
		return "", "", false
	}

	netrcPath := filepath.Join(homeDir, ".netrc")
	netrcData, err := os.ReadFile(netrcPath)
	if err != nil {
		log.Printf("Error reading .netrc file: %v", err)
		return "", "", false
	}

	// Simple parser for .netrc format
	lines := strings.Split(string(netrcData), "\n")
	currentMachine := ""
	netrcUser := ""
	netrcPass := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		for i := 0; i < len(fields); i++ {
			if fields[i] == "machine" && i+1 < len(fields) {
				currentMachine = fields[i+1]
				i++
			} else if fields[i] == "login" && i+1 < len(fields) && currentMachine == r.Host {
				netrcUser = fields[i+1]
				i++
			} else if fields[i] == "password" && i+1 < len(fields) && currentMachine == r.Host {
				netrcPass = fields[i+1]
				i++
			}
		}
	}

	// Validate credentials against .netrc
	if username == netrcUser && password == netrcPass {
		return username, password, true
	}

	return "", "", false
}

func main() {
	ps := newStore()
	mux := http.NewServeMux()

	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// try to authenticate
		username, password, authenticated := authenticateUser(r)
		if !authenticated {
			username = "" // ensure it's empty if auth failed
			password = ""
		}

		path := r.URL.Path[1:]

		// check if this is a syntax highlighting request
		parts := strings.SplitN(path, "/", 2)
		if len(parts) == 2 {
			id := parts[0]
			language := parts[1]

			if content, ok := ps.getSnippet(id); ok {
				// serve with syntax highlighting
				serveWithHighlighting(w, content, language)
				log.Printf("Fetched %s with %s highlighting", id, language)
				return
			}
			http.NotFound(w, r)
			return
		}

		// original CRUD logic continues here
		id := path

		switch r.Method {
		case http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}
			id := ps.createSnippet(string(body), username, password)
			url := constructURL(r, id)
			log.Printf("Created: %s by %s", url, username)
			w.Header().Set("Location", url)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, url)

		case http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}
			if ps.updateSnippet(id, string(body), username, password) {
				url := constructURL(r, id)
				fmt.Fprint(w, url)
				log.Printf("Updated %s by %s", id, username)
			} else {
				if authenticated {
					http.Error(w, "Not authorized or snippet not found", http.StatusForbidden)
				} else {
					http.NotFound(w, r)
				}
			}

		case http.MethodGet:
			if content, ok := ps.getSnippet(id); ok {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				fmt.Fprint(w, content)
				log.Printf("Fetched %s", id)
			} else {
				http.NotFound(w, r)
			}

		case http.MethodDelete:
			if ps.deleteSnippet(id, username, password) {
				url := constructURL(r, id)
				fmt.Fprint(w, url)
				log.Printf("Deleted %s by %s", id, username)
			} else {
				if authenticated {
					http.Error(w, "Not authorized or snippet not found", http.StatusForbidden)
				} else {
					w.WriteHeader(http.StatusUnauthorized)
					fmt.Fprint(w, "Authentication required for deletion")
				}
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

func serveWithHighlighting(w http.ResponseWriter, content, language string) {
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <link rel="stylesheet" href="/static/tomorrow-night-bright.min.css">
    <script src="/static/highlight.min.js"></script>
    <style>
        body { margin: 0; padding: 0; background-color: #000; color: #fff; }
        pre { margin: 0; padding: 0; }
        ::selection {
          background-color: white;
          color: black;
        }
        @font-face {
            font-family: 'Source Code Pro';
            font-style: normal;
            font-weight: 400;
            src: url('/static/source-code-pro-v23-latin-regular.woff2') format('woff2');
        }
        code { font-family: 'Source Code Pro', monospace; }
    </style>
</head>
<body>
    <pre><code class="language-%s">%s</code></pre>
    <script>hljs.highlightAll();</script>
</body>
</html>`, html.EscapeString(language), html.EscapeString(content))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, html)
}
