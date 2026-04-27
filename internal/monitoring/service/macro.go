package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/marketdata"
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

func (s *MacroService) GetLatestSnapshot() ([]byte, error) {
	path := filepath.Join(s.MacroIngestor.SnapshotDir(), "latest.json")
	return os.ReadFile(path)
}

func (s *MacroService) GetSnapshotByDate(date string) ([]byte, error) {
	path := filepath.Join(s.MacroIngestor.SnapshotDir(), date+".json")
	return os.ReadFile(path)
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

	geoStore := narrative.NewGeopoliticalStore(filepath.Join(s.WorkDir, "data/state/geopolitical"))
	index, err := s.TaiwanStressCalc.CalculateFromSnapshotWithStore(ctx, snap, geoStore)
	if err != nil {
		return narrative.TaiwanStressIndex{}, err
	}
	return index, nil
}
