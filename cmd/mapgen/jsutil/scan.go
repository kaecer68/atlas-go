// Package jsutil provides JavaScript file scanning utilities for the mapgen tool.
package jsutil

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// FrontendEndpoint describes a single API call found in a JavaScript frontend file.
type FrontendEndpoint struct {
	File     string // absolute file path
	RelFile  string // path relative to repo root
	Line     int    // line number where the call was found
	URL      string // extracted URL pattern
	PageName string // short page name derived from filename
	LOC      int    // total lines in the JS file
}

// ScanJSFiles scans all .js files under rootDir for API endpoint references.
func ScanJSFiles(rootDir string) ([]FrontendEndpoint, error) {
	info, err := os.Stat(rootDir)
	if err != nil {
		return nil, fmt.Errorf("stat js dir %s: %w", rootDir, err)
	}

	var files []string
	if info.IsDir() {
		files, err = collectJSFiles(rootDir)
		if err != nil {
			return nil, fmt.Errorf("collect js files: %w", err)
		}
	} else {
		files = []string{rootDir}
	}

	// Determine repo root for relative paths.
	repoRoot := rootDir
	for {
		if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(repoRoot)
		if parent == repoRoot {
			break
		}
		repoRoot = parent
	}

	var results []FrontendEndpoint
	seen := make(map[string]bool)

	for _, path := range files {
		endpoints := scanFile(path, repoRoot)
		for _, ep := range endpoints {
			key := ep.URL + "|" + ep.RelFile
			if seen[key] {
				continue
			}
			seen[key] = true
			results = append(results, ep)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].PageName != results[j].PageName {
			return results[i].PageName < results[j].PageName
		}
		return results[i].URL < results[j].URL
	})

	return results, nil
}

// collectJSFiles recursively collects all .js file paths under dir.
func collectJSFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(info.Name(), ".js") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// scanFile reads a single JS file and extracts all API endpoint references.
func scanFile(path, repoRoot string) []FrontendEndpoint {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	loc := countLinesFromReader(path)
	rel, _ := filepath.Rel(repoRoot, path)
	pageName := derivePageName(rel)

	var endpoints []FrontendEndpoint

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		urls := extractURLsFromLine(line)
		for _, u := range urls {
			endpoints = append(endpoints, FrontendEndpoint{
				File:     path,
				RelFile:  rel,
				Line:     lineNum,
				URL:      u,
				PageName: pageName,
				LOC:      loc,
			})
		}
	}

	return endpoints
}

func countLinesFromReader(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	if len(data) == 0 {
		return 0
	}
	count := 1
	for _, b := range data {
		if b == '\n' {
			count++
		}
	}
	return count
}

func derivePageName(relFile string) string {
	base := filepath.Base(relFile)
	base = strings.TrimSuffix(base, ".js")
	base = strings.ReplaceAll(base, "_", "-")
	return base
}

// ---------------------------------------------------------------------------
// URL extraction patterns (no backreferences — Go regexp doesn't support \1)
// ---------------------------------------------------------------------------

// funcCall: getJSON('X') / silentGetJSON("X") / postJSON(`X`, ...)
var (
	funcCallSQ = regexp.MustCompile(`(?:getJSON|silentGetJSON|safeGetJSON|postJSON)\s*\(\s*'([^']*)'`)
	funcCallDQ = regexp.MustCompile(`(?:getJSON|silentGetJSON|safeGetJSON|postJSON)\s*\(\s*"([^"]*)"`)
	funcCallBT = regexp.MustCompile("(?:getJSON|silentGetJSON|safeGetJSON|postJSON)\\s*\\(\\s*`([^`]*)`")
)

// fetch: fetch('X') / fetch("X") / fetch(`X`)
var (
	fetchSQ = regexp.MustCompile(`fetch\s*\(\s*'([^']*)'`)
	fetchDQ = regexp.MustCompile(`fetch\s*\(\s*"([^"]*)"`)
	fetchBT = regexp.MustCompile("fetch\\s*\\(\\s*`([^`]*)`")
)

// concat: '/api/.../' + expr
var (
	concatSQ = regexp.MustCompile(`'((?:/(?:\w+/)*api/[^']*))'\s*\+`)
	concatDQ = regexp.MustCompile(`"((?:/(?:\w+/)*api/[^"]*))"\s*\+`)
)

// direct string literals with /api/
var (
	directSQ = regexp.MustCompile(`'((?:/(?:\w+/)*api/[^']*))'`)
	directDQ = regexp.MustCompile(`"((?:/(?:\w+/)*api/[^"]*))"`)
)

// template literal: `.../api/...`
var tplLit = regexp.MustCompile("`([^`]*)/api/([^`]*)`")

