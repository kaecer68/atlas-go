package server

import (
	"net/url"
)

// urlValues is a tiny helper to build url.Values from a string map. Keeps tool
// handlers free of map→url.Values boilerplate.
func urlValues(in map[string]string) url.Values {
	v := url.Values{}
	for k, vv := range in {
		v.Set(k, vv)
	}
	return v
}
