//go:build !embed_dist

package admin_web

import "embed"

// DistFS is a zero-value embed.FS — no files available.
// Real frontend assets are embedded when built with -tags=embed_dist.
var DistFS embed.FS
