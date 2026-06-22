package main

import (
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
)

// staticHandler returns an http.Handler that serves static assets from the given fs.FS.
// It applies Cache-Control headers (immutable for hashed assets, no-cache for others)
// and implements SPA fallback (serves index.html for paths not matching any file).
func staticHandler(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		cleanPath := filepath.Clean(r.URL.Path)
		// Serve hashed assets with long-lived cache
		if strings.Contains(cleanPath, "-") && (strings.HasSuffix(cleanPath, ".js") || strings.HasSuffix(cleanPath, ".css")) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			w.Header().Del("Pragma")
			w.Header().Del("Expires")
		}
		// SPA fallback: serve index.html for paths that don't match static files
		if _, err := fs.Stat(assets, strings.TrimPrefix(cleanPath, "/")); err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
