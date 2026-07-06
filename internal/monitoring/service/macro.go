package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/narrative"
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
}

func NewMacroService(workDir string, macroIngestor *narrative.MacroIngestor, taiwanStressCalc *narrative.TaiwanStressCalculator) *MacroService {
	return &MacroService{
		WorkDir:          workDir,
		MacroIngestor:    macroIngestor,
		TaiwanStressCalc: taiwanStressCalc,
	}
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

	geoStore := narrative.NewGeopoliticalStore(filepath.Join(s.WorkDir, constants.StateGeopolitical))
	index, err := s.TaiwanStressCalc.CalculateFromSnapshotWithStore(ctx, snap, prev, geoStore)
	if err != nil {
		return narrative.TaiwanStressIndex{}, err
	}
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
