package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/kaecer68/atlas-go/internal/paramcheck"
)

func main() {
	path, strict := "configs/parameters.json", false
	for _, a := range os.Args[1:] {
		if a == "--strict" {
			strict = true
		} else {
			path = a
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "FAIL: cannot read %s: %v\n", path, err)
		os.Exit(1)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "FAIL: invalid JSON in %s: %v\n", path, err)
		os.Exit(1)
	}
	os.Exit(paramcheck.ValidateAndReport(config, path, strict, os.Stdout, os.Stderr))
}
