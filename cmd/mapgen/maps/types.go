// Package maps provides shared types and output utilities for map generation.
package maps

import "time"

// ModuleInfo describes a single module in the system.
type ModuleInfo struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	GoFiles     int      `json:"go_files"`
	TestFiles   int      `json:"test_files"`
	LOC         int      `json:"loc"`
	Role        string   `json:"role"`
	HasAgentsMD bool     `json:"has_agents_md"`
	Imports     []string `json:"imports,omitempty"`
}

// RouteInfo describes a single HTTP API route.
type RouteInfo struct {
	Method      string `json:"method,omitempty"`
	Pattern     string `json:"pattern"`
	HandlerName string `json:"handler"`
	File        string `json:"file"`
	RelFile     string `json:"rel_file"`
	Line        int    `json:"line"`
	Group       string `json:"group"`
	IsStub      bool   `json:"is_stub"`
}

// CompletenessReport captures module health metrics.
type CompletenessReport struct {
	Module          string  `json:"module"`
	GoFiles         int     `json:"go_files"`
	TotalFiles      int     `json:"total_files"`
	TotalFuncs      int     `json:"total_funcs"`
	StubCount       int     `json:"stub_count"`
	TODOCount       int     `json:"todo_count"`
	FIXMECount      int     `json:"fixme_count"`
	TestCoverage    float64 `json:"test_coverage"`
	CompletenessPct int     `json:"completeness_pct"`
	PreviousPct     int     `json:"previous_pct,omitempty"`
	Notes           string  `json:"notes,omitempty"`
}

// FrontendPage describes a frontend page and the API endpoints it calls.
type FrontendPage struct {
	Name     string   `json:"name"`
	FileName string   `json:"file_name"`
	File     string   `json:"file"`
	LOC      int      `json:"loc"`
	APICalls []string `json:"api_calls"`
}

// FEBEMapping is a row in the frontend-backend cross-reference.
type FEBEMapping struct {
	BackendRoute  string   `json:"backend_route"`
	HandlerName   string   `json:"handler"`
	FrontendPages []string `json:"frontend_pages"`
	Status        string   `json:"status"` // "matched", "orphan", "broken"
}

// MapMeta is metadata header written to each map file.
type MapMeta struct {
	Title     string    `json:"title"`
	Generated time.Time `json:"generated"`
	Source    string    `json:"source"`
}
