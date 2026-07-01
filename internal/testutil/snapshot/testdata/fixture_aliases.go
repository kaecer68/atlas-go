// Package fixture_aliases exercises CaptureAPI's type-distinction
// coverage: defined types, type aliases, named slice/map/chan types,
// embedded struct fields.
package fixture_aliases

type MyInt = int

type MyString string

type MySlice []int

type MyMap map[string]int

type Base struct {
	ID int
}

type Derived struct {
	Base
	Name string
}
