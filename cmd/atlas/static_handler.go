package main

import (
	"io/fs"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
)

// hashRe matches esbuild content-hashed asset names: <name>-<8 alphanumerics>
// (e.g. metrics-IBTZZMC5.js, chunk-AEYXGOVL.js). Non-handed shared files
// (component-init.js, bootstrap-utils.js) do not match and stay no-cache.
var hashRe = regexp.MustCompile(`-[0-9A-Za-z]{8}\.(js|css)$`)

// staticHandler returns an http.Handler that serves static assets from the given fs.FS.
// It applies Cache-Control headers (immutable for hashed assets, no-cache for others)
// and implements SPA fallback (serves index.html for paths not matching any file).
//
// The handler expects an fs.FS whose root mirrors the `dist/` subtree (callers
// pass `fs.Sub(embed.DistFS, "dist")`), so the FS root has files at
// `js/main.js`, `index.html`, etc. Request URLs may still arrive with the
// inner `dist/` prefix because http.StripPrefix only strips the outer URL
// mount prefix (e.g. `/client/`), not the inner `dist/`. When the cleaned
// path starts with `dist/`, we look up the stripped path in the FS and, if
// present, rewrite `r.URL.Path` so the embedded fileServer can serve the
// file. Without this, every `/client/dist/...` request would fall through
// to the SPA fallback and return `index.html` instead of the asset.
func staticHandler(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		cleanPath := filepath.Clean(r.URL.Path)
		// Serve hashed assets with long-lived cache. Hash detection must be
		// strict (final dash + 8 alphanumeric chars, esbuild's hash shape):
		// the old "filename contains -" heuristic also matched NON-hashed
		// files like component-init.js / event-listeners.js and pinned them
		// immutable for a year — deployments then served stale JS until a
		// manual hard refresh (observed 2026-09-02 on /admin/metrics).
		if hashRe.MatchString(cleanPath) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			w.Header().Del("Pragma")
			w.Header().Del("Expires")
		}
		// Resolve a `dist/...` URL to the corresponding file in the sub-FS.
		// We try the stripped path first; if it exists, we both use it for
		// the SPA-fallback check and rewrite the request URL so the file
		// server can locate it. Paths that do not start with `dist/` (root,
		// explicit `index.html`, SPA-fallback targets) keep their original
		// lookup semantics.
		lookupPath := strings.TrimPrefix(cleanPath, "/")
		if rest, ok := strings.CutPrefix(lookupPath, "dist/"); ok {
			if _, err := fs.Stat(assets, rest); err == nil {
				r.URL.Path = "/" + rest
				lookupPath = rest
			}
		}
		// SPA fallback: serve index.html for paths that don't match static files
		if _, err := fs.Stat(assets, lookupPath); err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
