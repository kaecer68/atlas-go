package risktest

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadConfig reads a JSON stress-test config file from path. If path is empty
// or the file cannot be read/parsed, it returns an empty Config and logs a
// warning to stderr.
func LoadConfig(path string) Config {
	if path == "" {
		return Config{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: cannot read %s: %v (using defaults)\n", path, err)
		return Config{}
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "config: cannot parse %s: %v (using defaults)\n", path, err)
		return Config{}
	}
	return cfg
}
