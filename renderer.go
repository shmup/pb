package main

import (
	"fmt"
	"html"
	"net/http"
)

func serveUserListing(w http.ResponseWriter, _ *http.Request, username string, _ *Store) {
	// In a real implementation, you'd query the store for pastes by this user
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if username == "" {
		// Display the last 100 anonymous pastes
		fmt.Fprintf(w, "<html><body><h1>Last 100 Anonymous Pastes</h1><p>Feature not yet implemented</p></body></html>")
	} else {
		// Display pastes from this user
		fmt.Fprintf(w, "<html><body><h1>Pastes from %s</h1><p>Feature not yet implemented</p></body></html>",
			html.EscapeString(username))
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
