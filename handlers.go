package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
)

func constructURL(r *http.Request, id string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/%s", scheme, r.Host, id)
}

func createMainHandler(ps *Store, readCounter *ReadCounter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, password, _ := authenticateUser(r)
		path := r.URL.Path[1:]

		// Handle user listings
		if strings.HasPrefix(path, "user/") {
			userParts := strings.SplitN(path, "/", 3)
			if len(userParts) >= 2 {
				serveUserListing(w, r, userParts[1], ps)
				return
			}
		}

		// Handle console highlighting with + syntax
		if strings.Contains(path, "+") {
			handleSyntaxHighlighting(w, r, path, ps, readCounter)
			return
		}

		// Handle regular syntax highlighting
		if parts := strings.SplitN(path, "/", 2); len(parts) == 2 {
			if content, ok := ps.getSnippet(parts[0]); ok {
				serveWithHighlighting(w, content, parts[1])

				// Check if read count should delete this paste
				if readCounter.incrementAndCheck(parts[0]) {
					ps.deleteSnippet(parts[0], "", "")
					log.Printf("Auto-deleted %s after reaching read limit", parts[0])
				}
				return
			}
			http.NotFound(w, r)
			return
		}

		id := path
		switch r.Method {
		case http.MethodPost:
			handlePostRequest(w, r, ps, readCounter, id, username, password)
		case http.MethodPut:
			handlePutRequest(w, r, ps, id, username, password)
		case http.MethodGet:
			handleGetRequest(w, r, ps, readCounter, id)
		case http.MethodDelete:
			handleDeleteRequest(w, r, ps, id, username, password)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleSyntaxHighlighting(w http.ResponseWriter, r *http.Request, path string, ps *Store, rc *ReadCounter) {
	parts := strings.SplitN(path, "+", 2)
	if content, ok := ps.getSnippet(parts[0]); ok {
		lang := "console"
		if len(parts) > 1 && parts[1] != "" {
			lang = parts[1]
		}
		serveWithHighlighting(w, content, lang)

		// Check if read count should delete this paste
		if rc.incrementAndCheck(parts[0]) {
			ps.deleteSnippet(parts[0], "", "")
			log.Printf("Auto-deleted %s after reaching read limit", parts[0])
		}
		return
	}
	http.NotFound(w, r)
}

func handlePostRequest(w http.ResponseWriter, r *http.Request, ps *Store, rc *ReadCounter, id, username, password string) {
	if err := r.ParseMultipartForm(10 << 20); err == nil { // 10 MB max
		// Handle file deletion request
		if rmID := r.FormValue("rm"); rmID != "" {
			if ps.deleteSnippet(rmID, username, password) {
				fmt.Fprint(w, constructURL(r, rmID))
				log.Printf("Deleted %s by %s", rmID, username)
				return
			}
			http.Error(w, "Not authorized or snippet not found", http.StatusForbidden)
			return
		}

		// Handle multipart form data
		var allContent strings.Builder
		var filesProcessed bool

		// Process multiple files
		for i := 1; ; i++ {
			fileKey := fmt.Sprintf("f:%d", i)
			nameKey := fmt.Sprintf("name:%d", i)
			extKey := fmt.Sprintf("ext:%d", i)
			readKey := fmt.Sprintf("read:%d", i)

			// Check for read count
			if readValue := r.FormValue(readKey); readValue != "" {
				if readCount, err := strconv.Atoi(readValue); err == nil && readCount > 0 {
					// Store read count for later application
					log.Printf("Setting read count %d for file %d", readCount, i)
					defer func(count int) {
						if id != "" {
							rc.setMaxReads(id, count)
							log.Printf("Set read count %d for %s", count, id)
						}
					}(readCount)
				}
			}

			// Process file or content
			if processFileOrContent(r, i, fileKey, nameKey, extKey, &allContent, &filesProcessed) {
				continue
			} else {
				break // No more files
			}
		}

		// If we processed multipart data
		if filesProcessed {
			// Check if an ID is provided to replace
			if idValue := r.FormValue("id:1"); idValue != "" && username != "" {
				if ps.updateSnippet(idValue, allContent.String(), username, password) {
					id = idValue
					fmt.Fprint(w, constructURL(r, id))
					log.Printf("Updated %s by %s", id, username)
					return
				}
			}

			// Create new snippet
			id = ps.createSnippet(allContent.String(), username, password)
			url := constructURL(r, id)
			log.Printf("Created: %s by %s", url, username)
			w.Header().Set("Location", url)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, url)
			return
		}
	}

	// Fallback to reading body directly if not multipart
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	id = ps.createSnippet(string(body), username, password)
	url := constructURL(r, id)
	log.Printf("Created: %s by %s", url, username)
	w.Header().Set("Location", url)
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, url)
}

