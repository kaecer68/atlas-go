package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type sessionSummary struct {
	SessionID      string  `json:"session_id"`
	PortfolioValue float64 `json:"portfolio_value"`
}

func main() {
	sessionsDir := os.Args[1]
	stateFile := os.Args[2]

	// Collect all session summaries
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read dir: %v\n", err)
		os.Exit(1)
	}

	type entry struct {
		date string
		pv   float64
	}
	var items []entry

	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "session-") {
			continue
		}
		summaryPath := filepath.Join(sessionsDir, e.Name(), "summary.json")
		data, err := os.ReadFile(summaryPath)
		if err != nil {
			continue
		}
		var s sessionSummary
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		// Extract date from session ID: "session-20260101-daily"
		parts := strings.Split(s.SessionID, "-")
		if len(parts) < 2 {
			continue
		}
		date := parts[1]
		items = append(items, entry{date, s.PortfolioValue})
	}

	if len(items) < 2 {
		fmt.Fprintf(os.Stderr, "need at least 2 sessions, got %d\n", len(items))
		os.Exit(1)
	}

	// Sort by date
	sort.Slice(items, func(i, j int) bool { return items[i].date < items[j].date })

	// Compute daily returns
	var returns []float64
	for i := 1; i < len(items); i++ {
		prev := items[i-1].pv
		curr := items[i].pv
		if prev > 0 && curr > 0 {
			returns = append(returns, (curr-prev)/prev)
		}
	}

	fmt.Printf("Sessions: %d, Returns: %d\n", len(items), len(returns))
	if len(returns) > 0 {
		fmt.Printf("First: %.4f%%, Last: %.4f%%\n", returns[0]*100, returns[len(returns)-1]*100)
	}

	// Read existing simulation_state.json
	stateData, err := os.ReadFile(stateFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read state: %v\n", err)
		os.Exit(1)
	}
	var state map[string]any
	if err := json.Unmarshal(stateData, &state); err != nil {
		fmt.Fprintf(os.Stderr, "parse state: %v\n", err)
		os.Exit(1)
	}

	// Write daily_returns
	state["daily_returns"] = returns

	out, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
		os.Exit(1)
	}

	tmpPath := stateFile + ".tmp"
	if err := os.WriteFile(tmpPath, out, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write tmp: %v\n", err)
		os.Exit(1)
	}
	if err := os.Rename(tmpPath, stateFile); err != nil {
		fmt.Fprintf(os.Stderr, "rename: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Wrote %d daily returns to %s\n", len(returns), stateFile)
	if len(returns) < 252 {
		fmt.Printf("NOTE: %d more observations needed for full 252-gate VaR.\n", 252-len(returns))
		fmt.Println("Use CalculateVaRPercentile for lower-gate VaR estimates.")
	}
}
