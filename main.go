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
)

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
		return "", "", false
	}

	netrcData, err := os.ReadFile(filepath.Join(homeDir, ".netrc"))
	if err != nil {
		return "", "", false
	}

	var currentMachine, netrcUser, netrcPass string
	for _, line := range strings.Split(string(netrcData), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}

		for i := 0; i < len(fields)-1; i++ {
			switch fields[i] {
			case "machine":
				currentMachine = fields[i+1]
			case "login":
				if currentMachine == r.Host {
					netrcUser = fields[i+1]
				}
			case "password":
				if currentMachine == r.Host {
					netrcPass = fields[i+1]
				}
			}
		}
	}

	return username, password, username == netrcUser && password == netrcPass
}

func main() {
	ps := newStore()
	mux := http.NewServeMux()

	// Serve static files
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Main handler for all routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		username, password, _ := authenticateUser(r)
		path := r.URL.Path[1:]

		// Handle syntax highlighting
		if parts := strings.SplitN(path, "/", 2); len(parts) == 2 {
			if content, ok := ps.getSnippet(parts[0]); ok {
				serveWithHighlighting(w, content, parts[1])
				log.Printf("Fetched %s with %s highlighting", parts[0], parts[1])
				return
			}
			http.NotFound(w, r)
			return
		}

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
				fmt.Fprint(w, constructURL(r, id))
				log.Printf("Updated %s by %s", id, username)
			} else if username != "" {
				http.Error(w, "Not authorized or snippet not found", http.StatusForbidden)
			} else {
				http.NotFound(w, r)
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
				fmt.Fprint(w, constructURL(r, id))
				log.Printf("Deleted %s by %s", id, username)
			} else if username != "" {
				http.Error(w, "Not authorized or snippet not found", http.StatusForbidden)
			} else {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, "Authentication required for deletion")
			}

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Start the server
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

	// Handle graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	log.Println("Shutting down...")
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Fatalf("Shutdown error: %v", err)
	}
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
        ::selection { background-color: white; color: black; }
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
