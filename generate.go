// (mapgen takes ~100s due to coverage tests; run selectively via: go run ./cmd/mapgen -map arch,routes,fe-be)
//
//go:generate go run ./cmd/gentags
//go:generate go run ./cmd/mapgen -map arch
package main
