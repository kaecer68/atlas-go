// Package storage provides file lifecycle management for data state directories.
//
// LifecycleManager cleans up old files across multiple data directories based on
// configurable retention policies. It is designed to be called by a background task
// that runs daily.
package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// RetentionPolicy defines how files in a directory should be cleaned up.
type RetentionPolicy struct {
	// Dir is the subdirectory name under the state directory (e.g. "macro", "margin").
	Dir string `json:"dir"`
	// MaxAgeDays is the maximum age in days before a file is eligible for deletion.
	MaxAgeDays int `json:"max_age_days"`
	// Pattern is a glob pattern to match files (e.g. "20*.json", "*_margin.json").
	Pattern string `json:"pattern"`
	// ExcludeFiles lists filenames that must never be deleted regardless of age.
	ExcludeFiles []string `json:"exclude_files,omitempty"`
}

// CleanupReport is the aggregate result of a full lifecycle run.
type CleanupReport struct {
	TotalDeleted int            `json:"total_deleted"`
	TotalKept    int            `json:"total_kept"`
	Policies     []PolicyReport `json:"policies"`
}

// PolicyReport captures the result of applying a single retention policy.
type PolicyReport struct {
	Policy     string `json:"policy"`
	Dir        string `json:"dir"`
	MaxAgeDays int    `json:"max_age_days"`
	Deleted    int    `json:"deleted"`
	Kept       int    `json:"kept"`
	OldestKept string `json:"oldest_kept"`
}

// LifecycleManager manages file cleanup across data state directories.
type LifecycleManager struct {
	stateDir   string
	policies   []RetentionPolicy
	reportMu   sync.RWMutex
	lastReport CleanupReport
}

// NewLifecycleManager creates a manager with default retention policies for the
// standard atlas data directories.
func NewLifecycleManager(stateDir string) *LifecycleManager {
	return &LifecycleManager{
		stateDir: stateDir,
		policies: defaultPolicies(),
	}
}

// NewLifecycleManagerWithPolicies creates a manager with custom retention policies.
func NewLifecycleManagerWithPolicies(stateDir string, policies []RetentionPolicy) *LifecycleManager {
	return &LifecycleManager{
		stateDir: stateDir,
		policies: policies,
	}
}

// defaultPolicies returns the standard retention policies for atlas data directories.
func defaultPolicies() []RetentionPolicy {
	return []RetentionPolicy{
		{
			Dir:          "macro",
			MaxAgeDays:   90,
			Pattern:      "20*.json",
			ExcludeFiles: []string{"latest.json"},
		},
		{
			Dir:        "margin",
			MaxAgeDays: 90,
			Pattern:    "*_margin.json",
		},
		{
			Dir:        "export",
			MaxAgeDays: 90,
			Pattern:    "*_export.json",
		},
		{
			Dir:        "capital_flow",
			MaxAgeDays: 90,
			Pattern:    "*.json",
		},
		{
			Dir:        "tsmc_revenue",
			MaxAgeDays: 90,
			Pattern:    "*_revenue.json",
		},
		{
			Dir:        "traces",
			MaxAgeDays: 7,
			Pattern:    "sim-*.jsonl",
		},
		{
			Dir:        "traces",
			MaxAgeDays: 30,
			Pattern:    "session-*.jsonl",
		},
	}
}

// Run executes all retention policies. When dryRun is true, files are counted but
// not actually removed.
func (lm *LifecycleManager) Run(ctx context.Context, dryRun bool) (CleanupReport, error) {
	report := CleanupReport{
		Policies: make([]PolicyReport, 0, len(lm.policies)),
	}

	for _, policy := range lm.policies {
		if err := ctx.Err(); err != nil {
			return report, fmt.Errorf("lifecycle run canceled: %w", err)
		}

		pr, err := lm.applyPolicy(policy, dryRun)
		if err != nil {
			return report, fmt.Errorf("apply policy %s: %w", policy.Dir, err)
		}

		report.TotalDeleted += pr.Deleted
		report.TotalKept += pr.Kept
		report.Policies = append(report.Policies, pr)
	}

	lm.reportMu.Lock()
	lm.lastReport = report
	lm.reportMu.Unlock()
	return report, nil
}

