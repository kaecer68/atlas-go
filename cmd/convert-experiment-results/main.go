package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("convert-experiment-results", flag.ContinueOnError)
	fs.SetOutput(stdout)
	dir := fs.String("dir", "data/state", "state directory to scan")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	files := discoverFiles(*dir)
	if len(files) == 0 {
		fmt.Fprintln(stdout, "No files found.")
		return nil
	}

	for _, path := range files {
		if err := convertFile(path); err != nil {
			return err
		}
	}

	return nil
}

func discoverFiles(dir string) []string {
	pattern := filepath.Join(dir, "experiments", "*.json")
	matches, _ := filepath.Glob(pattern)
	return matches
}

func convertFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}

	if info.Size() == 0 {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	var result domain.PromptExperimentResult
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("%s: decode error: %w", path, err)
	}

	canonical, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: encode error: %w", path, err)
	}

	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".convert-experiment-results-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", path, err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(canonical); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, path, err)
	}

	return nil
}
