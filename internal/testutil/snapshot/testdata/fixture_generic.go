// Package fixture_generic exercises CaptureAPI's generic AST coverage:
// TypeSpec with *ast.StructType wrapping a generic parameter list,
// FuncDecl with type parameters.
package fixture_generic

type Box[T any] struct {
	Value T
}

func Map[T, U any](in []T) []U { return nil }

type Pair[K comparable, V any] struct {
	Key K
	Val V
}

func Zero[T any]() T { var z T; return z }
