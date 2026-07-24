//go:build !embed_dist

package client_web

import "embed"

// DistFS is a zero-value embed.FS — no files available.
// Real frontend assets are embedded when built with -tags=embed_dist.
var DistFS embed.FS

// ValidFieldsJSON is nil — real value embedded with -tags=embed_dist.
var ValidFieldsJSON []byte
