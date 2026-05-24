package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("convert-experiments-jsonl", flag.ContinueOnError)
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
	var files []string

	rootPath := filepath.Join(dir, "experiments.jsonl")
	if _, err := os.Stat(rootPath); err == nil {
		files = append(files, rootPath)
	}

	return files
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

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".convert-experiments-jsonl-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", path, err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			if _, err := tmpFile.WriteString("\n"); err != nil {
				tmpFile.Close()
				return fmt.Errorf("write blank line %d in %s: %w", lineNum, path, err)
			}
			continue
		}

		var record domain.ExperimentRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			tmpFile.Close()
			return fmt.Errorf("%s line %d: decode error: %w", path, lineNum, err)
		}

		canonical, err := json.Marshal(record)
		if err != nil {
			tmpFile.Close()
			return fmt.Errorf("%s line %d: encode error: %w", path, lineNum, err)
		}

		if _, err := tmpFile.Write(canonical); err != nil {
			tmpFile.Close()
			return fmt.Errorf("write line %d in %s: %w", lineNum, path, err)
		}
		if _, err := tmpFile.WriteString("\n"); err != nil {
			tmpFile.Close()
			return fmt.Errorf("write newline line %d in %s: %w", lineNum, path, err)
		}
	}

	if err := scanner.Err(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("scan %s: %w", path, err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, path, err)
	}

	return nil
}
