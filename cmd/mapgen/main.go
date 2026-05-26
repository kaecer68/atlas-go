// Command mapgen generates system architecture maps for the atlas-go project.
//
// Usage:
//
//	go run ./cmd/mapgen -map arch          # Generate architecture map only
//	go run ./cmd/mapgen -map routes        # Generate API routes map only
//	go run ./cmd/mapgen -map completeness  # Generate module completeness map only
//	go run ./cmd/mapgen -map fe-be         # Generate frontend-backend map only
//	go run ./cmd/mapgen -gen-all           # Generate all maps
//	go run ./cmd/mapgen -help              # Show usage
//
// Output directory: .omo/maps/
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("%v", err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("mapgen", flag.ContinueOnError)
	genAll := fs.Bool("gen-all", false, "generate all maps")
	mapType := fs.String("map", "", "specific map to generate (arch|routes|completeness|fe-be)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *mapType != "" && *genAll {
		return fmt.Errorf("use either -map or -gen-all, not both")
	}

	if !*genAll && *mapType == "" {
		fs.Usage()
		return nil
	}

	start := time.Now()

	if *genAll || *mapType == "arch" {
		fmt.Println("🗺️  Generating architecture map...")
		if err := generateArchitecture(); err != nil {
			return fmt.Errorf("architecture map: %w", err)
		}
		fmt.Println("   ✅ .omo/maps/architecture.md")
	}

	if *genAll || *mapType == "routes" {
		fmt.Println("🗺️  Generating API routes map...")
		if err := generateRoutes(); err != nil {
			return fmt.Errorf("API routes map: %w", err)
		}
		fmt.Println("   ✅ .omo/maps/api-routes.md")
	}

	if *genAll || *mapType == "completeness" {
		fmt.Println("🗺️  Generating module completeness map...")
		if err := generateCompleteness(); err != nil {
			return fmt.Errorf("completeness map: %w", err)
		}
		fmt.Println("   ✅ .omo/maps/module-completeness.md")
	}

	if *genAll || *mapType == "fe-be" {
		fmt.Println("🗺️  Generating frontend-backend map...")
		if err := generateFrontendBackend(); err != nil {
			return fmt.Errorf("frontend-backend map: %w", err)
		}
		fmt.Println("   ✅ .omo/maps/frontend-backend.md")
	}

	fmt.Printf("\n✨ All maps generated in %v\n", time.Since(start).Round(time.Millisecond))
	return nil
}
