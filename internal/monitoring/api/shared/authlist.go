package shared

import (
	"net/http"
	"strings"
)

// AuthFreeExactPaths lists paths that bypass API-key authentication for
// read-only methods (GET/HEAD/OPTIONS). These must be kept in sync with
// AuthFreePrefixPaths: any top-level route that has sub-resources should
// usually have both an exact entry (for the bare path) and a prefix entry
// (for nested paths).
//
// This is the single source of truth used by cmd/atlas/main.go isPublicPath
// and by the AuthMiddleware below. Mutating methods (POST/PUT/DELETE/PATCH)
// are never auth-free regardless of path.
var AuthFreeExactPaths = map[string]bool{
	"/":                             true,
	"/health":                       true,
	"/ready":                        true,
	"/metrics":                      true,
	"/admin":                        true,
	"/admin_web":                    true,
	"/client":                       true,
	"/api/health":                   true,
	"/api/health/":                  true,
	"/api/llm/health":               true,
	"/api/v1/alerts":                true,
	"/api/health/aggregate":         true,
	"/api/version":                  true,
	"/api/alerts":                   true,
	"/api/stock":                    true,
	"/api/recommendations":          true,
	"/api/tasks":                    true,
	"/api/field-contract":           true,
	"/api/control/audit-log":        true,
	"/api/control/active-overrides": true,
	"/api/config":                   true,
	"/api/experiment/history":       true,
	"/api/experiment/diff":          true,
	"/api/strategies":               true,
	// PR-3d (#1824 pattern): the admin 熱圖 period-matrix page needs the
	// read-only matrix endpoint without an API key. GET-only via the
	// public-read path rules below.
	"/api/strategy/period-matrix": true,
	"/api/parameters":             true,
	"/api/dashboard":              true,
	"/api/dashboard/sessions":     true,
	"/api/taiwan":                 true,
	"/api/narrative":              true,
	"/api/macro":                  true,
	"/api/synergy":                true,
	"/api/cross-market":           true,
	"/api/detector":               true,
	"/api/capital-flow":           true,
	"/api/events":                 true,
	"/api/industry":               true,
	"/api/reports":                true,
	"/api/strategy-ranker":        true,
	"/api/backtest":               true,
	"/api/auth":                   true,
	"/api/user":                   true,
	"/api/risk":                   true,
	"/api/janus":                  true,
	"/api/regime":                 true,
	"/api/report":                 true,
	"/api/geopolitical":           true,
	"/api/scheduler":              true,
	"/api/traces":                 true,
}

// AuthFreePrefixPaths lists path prefixes that bypass API-key authentication
// for read-only methods. See AuthFreeExactPaths.
var AuthFreePrefixPaths = []string{
	"/admin/",
	"/admin_web/",
	"/client/",
	"/api/routes",
	"/api/dashboard/",
	"/api/dashboard/sessions/",
	"/api/taiwan/",
	"/api/narrative/",
	"/api/macro/",
	"/api/alerts/",
	"/api/synergy/",
	"/api/cross-market/",
	"/api/detector/",
	"/api/capital-flow/",
	"/api/events/",
	"/api/industry/",
	"/api/stock/",
	"/api/recommendations/",
	"/api/reports/",
	"/api/strategy-ranker/",
	"/api/parameters/",
	"/api/backtest/",
	"/api/auth/",
	"/api/user/",
	"/api/strategies/",
	"/api/risk/",
	"/api/janus/",
	"/api/regime/",
	"/api/report/",
	"/api/geopolitical/",
	"/api/scheduler/",
	"/api/tasks/",
	"/api/traces/",
	"/api/llm/",
	"/api/llm_annotator/",
	"/api/metrics/",
	"/api/admin/live/",
	"/api/prism/",
}

// IsPublicReadPath reports whether p is in the public-read whitelist
// (exact or prefix match). It does not consider HTTP method.
// Hashed frontend JS chunks served at root level are also public-read so
// the client-side router can load them without credentials.
func IsPublicReadPath(p string) bool {
	if AuthFreeExactPaths[p] {
		return true
	}
	for _, prefix := range AuthFreePrefixPaths {
		if len(p) >= len(prefix) && p[:len(prefix)] == prefix {
			return true
		}
	}
	// Hashed frontend chunks (e.g. stock-quote-*.js) are served at root level.
	// Standard API routes never end in .js, so this is safe.
	if strings.HasSuffix(p, ".js") {
		return true
	}
	return false
}

// AuthFreeWritePrefixes lists path prefixes that bypass API-key
// authentication for all methods. This is reserved for user authentication
// endpoints (/api/auth/* and /api/user/*) which have their own JWT/cookie
// auth layer; requiring the Atlas API key here would prevent users from
// logging in or registering.
var AuthFreeWritePrefixes = []string{
	"/api/auth/",
	"/api/user/",
}

// IsPublicPath reports whether a request with the given method and path
// bypasses API-key authentication. GET/HEAD/OPTIONS on the public-read
// whitelist are public; mutating methods (POST/PUT/DELETE/PATCH) require
// authentication unless they are under a user-auth prefix.
func IsPublicPath(method, p string) bool {
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return IsPublicReadPath(p)
	}
	for _, prefix := range AuthFreeWritePrefixes {
		if len(p) >= len(prefix) && p[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
