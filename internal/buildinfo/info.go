// Package buildinfo exposes runtime metadata about the running binary.
//
// The package-level variables Version, Commit, and BuildTime are intended to
// be overridden at link time via `-ldflags '-X ...'` (see the project's
// Makefile, Dockerfile, and .github/workflows/release.yml). When the binary is
// built without those overrides — for example during a plain `go test` run —
// every field falls back to the sentinel string "unknown" so downstream code
// (system health endpoint, MCP tools, dashboards) can still distinguish
// "no build info" from "empty string".
//
// Current() reads the package variables at call time (not at init) so test
// code can mutate them via direct assignment and observe the change without a
// rebuild. Production binaries that need the values embedded should rely on
// the linker overrides; this package never shells out to git or reads
// environment variables at runtime.
package buildinfo

import "runtime"

var (
	Version   = "unknown"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// Info is the JSON-serialisable snapshot of build metadata returned by
// Current(). Field order matches the brief and the spec
// (docs/specs/capital-flow-seven-dimension-spec.md §11.4).
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}

// Current returns a snapshot of the package-level linker-injected vars plus
// the running Go runtime version. Safe for concurrent use; the underlying
// string vars are read-only after link-time injection in production.
func Current() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
		GoVersion: runtime.Version(),
	}
}
