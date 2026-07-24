//go:build embed_dist

package admin_web

import "embed"

// DistFS embeds the built frontend assets.
//
//go:embed all:dist
var DistFS embed.FS
