package web

import "embed"

// DistFS embeds the esbuild output directory (web/dist).
// Use fs.Sub(DistFS, "dist") to strip the "dist" prefix for serving.
//
// Build requirement: web/dist/ must contain at least one file for the
// //go:embed directive to succeed. CI runs `npm run build` (esbuild)
// before `go build`, so dist/ is always populated in CI. On a fresh
// clone, the embedded web/dist/.gitkeep placeholder keeps the directory
// present in git so `go build ./...` works without first running
// `npm run build` (the resulting binary will serve a placeholder dist
// until you run the frontend build).
//
//go:embed all:dist
var DistFS embed.FS
