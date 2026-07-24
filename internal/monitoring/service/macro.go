package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
)

// MacroIndicatorHealth represents data quality for a single macro indicator.
type MacroIndicatorHealth struct {
	Name       string  `json:"name"`
	Symbol     string  `json:"symbol"`
	Value      float64 `json:"value"`
	ChangePct  float64 `json:"change_pct"`
	Timestamp  int64   `json:"timestamp"`
	Status     string  `json:"status"` // ok, warn, error, missing
	StatusText string  `json:"status_text"`
}

// MacroDataHealthResponse is the response for per-indicator macro data quality.
type MacroDataHealthResponse struct {
	RecordedAt  int64                  `json:"recorded_at"`
	GeneratedAt string                 `json:"generated_at"`
	Indicators  []MacroIndicatorHealth `json:"indicators"`
}

type MacroService struct {
	WorkDir          string
	MacroIngestor    *narrative.MacroIngestor
	TaiwanStressCalc *narrative.TaiwanStressCalculator
	GeoProvider      geopolitical.GeopoliticalRiskProvider
	HistoricalStore  ledger.HistoricalStore
}

func NewMacroService(workDir string, macroIngestor *narrative.MacroIngestor, taiwanStressCalc *narrative.TaiwanStressCalculator) *MacroService {
	return &MacroService{
		WorkDir:          workDir,
		MacroIngestor:    macroIngestor,
		TaiwanStressCalc: taiwanStressCalc,
	}
}

// WithGeoProvider injects a geopolitical risk provider so the on-demand stress
// index can resolve live geo scores.
func (s *MacroService) WithGeoProvider(p geopolitical.GeopoliticalRiskProvider) *MacroService {
	s.GeoProvider = p
	return s
}

// WithHistoricalStore injects the historical ledger so the on-demand stress
// index can fall back to persisted geo scores when the live fetch fails.
func (s *MacroService) WithHistoricalStore(hs ledger.HistoricalStore) *MacroService {
	s.HistoricalStore = hs
	return s
}

// resolveGeoScore returns the best available geopolitical score for the live
// stress index. It mirrors the fallback chain in DashboardAPI.resolveGeoScore so
// the on-demand calculator and the macro-ingest pipeline use the same geo source.
func (s *MacroService) resolveGeoScore(ctx context.Context) geopolitical.GeopoliticalRiskScore {
	if s.GeoProvider != nil {
		geoCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if score, err := s.GeoProvider.FetchScore(geoCtx); err == nil && score.Intensity != 0 && !score.Timestamp.IsZero() {
			return score
		}
	}

	if s.HistoricalStore != nil {
		rows, err := s.HistoricalStore.LoadGeopoliticalHistoryAll(ctx, 1)
		if err == nil && len(rows) > 0 && rows[0].Intensity != 0 && !rows[0].CapturedAt.IsZero() {
			return geopolitical.GeopoliticalRiskScore{
				Intensity: rows[0].Intensity,
				Timestamp: rows[0].CapturedAt,
				Sources:   rows[0].Sources,
			}
		}
	}

	store := geopolitical.NewGeopoliticalStore(filepath.Join(s.WorkDir, constants.StateGeopolitical))
	if fallback, err := store.Load(); err == nil && fallback.Intensity != 0 && !fallback.Timestamp.IsZero() {
		return fallback
	}
	return geopolitical.GeopoliticalRiskScore{}
}

func (s *MacroService) Ingest(ctx context.Context) ([]narrative.NarrativeEvent, marketdata.MacroDataSnapshot, error) {
	return s.MacroIngestor.Ingest(ctx)
}

