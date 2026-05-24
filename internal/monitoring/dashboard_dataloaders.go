package monitoring

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

// DataLoader provides methods for loading session data and building symbol-sector mappings.
// It is implemented by DashboardAPI and can be used by sub-packages to avoid direct struct coupling.
type DataLoader interface {
	LoadRecommendationOutcomes(sessionID string) ([]domain.RecommendationOutcome, error)
	BuildSymbolSectorMap() map[string]string
}

// LoadRecommendationOutcomes reads recommendation outcomes from the ledger directory.
// If sessionID is empty, it auto-discovers the latest session.
func LoadRecommendationOutcomes(ledgerDir, sessionID string) ([]domain.RecommendationOutcome, error) {
	sessionsDir := filepath.Join(ledgerDir, "sessions")
	if sessionID == "" {
		entries, err := os.ReadDir(sessionsDir)
		if err != nil {
			return nil, err
		}
		var latest string
		for _, entry := range entries {
			if entry.IsDir() && entry.Name() > latest {
				latest = entry.Name()
			}
		}
		sessionID = latest
	} else {
		if err := shared.ValidateSessionID(sessionID); err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(sessionsDir)
		if err != nil {
			return nil, err
		}
		found := false
		for _, entry := range entries {
			if entry.IsDir() && entry.Name() == sessionID {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}
	}
	path := filepath.Join(sessionsDir, sessionID, "recommendation_outcomes.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var outcomes []domain.RecommendationOutcome
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var oc domain.RecommendationOutcome
		if err := json.Unmarshal([]byte(line), &oc); err != nil {
			logging.Warn("dashboard_dataloaders", "corrupted_outcome_skipped", logging.Err(err))
			continue
		}
		outcomes = append(outcomes, oc)
	}
	return outcomes, scanner.Err()
}

// BuildSymbolSectorMap constructs a symbol-to-sector mapping from the industry classifier.
func BuildSymbolSectorMap(classifier *industry.ClassificationTree) map[string]string {
	m := make(map[string]string)
	if classifier == nil {
		return m
	}
	for _, seg := range classifier.GetAllSegments() {
		for _, sym := range seg.RepresentativeStocks {
			m[sym] = seg.ID
		}
	}
	return m
}

// Ensure DashboardAPI implements DataLoader interface.
var _ DataLoader = (*DashboardAPI)(nil)

// LoadRecommendationOutcomes implements DataLoader.
func (a *DashboardAPI) LoadRecommendationOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	return LoadRecommendationOutcomes(a.ledgerDir, sessionID)
}

// BuildSymbolSectorMap implements DataLoader.
func (a *DashboardAPI) BuildSymbolSectorMap() map[string]string {
	if a.industryService == nil {
		return make(map[string]string)
	}
	return BuildSymbolSectorMap(a.industryService.Classifier)
}
