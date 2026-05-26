package astutil

import (
	"go/ast"
	"go/token"
	"regexp"
)

// CountPatterns counts occurrences of regex patterns in a Go file's comments.
// Returns a map of pattern name to count.
func CountPatterns(fset *token.FileSet, f *ast.File, patterns map[string]*regexp.Regexp) map[string]int {
	counts := make(map[string]int)
	for name := range patterns {
		counts[name] = 0
	}

	for _, group := range f.Comments {
		for _, comment := range group.List {
			text := comment.Text
			for name, re := range patterns {
				if re.MatchString(text) {
					counts[name]++
				}
			}
		}
	}

	return counts
}

// Common patterns for stub/TODO detection.
var (
	ReTODO  = regexp.MustCompile(`(?i)\bTODO\b`)
	ReFIXME = regexp.MustCompile(`(?i)\bFIXME\b`)
	ReHACK  = regexp.MustCompile(`(?i)\bHACK\b|待辦|修正`)
	ReStub  = regexp.MustCompile(`(?i)\bstub\b|not implemented|unimplemented`)
)