func processFileOrContent(r *http.Request, i int, fileKey, nameKey, extKey string, allContent *strings.Builder, filesProcessed *bool) bool {
	// Get filename and extension
	filename := fmt.Sprintf("File %d", i)
	if name := r.FormValue(nameKey); name != "" {
		filename = name
	}

	ext := ""
	if extValue := r.FormValue(extKey); extValue != "" {
		if !strings.HasPrefix(extValue, ".") {
			ext = "." + extValue
		} else {
			ext = extValue
		}
	}

	// Check for uploaded files
	files := r.MultipartForm.File[fileKey]
	if len(files) > 0 {
		*filesProcessed = true
		file, err := files[0].Open()
		if err != nil {
			return true // Skip this file but continue processing
		}

		fileContent, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			return true // Skip this file but continue processing
		}

		// Use the file's name if no name was specified
		if r.FormValue(nameKey) == "" && files[0].Filename != "" {
			filename = files[0].Filename
		}

		if i > 1 {
			allContent.WriteString("\n\n--- " + filename + ext + " ---\n\n")
		}
		allContent.Write(fileContent)
		return true

	} else if formValue := r.FormValue(fileKey); formValue != "" {
		// Check for form field with content
		*filesProcessed = true

		if i > 1 {
			allContent.WriteString("\n\n--- " + filename + ext + " ---\n\n")
		}
		allContent.WriteString(formValue)
		return true
	}

	return false // No more files
}

func handlePutRequest(w http.ResponseWriter, r *http.Request, ps *Store, id, username, password string) {
	if err := r.ParseMultipartForm(10 << 20); err == nil {
		// Handle multipart form data for PUT
		if fileContent := r.FormValue("f:1"); fileContent != "" {
			if ps.updateSnippet(id, fileContent, username, password) {
				fmt.Fprint(w, constructURL(r, id))
				log.Printf("Updated %s by %s", id, username)
				return
			}
		}

		// Try to get uploaded file
		if files := r.MultipartForm.File["f:1"]; len(files) > 0 {
			file, err := files[0].Open()
			if err == nil {
				fileContent, err := io.ReadAll(file)
				file.Close()
				if err == nil && ps.updateSnippet(id, string(fileContent), username, password) {
					fmt.Fprint(w, constructURL(r, id))
					log.Printf("Updated %s by %s", id, username)
					return
				}
			}
		}
	}

	// Fallback to reading body directly
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
}

func handleGetRequest(w http.ResponseWriter, r *http.Request, ps *Store, rc *ReadCounter, id string) {
	if content, ok := ps.getSnippet(id); ok {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, content)
		log.Printf("Fetched %s", id)

		// Check if read count should delete this paste
		if rc.incrementAndCheck(id) {
			ps.deleteSnippet(id, "", "")
			log.Printf("Auto-deleted %s after reaching read limit", id)
		}
	} else {
		http.NotFound(w, r)
	}
}

func handleDeleteRequest(w http.ResponseWriter, r *http.Request, ps *Store, id, username, password string) {
	if r.ParseMultipartForm(1<<20) == nil {
		if rmID := r.FormValue("rm"); rmID != "" {
			if ps.deleteSnippet(rmID, username, password) {
				fmt.Fprint(w, constructURL(r, rmID))
				log.Printf("Deleted %s by %s", rmID, username)
				return
			}
			http.Error(w, "Not authorized or snippet not found", http.StatusForbidden)
			return
		}
	}

	if ps.deleteSnippet(id, username, password) {
		fmt.Fprint(w, constructURL(r, id))
		log.Printf("Deleted %s by %s", id, username)
	} else if username != "" {
		http.Error(w, "Not authorized or snippet not found", http.StatusForbidden)
	} else {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "Authentication required for deletion")
	}
}
