package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type artifactItem struct {
	Path           string
	Classification string
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("check-persistence-format", flag.ContinueOnError)
	fs.SetOutput(stdout)
	dir := fs.String("dir", "data/state", "state directory to scan")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	items := inventoryArtifacts(*dir)
	if len(items) == 0 {
		fmt.Fprintln(stdout, "No artifacts found.")
		return nil
	}

	fmt.Fprintf(stdout, "Scanned: %s\n\n", *dir)
	fmt.Fprintln(stdout, "Artifact Inventory:")
	fmt.Fprintln(stdout, "-------------------")

	for _, item := range items {
		fmt.Fprintf(stdout, "%-60s %s\n", item.Path, item.Classification)
	}

	fmt.Fprintln(stdout, "\nWriter Consistency Checks:")
	fmt.Fprintln(stdout, "--------------------------")

	checked := 0
	for _, item := range items {
		if !strings.Contains(item.Path, "recommendation_outcomes.jsonl") {
			continue
		}
		checked++
		issues := checkWriterConsistency(item.Path)
		if len(issues) == 0 {
			fmt.Fprintf(stdout, "OK   %s\n", item.Path)
		} else {
			fmt.Fprintf(stdout, "FAIL %s\n", item.Path)
			for _, issue := range issues {
				fmt.Fprintf(stdout, "     - %s\n", issue)
			}
		}
	}

	if checked == 0 {
		fmt.Fprintln(stdout, "No recommendation_outcomes.jsonl files found.")
	}

	return nil
}

func inventoryArtifacts(dir string) []artifactItem {
	var items []artifactItem

	patterns := []struct {
		glob string
	}{
		{filepath.Join(dir, "sessions", "*", "summary.json")},
		{filepath.Join(dir, "sessions", "*", "recommendation_outcomes.jsonl")},
		{filepath.Join(dir, "recommendation_outcomes.jsonl")},
		{filepath.Join(dir, "experiments.jsonl")},
		{filepath.Join(dir, "experiments", "*.json")},
		{filepath.Join(dir, "baseline_policy.json")},
	}

	for _, p := range patterns {
		matches, _ := filepath.Glob(p.glob)
		for _, path := range matches {
			items = append(items, artifactItem{
				Path:           path,
				Classification: classifyArtifact(path),
			})
		}
	}

	return items
}

func classifyArtifact(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "unknown"
	}

	if info.Size() == 0 {
		return "empty"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "unknown"
	}

	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".jsonl" {
		return classifyJSONLContent(data)
	}

	return classifyJSONContent(data)
}

func classifyJSONContent(data []byte) string {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return "unknown"
	}

	return classifyKeys(raw)
}

func classifyJSONLContent(data []byte) string {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	var firstLine []byte
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			firstLine = []byte(line)
			break
		}
	}

	if len(firstLine) == 0 {
		return "empty"
	}

	var raw map[string]any
	if err := json.Unmarshal(firstLine, &raw); err != nil {
		return "unknown"
	}

	return classifyKeys(raw)
}

func classifyKeys(obj map[string]any) string {
	if len(obj) == 0 {
		return "empty"
	}

	hasSnake := false
	hasPascal := false

	for key := range obj {
		if isSnakeCase(key) {
			hasSnake = true
		} else if isPascalCase(key) {
			hasPascal = true
		}
	}

	if hasSnake && hasPascal {
		return "mixed"
	}
	if hasPascal {
		return "pascal_case"
	}
	if hasSnake {
		return "snake_case"
	}
	return "unknown"
}

func isSnakeCase(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if unicode.IsUpper(r) {
			return false
		}
	}
	return strings.Contains(s, "_")
}

func isPascalCase(s string) bool {
	if s == "" {
		return false
	}
	first := rune(s[0])
	if !unicode.IsUpper(first) {
		return false
	}
	for _, r := range s {
		if r == '_' {
			return false
		}
	}
	return true
}

func checkWriterConsistency(path string) []string {
	var issues []string

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{"file missing"}
		}
		return []string{fmt.Sprintf("stat error: %v", err)}
	}

	if info.Size() == 0 {
		summaryIssues := checkEmptyWithSiblingSummary(path)
		return summaryIssues
	}

	f, err := os.Open(path)
	if err != nil {
		return []string{fmt.Sprintf("open error: %v", err)}
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	pascalKeyPattern := regexp.MustCompile(`"[A-Z][a-zA-Z0-9]*[A-Z][a-zA-Z0-9]*"\s*:`)

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var outcome domain.RecommendationOutcome
		if err := json.Unmarshal([]byte(line), &outcome); err != nil {
			issues = append(issues, fmt.Sprintf("line %d: decode error: %v", lineNum, err))
			continue
		}

		reencoded, err := json.Marshal(outcome)
		if err != nil {
			issues = append(issues, fmt.Sprintf("line %d: re-encode error: %v", lineNum, err))
			continue
		}

		if pascalKeyPattern.MatchString(line) {
			issues = append(issues, fmt.Sprintf("line %d: contains PascalCase keys (legacy format)", lineNum))
		}

		var decoded map[string]any
		if err := json.Unmarshal(reencoded, &decoded); err != nil {
			issues = append(issues, fmt.Sprintf("line %d: re-decode error: %v", lineNum, err))
			continue
		}

		for key := range decoded {
			if isPascalCase(key) {
				issues = append(issues, fmt.Sprintf("line %d: re-encoded output contains PascalCase key %q", lineNum, key))
			}
		}
	}

	if err := scanner.Err(); err != nil {
		issues = append(issues, fmt.Sprintf("scan error: %v", err))
	}

	return issues
}

func checkEmptyWithSiblingSummary(outcomesPath string) []string {
	parentDir := filepath.Dir(outcomesPath)
	grandparentDir := filepath.Dir(parentDir)
	if filepath.Base(outcomesPath) == "recommendation_outcomes.jsonl" && filepath.Base(grandparentDir) != "sessions" {
		return nil
	}

	sessionDir := parentDir
	summaryPath := filepath.Join(sessionDir, "summary.json")

	data, err := os.ReadFile(summaryPath)
	if err != nil {
		return []string{"file empty"}
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return []string{"file empty"}
	}

	countVal, ok := raw["outcome_count"]
	if !ok {
		countVal, ok = raw["OutcomeCount"]
	}
	if !ok {
		return []string{"file empty"}
	}

	count, ok := toFloat64(countVal)
	if !ok {
		return []string{"file empty"}
	}

	if count == 0 {
		return nil
	}

	return []string{fmt.Sprintf("file empty but summary reports outcome_count=%.0f", count)}
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}
