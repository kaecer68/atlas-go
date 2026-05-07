package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/baseline"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("convert-baseline-policy", flag.ContinueOnError)
	fs.SetOutput(stdout)
	stateDir := fs.String("state-dir", "data/state", "state directory containing baseline_policy.json")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	policyPath := filepath.Join(*stateDir, "baseline_policy.json")
	if _, err := os.Stat(policyPath); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(stdout, "No file: %s not found\n", policyPath)
			return nil
		}
		return fmt.Errorf("stat %s: %w", policyPath, err)
	}

	data, err := os.ReadFile(policyPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", policyPath, err)
	}

	var policy baseline.Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		return fmt.Errorf("decode %s: %w", policyPath, err)
	}

	canonical, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", policyPath, err)
	}

	tmpFile, err := os.CreateTemp(*stateDir, ".convert-baseline-policy-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", policyPath, err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(canonical); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp %s: %w", tmpPath, err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, policyPath); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, policyPath, err)
	}

	return nil
}