// LastReport returns the most recent cleanup report. Before the first run
// (fresh process, 24h scheduled task not yet fired) it synthesizes a dry-run
// report so the dashboard still shows the retention schedule per directory
// instead of an empty placeholder.
func (lm *LifecycleManager) LastReport() any {
	lm.reportMu.RLock()
	last := lm.lastReport
	lm.reportMu.RUnlock()
	if last.Policies == nil {
		report := CleanupReport{
			Policies: make([]PolicyReport, 0, len(lm.policies)),
		}
		for _, policy := range lm.policies {
			report.Policies = append(report.Policies, PolicyReport{
				Policy:     policy.Dir,
				Dir:        policy.Dir,
				MaxAgeDays: policy.MaxAgeDays,
			})
		}
		return report
	}
	return last
}

// Stats returns current file counts per policy directory without performing any cleanup.
func (lm *LifecycleManager) Stats() (map[string]int, error) {
	result := make(map[string]int, len(lm.policies))

	for _, policy := range lm.policies {
		dirPath := filepath.Join(lm.stateDir, policy.Dir)
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			if os.IsNotExist(err) {
				result[policy.Dir] = 0
				continue
			}
			return nil, fmt.Errorf("read dir %s: %w", policy.Dir, err)
		}

		count, err := countMatches(dirPath, entries, policy.Pattern, policy.ExcludeFiles)
		if err != nil {
			return nil, fmt.Errorf("count matches in %s: %w", policy.Dir, err)
		}
		result[policy.Dir] = count
	}

	return result, nil
}

// applyPolicy applies a single retention policy and returns the report.
func (lm *LifecycleManager) applyPolicy(policy RetentionPolicy, dryRun bool) (PolicyReport, error) {
	pr := PolicyReport{
		Policy:     policy.Dir,
		Dir:        policy.Dir,
		MaxAgeDays: policy.MaxAgeDays,
	}

	dirPath := filepath.Join(lm.stateDir, policy.Dir)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return pr, nil
		}
		return pr, fmt.Errorf("read dir: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -policy.MaxAgeDays)
	var oldestKept time.Time

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		if isExcluded(name, policy.ExcludeFiles) {
			pr.Kept++
			continue
		}

		matched, err := filepath.Match(policy.Pattern, name)
		if err != nil {
			return pr, fmt.Errorf("invalid pattern %q: %w", policy.Pattern, err)
		}
		if !matched {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			if !dryRun {
				if err := os.Remove(filepath.Join(dirPath, name)); err != nil {
					fmt.Fprintf(os.Stderr, "lifecycle: failed to remove %s: %v\n", name, err)
					pr.Kept++
					continue
				}
			}
			pr.Deleted++
		} else {
			pr.Kept++
			if oldestKept.IsZero() || info.ModTime().Before(oldestKept) {
				oldestKept = info.ModTime()
				pr.OldestKept = name
			}
		}
	}

	return pr, nil
}

// fileMatch holds metadata for a file that matched a policy pattern.
type fileMatch struct {
	name    string
	path    string
	modTime time.Time
}

// listMatches returns all files matching the pattern and not in the exclusion list.
func listMatches(dirPath string, entries []os.DirEntry, pattern string, exclude []string) ([]fileMatch, error) {
	var matches []fileMatch

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if isExcluded(name, exclude) {
			continue
		}

		matched, err := filepath.Match(pattern, name)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", pattern, err)
		}
		if !matched {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		matches = append(matches, fileMatch{
			name:    name,
			path:    filepath.Join(dirPath, name),
			modTime: info.ModTime(),
		})
	}

	return matches, nil
}

// countMatches returns the number of files matching pattern and not excluded.
func countMatches(dirPath string, entries []os.DirEntry, pattern string, exclude []string) (int, error) {
	matches, err := listMatches(dirPath, entries, pattern, exclude)
	if err != nil {
		return 0, err
	}
	return len(matches), nil
}

// isExcluded checks if a filename is in the exclusion list.
func isExcluded(name string, exclude []string) bool {
	return slices.Contains(exclude, name)
}
