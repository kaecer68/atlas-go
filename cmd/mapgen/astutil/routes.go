package astutil

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kaecer68/atlas-go/cmd/mapgen/maps"
)

// ExtractRoutes scans all Go files under dirs for HTTP route registrations.
// It detects:
//   - mux.HandleFunc(pattern, handler)
//   - mux.Handle(pattern, handler)
//   - Register*Routes(mux, ...) calls
func ExtractRoutes(dirs []string) []maps.RouteInfo {
	fset := token.NewFileSet()
	var routes []maps.RouteInfo
	routeSet := make(map[string]bool) // dedup by "pattern+handler" key

	for _, dir := range dirs {
		files, err := GetAllGoFiles(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "walk %s: %v\n", dir, err)
			continue
		}
		for _, path := range files {
			f := ParseGoFile(fset, path)
			if f == nil {
				continue
			}
			routes = append(routes, extractRoutesFromFile(fset, f, path, routeSet)...)
		}
	}

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Group != routes[j].Group {
			return routes[i].Group < routes[j].Group
		}
		return routes[i].Pattern < routes[j].Pattern
	})
	return routes
}

func extractRoutesFromFile(fset *token.FileSet, f *ast.File, path string, routeSet map[string]bool) []maps.RouteInfo {
	var routes []maps.RouteInfo

	// Collect register functions in this file (for pattern: Register*Routes)
	registerFuncs := make(map[string]bool)
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if strings.HasPrefix(fn.Name.Name, "Register") && strings.HasSuffix(fn.Name.Name, "Routes") {
			registerFuncs[fn.Name.Name] = true
		}
	}

	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		methodName := sel.Sel.Name
		if methodName != "HandleFunc" && methodName != "Handle" {
			return true
		}

		// Must have at least 2 args: (pattern, handler)
		if len(call.Args) < 2 {
			return true
		}

		pattern := extractStringLiteral(call.Args[0])
		if pattern == "" {
			return true
		}

		handlerName := extractHandlerName(call.Args[1])
		pos := fset.Position(call.Pos())

		key := pattern + "|" + handlerName
		if routeSet[key] {
			return true
		}
		routeSet[key] = true

		group := classifyRoute(pattern)
		rel, _ := filepath.Rel(filepath.Dir(path)+"/../..", path)

		routes = append(routes, maps.RouteInfo{
			Method:      "", // Go 1.22+ ServeMux embeds method in pattern
			Pattern:     pattern,
			HandlerName: handlerName,
			File:        path,
			RelFile:     rel,
			Line:        pos.Line,
			Group:       group,
			IsStub:      false, // checked in a separate pass
		})
		return true
	})

	return routes
}

// MarkStubHandlers checks each route's handler function to determine if it's a stub.
func MarkStubHandlers(routes []maps.RouteInfo, dirs []string) {
	fset := token.NewFileSet()
	handlerFuncs := make(map[string]*ast.FuncDecl) // handlerName -> AST node

	for _, dir := range dirs {
		files, _ := GetAllGoFiles(dir)
		for _, path := range files {
			f := ParseGoFile(fset, path)
			if f == nil {
				continue
			}
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				handlerFuncs[fn.Name.Name] = fn
			}
		}
	}

	for i, route := range routes {
		fn, ok := handlerFuncs[route.HandlerName]
		if !ok {
			continue
		}
		routes[i].IsStub = isStubFunc(fn)
	}
}

func isStubFunc(fn *ast.FuncDecl) bool {
	if fn.Body == nil {
		return true
	}
	stmtCount := len(fn.Body.List)
	if stmtCount == 0 {
		return true
	}
	// One statement: return nil, nil or return nil
	if stmtCount == 1 {
		ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
		if !ok {
			return false
		}
		return isNilReturn(ret)
	}
	return false
}

func isNilReturn(ret *ast.ReturnStmt) bool {
	if len(ret.Results) == 0 {
		return true
	}
	for _, r := range ret.Results {
		if id, ok := r.(*ast.Ident); ok && id.Name == "nil" {
			continue
		}
		return false
	}
	return true
}

func extractStringLiteral(expr ast.Expr) string {
	switch lit := expr.(type) {
	case *ast.BasicLit:
		if lit.Kind == token.STRING {
			return strings.Trim(lit.Value, "\"")
		}
	case *ast.Ident:
		return lit.Name // could be a constant reference
	}
	return ""
}

func extractHandlerName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		// x.HandlerName
		if x, ok := e.X.(*ast.Ident); ok {
			return x.Name + "." + e.Sel.Name
		}
		return e.Sel.Name
	case *ast.FuncLit:
		return "anonymous"
	}
	return fmt.Sprintf("%T", expr)
}

func classifyRoute(pattern string) string {
	switch {
	case strings.Contains(pattern, "/api/industry"):
		return "industry"
	case strings.Contains(pattern, "/api/narrative"):
		return "narrative"
	case strings.Contains(pattern, "/api/control") || strings.Contains(pattern, "/api/agents"):
		return "control"
	case strings.Contains(pattern, "/api/dashboard/live") || strings.Contains(pattern, "/api/dashboard/pnl") || strings.Contains(pattern, "/api/dashboard/risk"):
		return "live"
	case strings.Contains(pattern, "/api/experiment"):
		return "experiment"
	case strings.Contains(pattern, "/api/backtest"):
		return "backtest"
	case strings.Contains(pattern, "/api/dashboard/performance"):
		return "performance"
	case strings.Contains(pattern, "/api/dashboard"):
		return "dashboard"
	case strings.Contains(pattern, "/api/scheduler") || strings.Contains(pattern, "/api/tasks"):
		return "system"
	case strings.Contains(pattern, "/health"):
		return "system"
	default:
		return "other"
	}
}
