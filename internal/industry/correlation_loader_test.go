package industry

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadIndustryReturnsFromReplay_ValidData(t *testing.T) {
	tmpDir := t.TempDir()

	sectorSymbolsPath := filepath.Join(tmpDir, "sector_symbols.json")
	sectorJSON := `{"semiconductor":["2330.TW","2303.TW"],"financials":["2881.TW","2882.TW"]}`
	if err := os.WriteFile(sectorSymbolsPath, []byte(sectorJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	replayPath := filepath.Join(tmpDir, "replay.csv")
	csvFile, err := os.Create(replayPath)
	if err != nil {
		t.Fatal(err)
	}
	writer := csv.NewWriter(csvFile)
	writer.Write([]string{"Date", "Code", "Name", "TradeVolume", "Open", "High", "Low", "Close"})
	for i := 0; i < 20; i++ {
		date := fmt.Sprintf("2026-01-%02d", 1+i)
		writer.Write([]string{date, "2330", "TSMC", "10000", "100", "105", "99", "104"})
		writer.Write([]string{date, "2303", "UMC", "5000", "50", "52", "49", "51"})
		writer.Write([]string{date, "2881", "Fubon", "3000", "60", "62", "59", "61"})
		writer.Write([]string{date, "2882", "Cathay", "2000", "45", "46", "44", "45"})
	}
	writer.Flush()
	csvFile.Close()

	returns, err := LoadIndustryReturnsFromReplay(replayPath, sectorSymbolsPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(returns) == 0 {
		t.Fatal("expected non-empty returns")
	}
	if _, ok := returns["semiconductor"]; !ok {
		t.Error("expected semiconductor industry returns")
	}
	if _, ok := returns["financials"]; !ok {
		t.Error("expected financials industry returns")
	}
}

func TestLoadIndustryReturnsFromReplay_MissingSectorSymbols(t *testing.T) {
	_, err := LoadIndustryReturnsFromReplay("nonexistent.csv", "nonexistent.json")
	if err == nil {
		t.Fatal("expected error for missing sector symbols file")
	}
}

func TestLoadIndustryReturnsFromReplay_MissingReplayCSV(t *testing.T) {
	tmpDir := t.TempDir()
	sectorSymbolsPath := filepath.Join(tmpDir, "sector_symbols.json")
	if err := os.WriteFile(sectorSymbolsPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadIndustryReturnsFromReplay(filepath.Join(tmpDir, "nonexistent.csv"), sectorSymbolsPath)
	if err == nil {
		t.Fatal("expected error for missing replay CSV")
	}
}

func TestLoadIndustryReturnsFromReplay_EmptySymbols(t *testing.T) {
	tmpDir := t.TempDir()

	sectorSymbolsPath := filepath.Join(tmpDir, "sector_symbols.json")
	sectorJSON := `{"empty_sector":[]}`
	if err := os.WriteFile(sectorSymbolsPath, []byte(sectorJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	replayPath := filepath.Join(tmpDir, "replay.csv")
	csvFile, err := os.Create(replayPath)
	if err != nil {
		t.Fatal(err)
	}
	writer := csv.NewWriter(csvFile)
	writer.Write([]string{"Date", "Code", "Name", "TradeVolume", "Open", "High", "Low", "Close"})
	for i := 0; i < 10; i++ {
		writer.Write([]string{fmt.Sprintf("2026-01-%02d", 1+i), "2330", "TSMC", "10000", "100", "105", "99", "104"})
	}
	writer.Flush()
	csvFile.Close()

	_, err = LoadIndustryReturnsFromReplay(replayPath, sectorSymbolsPath)
	if err == nil {
		t.Fatal("expected error when all sectors have empty symbol lists")
	}
}
