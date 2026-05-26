package astutil

import (
	"go/ast"
	"strings"
)

// ExtractInternalImports returns internal import paths from a parsed Go file.
// Internal imports are those starting with the module prefix.
// Returns the short package names (last segment of the import path).
func ExtractInternalImports(f *ast.File, modulePrefix string) []string {
	var imports []string
	seen := make(map[string]bool)

	for _, imp := range f.Imports {
		if imp.Path == nil {
			continue
		}
		path := strings.Trim(imp.Path.Value, "\"")
		if !strings.HasPrefix(path, modulePrefix) {
			continue
		}
		// Extract the short package name (last segment)
		parts := strings.Split(path, "/")
		pkg := parts[len(parts)-1]
		if imp.Name != nil {
			pkg = imp.Name.Name // use explicit alias if present
		}
		if !seen[pkg] {
			seen[pkg] = true
			imports = append(imports, pkg)
		}
	}
	return imports
}
