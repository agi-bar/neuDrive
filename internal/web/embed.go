package web

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"strings"
)

//go:embed dist/*
var distFS embed.FS

// Handler returns an http.Handler that serves the embedded frontend.
// For SPA routing, it falls back to index.html for non-file paths.
func Handler() http.Handler {
	dist, _ := fs.Sub(distFS, "dist")
	fileServer := http.FileServer(http.FS(dist))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean path
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// Try to open the file in the embedded FS
		f, err := dist.(fs.ReadFileFS).ReadFile(path)
		if err != nil {
			// File not found — serve index.html for SPA client-side routing
			indexFile, err := fs.ReadFile(dist, "index.html")
			if err != nil {
				http.Error(w, "frontend not built", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(indexFile)
			return
		}
		_ = f

		// File exists — let the standard file server handle it
		// (it sets correct Content-Type, caching headers, etc.)
		fileServer.ServeHTTP(w, r)
	})
}

// DevProxy returns true if the AGENTHUB_DEV environment variable is set,
// indicating the frontend dev server should be used instead of embedded assets.
func DevProxy() bool {
	return os.Getenv("AGENTHUB_DEV") != ""
}
