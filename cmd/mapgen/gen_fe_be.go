package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/cmd/mapgen/astutil"
	"github.com/kaecer68/atlas-go/cmd/mapgen/jsutil"
	"github.com/kaecer68/atlas-go/cmd/mapgen/maps"
)

// generateFrontendBackend generates .omo/maps/frontend-backend.md.
func generateFrontendBackend() error {
	repoRoot, err := astutil.FindRepoRoot(".")
	if err != nil {
		return fmt.Errorf("find repo root: %w", err)
	}

	jsDirs := []string{
		filepath.Join(repoRoot, "admin_web", "static", "js"),
		filepath.Join(repoRoot, "client_web", "static", "js"),
		filepath.Join(repoRoot, "shared_web", "static", "js"),
	}
	internalDir := filepath.Join(repoRoot, "internal")
	cmdDir := filepath.Join(repoRoot, "cmd")

	// 1. Scan frontend JS files for API endpoint references.
	var feEndpoints []jsutil.FrontendEndpoint
	for _, jsDir := range jsDirs {
		eps, err := jsutil.ScanJSFiles(jsDir)
		if err != nil {
			return fmt.Errorf("scan JS files in %s: %w", jsDir, err)
		}
		feEndpoints = append(feEndpoints, eps...)
	}

	// 2. Extract backend routes from Go source.
	dirs := []string{internalDir, cmdDir}
	allRoutes := astutil.ExtractRoutes(dirs)

	// 3. Group frontend endpoints by page.
	type pageInfo struct {
		Name     string
		RelFile  string
		LOC      int
		APICalls []string
	}
	pageMap := make(map[string]*pageInfo)
	var pageNames []string
	for _, ep := range feEndpoints {
		pn := ep.PageName
		if _, ok := pageMap[pn]; !ok {
			pageMap[pn] = &pageInfo{
				Name:    pn,
				RelFile: ep.RelFile,
				LOC:     ep.LOC,
			}
			pageNames = append(pageNames, pn)
		}
		pageMap[pn].APICalls = append(pageMap[pn].APICalls, ep.URL)
	}
	// Also collect pages that have no API calls (just based on file existence).
	sort.Strings(pageNames)

	// Deduplicate API calls per page.
	for _, pg := range pageMap {
		pg.APICalls = dedupSorted(pg.APICalls)
	}

	// 4. Cross-reference: for each backend route, find which frontend pages call it.
	type matchedRoute struct {
		Route  maps.RouteInfo
		Pages  []string
		Status string // "matched", "orphan"
	}
	var matched []matchedRoute
	var orphans []matchedRoute
	routeUsed := make(map[string]bool)

	allFEURLs := make(map[string]bool)
	for _, ep := range feEndpoints {
		allFEURLs[ep.URL] = true
	}

	for _, r := range allRoutes {
		pages := findMatchingPages(r, feEndpoints)
		if len(pages) > 0 {
			routeUsed[r.Pattern] = true
			matched = append(matched, matchedRoute{
				Route:  r,
				Pages:  dedupSorted(pages),
				Status: "matched",
			})
		} else {
			orphans = append(orphans, matchedRoute{
				Route:  r,
				Pages:  nil,
				Status: "orphan",
			})
		}
	}

	// Sort matched by group and pattern.
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].Route.Group != matched[j].Route.Group {
			return matched[i].Route.Group < matched[j].Route.Group
		}
		return matched[i].Route.Pattern < matched[j].Route.Pattern
	})

	// Sort orphans by group and pattern.
	sort.Slice(orphans, func(i, j int) bool {
		if orphans[i].Route.Group != orphans[j].Route.Group {
			return orphans[i].Route.Group < orphans[j].Route.Group
		}
		return orphans[i].Route.Pattern < orphans[j].Route.Pattern
	})

	// 5. Find broken links: frontend URLs that don't match any backend route.
	type brokenLink struct {
		URL  string
		File string
		Line int
	}
	var broken []brokenLink
	for _, ep := range feEndpoints {
		if isBrokenLink(ep.URL, allRoutes) {
			broken = append(broken, brokenLink{
				URL:  ep.URL,
				File: ep.RelFile,
				Line: ep.Line,
			})
		}
	}
	// Deduplicate broken links.
	brokenSeen := make(map[string]bool)
	var brokenDedup []brokenLink
	for _, b := range broken {
		key := b.URL + "|" + b.File
		if brokenSeen[key] {
			continue
		}
		brokenSeen[key] = true
		brokenDedup = append(brokenDedup, b)
	}
	broken = brokenDedup
	sort.Slice(broken, func(i, j int) bool {
		if broken[i].URL != broken[j].URL {
			return broken[i].URL < broken[j].URL
		}
		return broken[i].File < broken[j].File
	})

	// 6. Build the markdown report.
	var sb strings.Builder

	uniquePageCount := len(pageMap)
	feCallCount := len(feEndpoints)
	beRouteCount := len(allRoutes)

	sb.WriteString("# Frontend-Backend Mapping\n")
	fmt.Fprintf(&sb, "> Generated: %s | Frontend pages: %d | API calls: %d | Backend routes: %d\n\n",
		time.Now().UTC().Format("2006-01-02 15:04 MST"),
		uniquePageCount, feCallCount, beRouteCount)

	// --- Frontend Pages section ---
	sb.WriteString("## Frontend Pages\n\n")
	sb.WriteString("| Page | File | LOC | API Calls |\n")
	sb.WriteString("|------|------|-----|----------|\n")
	for _, pn := range pageNames {
		pg := pageMap[pn]
		if pg == nil {
			continue
		}
		calls := listToInline(pg.APICalls, 3)
		fmt.Fprintf(&sb, "| %s | %s | %d | %s |\n",
			pg.Name, pg.RelFile, pg.LOC, calls)
	}
	sb.WriteString("\n")

	// --- API → Frontend Matrix ---
	sb.WriteString("## API → Frontend Matrix\n\n")
	sb.WriteString("| API Route | Handler | Frontend Page(s) | Status |\n")
	sb.WriteString("|-----------|---------|-----------------|--------|\n")
	for _, mr := range matched {
		pages := strings.Join(mr.Pages, ", ")
		fmt.Fprintf(&sb, "| %s | %s | %s | ✅ matched |\n",
			mr.Route.Pattern, mr.Route.HandlerName, pages)
	}
	sb.WriteString("\n")

	// --- Orphan APIs ---
	sb.WriteString("## Orphan APIs (no frontend consumer)\n\n")
	if len(orphans) == 0 {
		sb.WriteString("(none found)\n\n")
	} else {
		sb.WriteString("| Route | Handler | Group |\n")
		sb.WriteString("|-------|---------|-------|\n")
		for _, o := range orphans {
			fmt.Fprintf(&sb, "| %s | %s | %s |\n",
				o.Route.Pattern, o.Route.HandlerName, o.Route.Group)
		}
		sb.WriteString("\n")
	}

	// --- Broken Links ---
	sb.WriteString("## Broken Links (frontend calls non-existent API)\n\n")
	if len(broken) == 0 {
		sb.WriteString("(none found)\n\n")
	} else {
		sb.WriteString("| URL Called | Frontend File | Line |\n")
		sb.WriteString("|-----------|--------------|------|\n")
		for _, b := range broken {
			fmt.Fprintf(&sb, "| %s | %s | %d |\n",
				b.URL, b.File, b.Line)
		}
		sb.WriteString("\n")
	}

	content := sb.String()
	if err := maps.WriteMap("frontend-backend.md", content); err != nil {
		return fmt.Errorf("write frontend-backend map: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Matching logic
// ---------------------------------------------------------------------------

// findMatchingPages finds frontend pages that call a given backend route.
func findMatchingPages(route maps.RouteInfo, feEndpoints []jsutil.FrontendEndpoint) []string {
	seen := make(map[string]bool)
	var pages []string

	for _, ep := range feEndpoints {
		if matchURL(ep.URL, route.Pattern) {
			if !seen[ep.PageName] {
				seen[ep.PageName] = true
				pages = append(pages, ep.PageName)
			}
		}
	}
	return pages
}

// isBrokenLink checks whether a frontend URL does not match any backend route.
func isBrokenLink(feURL string, routes []maps.RouteInfo) bool {
	for _, r := range routes {
		if matchURL(feURL, r.Pattern) {
			return false
		}
	}
	return true
}

// matchURL checks if a frontend URL matches a backend route pattern.
func matchURL(feURL, bePattern string) bool {
	fe := stripQueryParams(feURL)
	be := stripMethodPrefix(bePattern)

	// Skip matching root route against non-root paths
	if be == "" || be == "/" {
		return fe == "" || fe == "/"
	}

	// 1. Exact match after stripping method prefix.
	if fe == be {
		return true
	}

	// 2. Template literal with "..." placeholder: normalize both and compare.
	if strings.Contains(fe, "...") {
		normBE := normalizeGoParams(be)
		if fe == normBE {
			return true
		}
		// Also try: if BE is a prefix of FE (without the ... marker)
		fePrefix := strings.TrimSuffix(fe, "...")
		if strings.HasPrefix(be, fePrefix) {
			return true
		}
		if strings.HasPrefix(fePrefix, be) {
			return true
		}
	}

	// 3. Path-segment based matching.
	feSegs := splitPath(fe)
	beSegs := splitPath(be)

	// Skip: empty segments means something went wrong (like root route).
	if len(feSegs) == 0 || len(beSegs) == 0 {
		return false
	}

	// Try matching: frontend segments against backend segments.
	// If FE is shorter or equal, match all FE segments against BE (BE can have extra params).
	if len(feSegs) <= len(beSegs) {
		match := true
		for i := range feSegs {
			fes := feSegs[i]
			bes := beSegs[i]
			// "..." in FE matches anything.
			if fes == "..." || isGoParam(bes) {
				continue
			}
			if fes != bes {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}

	// If BE is shorter, check if it's a prefix of FE.
	if len(beSegs) <= len(feSegs) {
		match := true
		for i := range beSegs {
			bes := beSegs[i]
			fes := feSegs[i]
			if fes == "..." || isGoParam(bes) {
				continue
			}
			if fes != bes {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}

	return false
}

// stripMethodPrefix removes HTTP method prefixes from a backend route pattern.
// e.g., "GET /api/dashboard/foo" or "POST /api/experiment/bar" → "/api/dashboard/foo"
func stripMethodPrefix(pattern string) string {
	for _, prefix := range []string{"GET ", "POST ", "PUT ", "DELETE ", "PATCH ", "HEAD ", "OPTIONS "} {
		if strings.HasPrefix(pattern, prefix) {
			return pattern[len(prefix):]
		}
	}
	return pattern
}

// normalizeGoParams replaces Go path parameters {param} with "..." for comparison.
func normalizeGoParams(pattern string) string {
	var b strings.Builder
	for {
		start := strings.Index(pattern, "{")
		if start < 0 {
			b.WriteString(pattern)
			break
		}
		end := strings.Index(pattern[start:], "}")
		if end < 0 {
			b.WriteString(pattern)
			break
		}
		end += start
		b.WriteString(pattern[:start])
		b.WriteString("...")
		pattern = pattern[end+1:]
	}
	return b.String()
}

// splitPath splits a URL path into segments.
func splitPath(path string) []string {
	var segs []string
	for s := range strings.SplitSeq(path, "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	return segs
}

// isGoParam returns true if the segment is a Go path parameter like {id}.
func isGoParam(seg string) bool {
	return len(seg) > 2 && seg[0] == '{' && seg[len(seg)-1] == '}'
}

// stripQueryParams removes query string and trailing slashes from a URL.
func stripQueryParams(u string) string {
	if idx := strings.Index(u, "?"); idx >= 0 {
		u = u[:idx]
	}
	u = strings.TrimRight(u, "/")
	return u
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// dedupSorted deduplicates and sorts a string slice.
func dedupSorted(vals []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, v := range vals {
		if seen[v] {
			continue
		}
		seen[v] = true
		result = append(result, v)
	}
	sort.Strings(result)
	return result
}

// listToInline formats a slice of API URLs as an inline comma-separated list.
// If maxShow is exceeded, the rest are truncated with "+N more".
func listToInline(urls []string, maxShow int) string {
	if len(urls) == 0 {
		return "_(none)_"
	}
	if len(urls) <= maxShow {
		return strings.Join(urls, ", ")
	}
	shown := strings.Join(urls[:maxShow], ", ")
	return fmt.Sprintf("%s, +%d more", shown, len(urls)-maxShow)
}
