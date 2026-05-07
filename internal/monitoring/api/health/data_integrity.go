package health

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

type IntegrityCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "ok", "warn", "error"
	Message string `json:"message"`
}

type DataIntegrityResponse struct {
	Overall  string            `json:"overall"` // "ok", "degraded", "failing"
	Checks   []IntegrityCheck  `json:"checks"`
	Warnings []string          `json:"warnings"`
}

// HandleDataIntegrity checks data health across the system.
func HandleDataIntegrity(workDir, ledgerDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var checks []IntegrityCheck
		var warnings []string
		errorCount := 0

		// 1. Check sessions exist
		sessionsDir := filepath.Join(ledgerDir, "sessions")
		entries, _ := os.ReadDir(sessionsDir)
		dailyCount := 0
		for _, e := range entries {
			if e.IsDir() && strings.HasSuffix(e.Name(), "-daily") {
				dailyCount++
			}
		}
		if dailyCount == 0 {
			checks = append(checks, IntegrityCheck{"sessions", "error", "no daily sessions found"})
			errorCount++
		} else {
			checks = append(checks, IntegrityCheck{"sessions", "ok", ""})
		}

		// 2. Check summary.json format consistency
		pascalCount := 0
		snakeCount := 0
		for _, e := range entries {
			if !e.IsDir() || !strings.HasSuffix(e.Name(), "-daily") {
				continue
			}
			sp := filepath.Join(sessionsDir, e.Name(), "summary.json")
			data, err := os.ReadFile(sp)
			if err != nil {
				continue
			}
			if strings.Contains(string(data[:200]), `"SessionID"`) {
				pascalCount++
			} else if strings.Contains(string(data[:200]), `"session_id"`) {
				snakeCount++
			}
		}
		if pascalCount > 0 {
			checks = append(checks, IntegrityCheck{"encoding", "error", "found PascalCase summary.json files"})
			warnings = append(warnings, "Some session summaries still use PascalCase encoding")
			errorCount++
		} else {
			checks = append(checks, IntegrityCheck{"encoding", "ok", ""})
		}

		// 3. Check tax data availability (was a known stub)
		taxHasData := false
		for _, e := range entries {
			if !e.IsDir() || !strings.HasSuffix(e.Name(), "-daily") {
				continue
			}
			sp := filepath.Join(sessionsDir, e.Name(), "summary.json")
			data, err := os.ReadFile(sp)
			if err != nil {
				continue
			}
			var summary domain.SessionSummary
			if json.Unmarshal(data, &summary) != nil {
				continue
			}
			if len(summary.TaxSnapshots) > 0 || summary.TotalTaxPaid != 0 {
				taxHasData = true
				break
			}
		}
		if !taxHasData {
			checks = append(checks, IntegrityCheck{"tax_data", "warn", "no sessions have tax data"})
			warnings = append(warnings, "Tax data not populated — run a simulation to generate")
		} else {
			checks = append(checks, IntegrityCheck{"tax_data", "ok", ""})
		}

		// 4. Check for position snapshots
		posCount := 0
		for _, e := range entries {
			if !e.IsDir() || !strings.HasSuffix(e.Name(), "-daily") {
				continue
			}
			if _, err := os.Stat(filepath.Join(sessionsDir, e.Name(), "positions.json")); err == nil {
				posCount++
			}
		}
		if posCount == 0 && dailyCount > 0 {
			checks = append(checks, IntegrityCheck{"position_data", "warn", "no sessions have positions.json"})
			warnings = append(warnings, "Position snapshots not generated — run a simulation to populate")
		} else {
			checks = append(checks, IntegrityCheck{"position_data", "ok", ""})
		}

		// 5. Check replay data freshness
		replayPath := filepath.Join(workDir, "data/replay/tw_extended_90days.csv")
		if info, err := os.Stat(replayPath); err != nil {
			checks = append(checks, IntegrityCheck{"replay_data", "error", "replay file missing"})
			errorCount++
		} else if info.Size() < 10000 {
			checks = append(checks, IntegrityCheck{"replay_data", "warn", "replay file seems too small"})
		} else {
			checks = append(checks, IntegrityCheck{"replay_data", "ok", ""})
		}

		overall := "ok"
		if errorCount > 0 {
			overall = "failing"
		} else if len(warnings) > 0 {
			overall = "degraded"
		}

		shared.WriteJSON(w, http.StatusOK, DataIntegrityResponse{
			Overall:  overall,
			Checks:   checks,
			Warnings: warnings,
		})
	}
}
