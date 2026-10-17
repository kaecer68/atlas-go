package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/cmd/mapgen/maps"
)

// routesGeneratedAt matches the report header's wall-clock line, normalized out
// of the determinism comparison so the assertion is about ordering, not timing.
var routesGeneratedAt = regexp.MustCompile(`(?m)^> Generated:.*$`)

// TestBuildRoutesMarkdown_DeterministicNonCanonicalGroupOrder pins the contract
// of api-routes.md: both the summary table and the per-group detail sections
// list the canonical groupOrder first and then the remaining groups in
// ascending name order.
//
// Before the fix the two non-canonical loops ranged over the groups map, so the
// same input produced a different report on every run (FU-20261016-33).
func TestBuildRoutesMarkdown_DeterministicNonCanonicalGroupOrder(t *testing.T) {
	routes := []maps.RouteInfo{
		{Pattern: "/api/status", HandlerName: "statusHandler", Group: "system", RelFile: "internal/apigateway/x.go", Line: 10},
	}
	groups := groupRoutes(routes)
	// Eight groups outside groupOrder — enough that Go's randomized map
	// iteration order is visible across two consecutive calls.
	for i := 0; i < 8; i++ {
		name := string(rune('a' + i))
		groups[name] = []maps.RouteInfo{{
			Pattern:     "/api/" + name,
			HandlerName: name + "Handler",
			Group:       name,
			RelFile:     "internal/apigateway/" + name + ".go",
			Line:        20 + i,
		}}
	}

	normalize := func(s string) string {
		return routesGeneratedAt.ReplaceAllString(s, "> Generated: <normalized>")
	}
	first := normalize(buildRoutesMarkdown(routes, groups))
	second := normalize(buildRoutesMarkdown(routes, groups))
	if first != second {
		t.Fatalf("api-routes.md is not byte-identical across two calls with the same input\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}

	// Contract: canonical groups first, then the rest in ascending name order.
	// Detail headings are capitalised by capitalizeGroup; summary rows are not.
	wantHeadings := []string{"System", "A", "B", "C", "D", "E", "F", "G", "H"}
	if got := groupHeadingOrder(first); !equalStrings(got, wantHeadings) {
		t.Fatalf("detail section order = %v, want %v", got, wantHeadings)
	}
	wantRows := []string{"system", "a", "b", "c", "d", "e", "f", "g", "h"}
	if got := summaryGroupOrder(first); !equalStrings(got, wantRows) {
		t.Fatalf("summary table group order = %v, want %v", got, wantRows)
	}
}

var (
	routesGroupHeading   = regexp.MustCompile(`(?m)^## (\S+) Routes \(`)
	routesSummarySection = regexp.MustCompile(`(?s)## Summary by Group\n.*?\n\n`)
	routesSummaryRow     = regexp.MustCompile(`(?m)^\| (\S+) \| \d+ \| \d+ \| \d+ \|$`)
)

// groupHeadingOrder extracts the group names in the order their detail sections
// appear in the report.
func groupHeadingOrder(markdown string) []string {
	matches := routesGroupHeading.FindAllStringSubmatch(markdown, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

// summaryGroupOrder extracts the group names in the order their rows appear in
// the "Summary by Group" table.
func summaryGroupOrder(markdown string) []string {
	section := routesSummarySection.FindString(markdown)
	out := []string(nil)
	for _, m := range routesSummaryRow.FindAllStringSubmatch(section, -1) {
		out = append(out, m[1])
	}
	return out
}

func equalStrings(got, want []string) bool {
	return strings.Join(got, ",") == strings.Join(want, ",")
}