func (s *MacroService) GetLatestSnapshot() (*marketdata.MacroDataSnapshot, error) {
	path := filepath.Join(s.MacroIngestor.SnapshotDir(), "latest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var snap marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

func (s *MacroService) GetSnapshotByDate(date string) (*marketdata.MacroDataSnapshot, error) {
	if err := shared.ValidateDateParam(date); err != nil {
		return nil, err
	}
	path := filepath.Join(s.MacroIngestor.SnapshotDir(), date+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var snap marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// TimelineEntry is one slot in a macro snapshot timeline response.
//
// Per CF-MS-01 / CF-MS-04:
//   - TradingDate is the snapshot filename date (data's date).
//   - Snapshot is nil when the file is missing/corrupt (NOT zero-patched).
//   - RecordedAt is the provider's recorded_at (Unix seconds; may lag
//     TradingDate by 1-3 days for weekend/holiday ingestion).
//   - SourceStatus reflects whether Snapshot is usable.
type TimelineEntry struct {
	TradingDate  string                        `json:"trading_date"`
	RecordedAt   int64                         `json:"recorded_at"`
	Snapshot     *marketdata.MacroDataSnapshot `json:"snapshot"`
	SourceStatus string                        `json:"source_status"`
}

// datedSnapshotPattern matches YYYY-MM-DD.json files (excludes latest.json,
// previous.json, _metadata.json and any non-date JSON files).
var datedSnapshotPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\.json$`)

// ListSnapshotsInRange reads dated snapshot files from SnapshotDir()
// between `from` and `to` (inclusive, YYYY-MM-DD format). Returns snapshots
// in trading_date ascending order.
//
// Behavior (per CF-MS-01/02/03/04):
//   - Missing/corrupt files are skipped (NOT patched with zero values);
//     their dates are reported in MissingDates.
//   - `limit` caps the response size; if the requested range exceeds limit,
//     capacityLimitHit=true and the response includes only the most recent
//     `limit` snapshots.
//   - limit <= 0 → default 30. limit > 365 → cap at 365 (CF-MS-02).
//   - Empty from/to: from ""  → no lower bound; to ""  → today (UTC).
//   - Only returns error on SnapshotDir unreadable; per-file errors are
//     swallowed per CF-MS-03.
//   - latest.json / previous.json / _metadata.json are NEVER included
//     even if their names were to match the pattern (defense in depth).
func (s *MacroService) ListSnapshotsInRange(
	ctx context.Context, from, to string, limit int,
) (snapshots []TimelineEntry, missingDates []string, capacityLimitHit bool, err error) {
	if limit <= 0 {
		limit = 30
	}
	if limit > 365 {
		limit = 365
	}

	snapDir := s.MacroIngestor.SnapshotDir()
	entries, err := os.ReadDir(snapDir)
	if err != nil {
		return nil, nil, false, fmt.Errorf("read snapshot dir %q: %w", snapDir, err)
	}

	// Default `to` to today (UTC) if empty
	if to == "" {
		to = time.Now().UTC().Format("2006-01-02")
	}

	var datedFiles []string
	for _, e := range entries {
		name := e.Name()
		if !datedSnapshotPattern.MatchString(name) {
			continue
		}
		date := strings.TrimSuffix(name, ".json")
		if from != "" && date < from {
			continue
		}
		if date > to {
			continue
		}
		datedFiles = append(datedFiles, name)
	}
	sort.Strings(datedFiles)

	// CF-MS-02: capacity clamp keeps the most recent `limit` files
	if len(datedFiles) > limit {
		datedFiles = datedFiles[len(datedFiles)-limit:]
		capacityLimitHit = true
	}

	for _, name := range datedFiles {
		date := strings.TrimSuffix(name, ".json")
		path := filepath.Join(snapDir, name)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			missingDates = append(missingDates, date)
			continue
		}
		var snap marketdata.MacroDataSnapshot
		if unmarshalErr := json.Unmarshal(data, &snap); unmarshalErr != nil {
			missingDates = append(missingDates, date)
			continue
		}
		if snap.RecordedAt == 0 {
			// CF-MS-01: corrupt/missing data must NOT be patched with zero values
			missingDates = append(missingDates, date)
			snapshots = append(snapshots, TimelineEntry{
				TradingDate:  date,
				RecordedAt:   0,
				Snapshot:     nil,
				SourceStatus: "missing",
			})
			continue
		}
		snapshots = append(snapshots, TimelineEntry{
			TradingDate:  date,
			RecordedAt:   snap.RecordedAt,
			Snapshot:     &snap,
			SourceStatus: "complete",
		})
	}

	return snapshots, missingDates, capacityLimitHit, nil
}

func (s *MacroService) GetCapitalFlow() (*marketdata.MacroDataSnapshot, error) {
	path := filepath.Join(s.MacroIngestor.SnapshotDir(), "latest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var snap marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

func (s *MacroService) CalculateStressIndex(ctx context.Context) (narrative.TaiwanStressIndex, error) {
	var snap marketdata.MacroDataSnapshot
	path := filepath.Join(s.MacroIngestor.SnapshotDir(), "latest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			_, snap, err = s.MacroIngestor.Ingest(ctx)
			if err != nil {
				return narrative.TaiwanStressIndex{}, fmt.Errorf("ingest failed: %w", err)
			}
		} else {
			return narrative.TaiwanStressIndex{}, fmt.Errorf("read snapshot: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, &snap); err != nil {
			return narrative.TaiwanStressIndex{}, fmt.Errorf("decode snapshot: %w", err)
		}
	}

	// Load previous snapshot for computing change percentages.
	var prev marketdata.MacroDataSnapshot
	prevPath := filepath.Join(s.MacroIngestor.SnapshotDir(), "previous.json")
	prevData, prevErr := os.ReadFile(prevPath)
	if prevErr == nil {
		_ = json.Unmarshal(prevData, &prev)
	}
	// If previous.json does not exist, try to find the most recent dated snapshot.
	if prev.RecordedAt == 0 {
		entries, dirErr := os.ReadDir(s.MacroIngestor.SnapshotDir())
		if dirErr == nil {
			var latestFile string
			var latestTime int64
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || entry.Name() == "latest.json" {
					continue
				}
				info, _ := entry.Info()
				if info != nil && info.ModTime().Unix() > latestTime {
					latestTime = info.ModTime().Unix()
					latestFile = entry.Name()
				}
			}
			if latestFile != "" {
				prevData, _ = os.ReadFile(filepath.Join(s.MacroIngestor.SnapshotDir(), latestFile))
				_ = json.Unmarshal(prevData, &prev)
			}
		}
	}

	geoScore := s.resolveGeoScore(ctx)
	index := s.TaiwanStressCalc.Calculate(snap, prev, geoScore)
	return index, nil
}

// GetMacroDataHealth returns per-indicator data quality status for all macro indicators.
func (s *MacroService) GetMacroDataHealth() (*MacroDataHealthResponse, error) {
	path := filepath.Join(s.MacroIngestor.SnapshotDir(), "latest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read macro snapshot: %w", err)
	}
	var snap marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("decode macro snapshot: %w", err)
	}

	now := time.Now()
	indicators := []struct {
		name string
		pt   marketdata.MacroDataPoint
	}{
		{"DXY-美元指數", snap.DXY},
		{"US10Y-美債10年期", snap.US10Y},
		{"VIX-波動率指數", snap.VIX},
		{"USD/TWD-匯率", snap.USD_TWD},
		{"原油", snap.Oil},
		{"黃金", snap.Gold},
		{"日圓", snap.JPY},
	}

	var result []MacroIndicatorHealth
	for _, item := range indicators {
		h := MacroIndicatorHealth{
			Name:      item.name,
			Symbol:    item.pt.Symbol,
			Value:     item.pt.Value,
			ChangePct: item.pt.ChangePct,
			Timestamp: item.pt.Timestamp,
		}

		switch {
		case item.pt.Symbol == "":
			h.Status = "error"
			h.StatusText = "資料缺失 — 資料提供者未回傳"
		case item.pt.Timestamp == 0:
			h.Status = "error"
			h.StatusText = "無時間戳記"
		case now.Unix()-item.pt.Timestamp > 7*86400:
			h.Status = "error"
			h.StatusText = fmt.Sprintf("資料過期 (%d 天前)", (now.Unix()-item.pt.Timestamp)/86400)
		case now.Unix()-item.pt.Timestamp > 86400:
			h.Status = "warn"
			h.StatusText = fmt.Sprintf("資料待更新 (%d 小時前)", (now.Unix()-item.pt.Timestamp)/3600)
		case item.pt.Value == 0:
			h.Status = "error"
			h.StatusText = "數值異常 — 資料提供者回傳零值"
		case math.Abs(item.pt.ChangePct) < 1e-9 && item.pt.Value > 0:
			// 僅在時間戳記遺失或超過 24 小時時才標記為異常。
			// 近期資料的零變動為正當情況（首次執行或市場持平）。
			if item.pt.Timestamp == 0 || now.Unix()-item.pt.Timestamp > 86400 {
				h.Status = "warn"
				h.StatusText = "日變動率為 0 — 資料可能未更新"
			} else {
				h.Status = "ok"
				h.StatusText = "正常"
			}
		default:
			h.Status = "ok"
			h.StatusText = "正常"
		}
		result = append(result, h)
	}

	return &MacroDataHealthResponse{
		RecordedAt:  snap.RecordedAt,
		GeneratedAt: now.Format(time.RFC3339),
		Indicators:  result,
	}, nil
}
