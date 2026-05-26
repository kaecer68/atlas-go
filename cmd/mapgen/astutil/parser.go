// Package astutil provides shared AST analysis utilities for the mapgen tool.
package astutil

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// GetAllGoFiles recursively walks dir and returns all .go file paths (excluding _test.go).
func GetAllGoFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip hidden directories and vendor
			if strings.HasPrefix(info.Name(), ".") || info.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// ParseGoFile parses a Go source file into an AST. Returns nil on error (logs to stderr).
func ParseGoFile(fset *token.FileSet, path string) *ast.File {
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse %s: %v\n", path, err)
		return nil
	}
	return f
}

// CountLines returns the number of lines in a file.
func CountLines(path string) int {
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

// PackagePath returns the Go import path for a directory relative to the repo root.
// Assumes module is github.com/kaecer68/atlas-go.
func PackagePath(repoRoot, pkgDir string) string {
	rel, err := filepath.Rel(repoRoot, pkgDir)
	if err != nil {
		return pkgDir
	}
	return "github.com/kaecer68/atlas-go/" + rel
}

// FindRepoRoot walks up from startDir to find the repo root (contains go.mod).
func FindRepoRoot(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("abs start dir: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from %s", startDir)
		}
		dir = parent
	}
}
