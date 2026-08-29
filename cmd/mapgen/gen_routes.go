package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/cmd/mapgen/astutil"
	"github.com/kaecer68/atlas-go/cmd/mapgen/maps"
)

// groupOrder defines the canonical display order for route groups
// matching the expected output sections.
var groupOrder = []string{
	"dashboard",
	"industry",
	"narrative",
	"control",
	"live",
	"experiment",
	"backtest",
	"performance",
	"system",
	"other",
}

// generateRoutes generates .omo/maps/api-routes.md.
func generateRoutes() error {
	// Find repo root for accurate relative path calculation.
	repoRoot, err := astutil.FindRepoRoot(".")
	if err != nil {
		return fmt.Errorf("find repo root: %w", err)
	}

	// Scan internal/ and cmd/ directories for route registrations.
	dirs := []string{
		filepath.Join(repoRoot, "internal"),
		filepath.Join(repoRoot, "cmd"),
	}
	routes := astutil.ExtractRoutes(dirs)

	// Detect stub handlers (functions that just return nil).
	astutil.MarkStubHandlers(routes, dirs)

	// Recompute relative paths from the actual repo root
	// (the built-in RelFile assumes a fixed depth, which may be wrong).
	for i := range routes {
		rel, err := filepath.Rel(repoRoot, routes[i].File)
		if err == nil {
			routes[i].RelFile = rel
		}
	}

	// Group routes by category.
	groups := groupRoutes(routes)

	// Build and write the markdown report.
	content := buildRoutesMarkdown(routes, groups)
	return maps.WriteMap("api-routes.md", content)
}

// groupRoutes groups routes by their Group field.
func groupRoutes(routes []maps.RouteInfo) map[string][]maps.RouteInfo {
	groups := make(map[string][]maps.RouteInfo)
	for _, r := range routes {
		groups[r.Group] = append(groups[r.Group], r)
	}
	return groups
}

// buildRoutesMarkdown builds the full markdown report from extracted routes.
func buildRoutesMarkdown(routes []maps.RouteInfo, groups map[string][]maps.RouteInfo) string {
	var sb strings.Builder

	// Count stubs.
	totalStubs := 0
	for _, r := range routes {
		if r.IsStub {
			totalStubs++
		}
	}

	// Header with metadata.
	sb.WriteString("# API Routes Map\n")
	fmt.Fprintf(&sb, "> Generated: %s | Total routes: %d | Stubs: %d\n\n",
		time.Now().UTC().Format("2006-01-02 15:04 UTC"),
		len(routes), totalStubs)

	// Summary table by group.
	sb.WriteString("## Summary by Group\n")
	sb.WriteString("| Group | Count | Stubs | Active |\n")
	sb.WriteString("|-------|-------|-------|--------|\n")

	// Display groups in canonical order; skip empty groups.
	for _, groupName := range groupOrder {
		gr, ok := groups[groupName]
		if !ok || len(gr) == 0 {
			continue
		}
		stubCount := countStubs(gr)
		active := len(gr) - stubCount
		fmt.Fprintf(&sb, "| %s | %d | %d | %d |\n",
			groupName, len(gr), stubCount, active)
	}
	// Include any groups not in the canonical order.
	for groupName, gr := range groups {
		if containsString(groupOrder, groupName) || len(gr) == 0 {
			continue
		}
		stubCount := countStubs(gr)
		active := len(gr) - stubCount
		fmt.Fprintf(&sb, "| %s | %d | %d | %d |\n",
			groupName, len(gr), stubCount, active)
	}
	sb.WriteString("\n")

	// Per-group detail sections.
	for _, groupName := range groupOrder {
		gr, ok := groups[groupName]
		if !ok || len(gr) == 0 {
			continue
		}
		writeGroupSection(&sb, groupName, gr)
	}
	// Non-canonical groups.
	for groupName, gr := range groups {
		if containsString(groupOrder, groupName) || len(gr) == 0 {
			continue
		}
		writeGroupSection(&sb, groupName, gr)
	}

	return sb.String()
}

// writeGroupSection writes a section header and route table for one group.
func writeGroupSection(sb *strings.Builder, groupName string, gr []maps.RouteInfo) {
	// Routes within a group are already sorted by Pattern from ExtractRoutes,
	// but re-sort to be safe against group merging.
	sort.Slice(gr, func(i, j int) bool {
		return gr[i].Pattern < gr[j].Pattern
	})

	fmt.Fprintf(sb, "## %s Routes (%d routes)\n\n",
		capitalizeGroup(groupName), len(gr))
	sb.WriteString("| Pattern | Handler | File:Line | Status |\n")
	sb.WriteString("|---------|---------|-----------|--------|\n")

	for _, r := range gr {
		status := "✅ active"
		if r.IsStub {
			status = "⚠️ stub"
		}
		fileLine := fmt.Sprintf("%s:%d", r.RelFile, r.Line)
		fmt.Fprintf(sb, "| %s | %s | %s | %s |\n",
			r.Pattern, r.HandlerName, fileLine, status)
	}
	sb.WriteString("\n")
}

// countStubs returns the number of stub routes in a slice.
func countStubs(routes []maps.RouteInfo) int {
	n := 0
	for _, r := range routes {
		if r.IsStub {
			n++
		}
	}
	return n
}

// capitalizeGroup returns the group name with the first letter uppercased
// for use as a section title (e.g., "dashboard" → "Dashboard").
func capitalizeGroup(g string) string {
	if len(g) == 0 {
		return g
	}
	return strings.ToUpper(g[:1]) + g[1:]
}

// containsString checks whether a string slice contains a target string.
func containsString(slice []string, target string) bool {
	return slices.Contains(slice, target)
}
