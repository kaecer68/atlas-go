package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("rewrite-state", flag.ContinueOnError)
	fs.SetOutput(stdout)
	stateDir := fs.String("state-dir", "data/state", "state directory to rewrite")
	archiveBase := fs.String("archive-base", "data/state-archive", "archive base directory")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	if _, err := os.Stat(*stateDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("state directory does not exist: %w", err)
		}
		return fmt.Errorf("stat state dir: %w", err)
	}

	ts := time.Now().UTC().Format("20060102T150405Z")
	archiveDir := filepath.Join(*archiveBase, ts)
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return fmt.Errorf("create archive dir: %w", err)
	}

	if err := archiveDirContents(*stateDir, archiveDir); err != nil {
		return fmt.Errorf("archive: %w", err)
	}

	count := 0

	c, err := convertBaselinePolicy(*stateDir)
	if err != nil {
		fmt.Fprintln(stdout, archiveDir)
		return fmt.Errorf("convert baseline policy: %w", err)
	}
	count += c

	c, err = convertRecommendationOutcomes(*stateDir)
	if err != nil {
		fmt.Fprintln(stdout, archiveDir)
		return fmt.Errorf("convert recommendation outcomes: %w", err)
	}
	count += c

	c, err = convertExperimentsJSONL(*stateDir)
	if err != nil {
		fmt.Fprintln(stdout, archiveDir)
		return fmt.Errorf("convert experiments jsonl: %w", err)
	}
	count += c

	c, err = convertExperimentResults(*stateDir)
	if err != nil {
		fmt.Fprintln(stdout, archiveDir)
		return fmt.Errorf("convert experiment results: %w", err)
	}
	count += c

	fmt.Fprintf(stdout, "Archive: %s\n", archiveDir)
	fmt.Fprintf(stdout, "Rewritten %d file(s)\n", count)
	return nil
}

func archiveDirContents(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dstPath := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}
		return copyFile(path, dstPath)
	})
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func convertBaselinePolicy(stateDir string) (int, error) {
	policyPath := filepath.Join(stateDir, "baseline_policy.json")
	if _, err := os.Stat(policyPath); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("stat %s: %w", policyPath, err)
	}

	data, err := os.ReadFile(policyPath)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", policyPath, err)
	}

	if len(data) == 0 {
		return 0, nil
	}

	var policy baseline.Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		return 0, fmt.Errorf("decode %s: %w", policyPath, err)
	}

	canonical, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("encode %s: %w", policyPath, err)
	}

	tmpFile, err := os.CreateTemp(stateDir, ".convert-baseline-policy-*.tmp")
	if err != nil {
		return 0, fmt.Errorf("create temp for %s: %w", policyPath, err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(canonical); err != nil {
		tmpFile.Close()
		return 0, fmt.Errorf("write temp %s: %w", tmpPath, err)
	}
	if err := tmpFile.Close(); err != nil {
		return 0, fmt.Errorf("close temp %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, policyPath); err != nil {
		return 0, fmt.Errorf("rename %s -> %s: %w", tmpPath, policyPath, err)
	}

	return 1, nil
}

func convertRecommendationOutcomes(stateDir string) (int, error) {
	var files []string

	rootPath := filepath.Join(stateDir, "recommendation_outcomes.jsonl")
	if _, err := os.Stat(rootPath); err == nil {
		files = append(files, rootPath)
	}

	sessionPattern := filepath.Join(stateDir, "sessions", "*", "recommendation_outcomes.jsonl")
	matches, _ := filepath.Glob(sessionPattern)
	files = append(files, matches...)

	if len(files) == 0 {
		return 0, nil
	}

	count := 0
	for _, path := range files {
		if err := convertRecommendationOutcomesFile(path); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func convertRecommendationOutcomesFile(path string) error {
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
	tmpFile, err := os.CreateTemp(dir, ".convert-recommendation-outcomes-*.tmp")
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

		var outcome domain.RecommendationOutcome
		if err := json.Unmarshal([]byte(line), &outcome); err != nil {
			tmpFile.Close()
			return fmt.Errorf("%s line %d: decode error: %w", path, lineNum, err)
		}

		canonical, err := json.Marshal(outcome)
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

func convertExperimentsJSONL(stateDir string) (int, error) {
	rootPath := filepath.Join(stateDir, "experiments.jsonl")
	if _, err := os.Stat(rootPath); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("stat %s: %w", rootPath, err)
	}

	if err := convertExperimentsJSONLFile(rootPath); err != nil {
		return 0, err
	}
	return 1, nil
}

func convertExperimentsJSONLFile(path string) error {
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

func convertExperimentResults(stateDir string) (int, error) {
	pattern := filepath.Join(stateDir, "experiments", "*.json")
	matches, _ := filepath.Glob(pattern)

	if len(matches) == 0 {
		return 0, nil
	}

	count := 0
	for _, path := range matches {
		if err := convertExperimentResultFile(path); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func convertExperimentResultFile(path string) error {
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
