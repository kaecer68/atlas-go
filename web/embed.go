package web

import "embed"

// DistFS embeds the esbuild output directory (web/dist).
// Use fs.Sub(DistFS, "dist") to strip the "dist" prefix for serving.
//
//go:embed all:dist
var DistFS embed.FS