// jQuery $.get
var (
	jqGetSQ = regexp.MustCompile(`\$\s*\.\s*get\s*\(\s*'([^']*)'`)
	jqGetDQ = regexp.MustCompile(`\$\s*\.\s*get\s*\(\s*"([^"]*)"`)
)

// jQuery $.ajax({url: '...'})
var (
	jqAjaxSQ = regexp.MustCompile(`\$\s*\.\s*ajax\s*\(\s*\{[^}]*url\s*:\s*'([^']*)'`)
	jqAjaxDQ = regexp.MustCompile(`\$\s*\.\s*ajax\s*\(\s*\{[^}]*url\s*:\s*"([^"]*)"`)
)

// extractURLsFromLine applies all patterns to a single line.
func extractURLsFromLine(line string) []string {
	seen := make(map[string]bool)
	var urls []string

	add := func(u string) {
		u = normalizeURL(u)
		if u == "" || !strings.Contains(u, "/api/") {
			return
		}
		if seen[u] {
			return
		}
		seen[u] = true
		urls = append(urls, u)
	}

	// 1. getJSON / silentGetJSON / safeGetJSON / postJSON
	for _, m := range funcCallSQ.FindAllStringSubmatch(line, -1) {
		if len(m) >= 2 {
			add(m[1])
		}
	}
	for _, m := range funcCallDQ.FindAllStringSubmatch(line, -1) {
		if len(m) >= 2 {
			add(m[1])
		}
	}
	for _, m := range funcCallBT.FindAllStringSubmatch(line, -1) {
		if len(m) >= 2 {
			add(m[1])
		}
	}

	// 2. fetch() calls
	for _, m := range fetchSQ.FindAllStringSubmatch(line, -1) {
		if len(m) >= 2 && strings.Contains(m[1], "/api/") {
			add(m[1])
		}
	}
	for _, m := range fetchDQ.FindAllStringSubmatch(line, -1) {
		if len(m) >= 2 && strings.Contains(m[1], "/api/") {
			add(m[1])
		}
	}
	for _, m := range fetchBT.FindAllStringSubmatch(line, -1) {
		if len(m) >= 2 && strings.Contains(m[1], "/api/") {
			add(m[1])
		}
	}

	// 3. Template literals with /api/
	for _, m := range tplLit.FindAllStringSubmatch(line, -1) {
		if len(m) >= 3 {
			prefix := m[1]
			if idx := strings.LastIndex(prefix, "/"); idx >= 0 {
				prefix = prefix[:idx+1]
			}
			u := prefix + "..."
			if strings.Contains(u, "/api/") {
				add(u)
			}
		}
	}

	// 4. String concatenation
	for _, m := range concatSQ.FindAllStringSubmatch(line, -1) {
		if len(m) >= 2 {
			add(m[1] + "...")
		}
	}
	for _, m := range concatDQ.FindAllStringSubmatch(line, -1) {
		if len(m) >= 2 {
			add(m[1] + "...")
		}
	}

	// 5. Direct string literals containing /api/
	for _, m := range directSQ.FindAllStringSubmatch(line, -1) {
		if len(m) >= 2 {
			add(m[1])
		}
	}
	for _, m := range directDQ.FindAllStringSubmatch(line, -1) {
		if len(m) >= 2 {
			add(m[1])
		}
	}

	// 6. jQuery $.get
	for _, m := range jqGetSQ.FindAllStringSubmatch(line, -1) {
		if len(m) >= 2 {
			add(m[1])
		}
	}
	for _, m := range jqGetDQ.FindAllStringSubmatch(line, -1) {
		if len(m) >= 2 {
			add(m[1])
		}
	}

	// 7. jQuery $.ajax with url
	for _, m := range jqAjaxSQ.FindAllStringSubmatch(line, -1) {
		if len(m) >= 2 {
			add(m[1])
		}
	}
	for _, m := range jqAjaxDQ.FindAllStringSubmatch(line, -1) {
		if len(m) >= 2 {
			add(m[1])
		}
	}

	return urls
}

// normalizeURL cleans up an extracted URL.
func normalizeURL(u string) string {
	u = strings.TrimSpace(u)
	u = strings.TrimRight(u, `'"`+"`")

	// Strip query parameters for path-matching.
	if idx := strings.Index(u, "?"); idx >= 0 {
		u = u[:idx]
	}

	// Handle template literal expressions: truncate before ${
	if idx := strings.Index(u, "${"); idx >= 0 {
		if lastSlash := strings.LastIndex(u[:idx], "/"); lastSlash >= 0 {
			u = u[:lastSlash+1] + "..."
		} else {
			u = u[:idx] + "..."
		}
	}

	if !strings.Contains(u, "/api/") {
		return ""
	}
	return u
}
