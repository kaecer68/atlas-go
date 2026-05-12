package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

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

	geoStore := narrative.NewGeopoliticalStore(filepath.Join(s.WorkDir, "data/state/geopolitical"))
	index, err := s.TaiwanStressCalc.CalculateFromSnapshotWithStore(ctx, snap, prev, geoStore)
	if err != nil {
		return narrative.TaiwanStressIndex{}, err
	}
	return index, nil
}
