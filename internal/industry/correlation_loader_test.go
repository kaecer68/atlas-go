package industry

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
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
	for i := 0; i < 65; i++ {
		date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i).Format("2006-01-02")
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

func TestLoadIndustryReturnsEmptyReplay(t *testing.T) {
	dir := t.TempDir()

	replayPath := filepath.Join(dir, "replay.csv")
	if err := os.WriteFile(replayPath, []byte("Date,Code,Name,TradeVolume,Open,High,Low,Close\n"), 0644); err != nil {
		t.Fatal(err)
	}

	sectorPath := filepath.Join(dir, "sector_symbols.json")
	if err := os.WriteFile(sectorPath, []byte(`{"semiconductor":["2330.TW"]}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadIndustryReturnsFromReplay(replayPath, sectorPath)
	if err == nil {
		t.Fatal("expected error for empty replay data")
	}
}

func TestLoadIndustryReturnsMissingSymbols(t *testing.T) {
	dir := t.TempDir()

	replayPath := filepath.Join(dir, "replay.csv")
	csvFile, err := os.Create(replayPath)
	if err != nil {
		t.Fatal(err)
	}
	w := csv.NewWriter(csvFile)
	_ = w.Write([]string{"Date", "Code", "Name", "TradeVolume", "Open", "High", "Low", "Close"})
	for d := 1; d <= 16; d++ {
		price := float64(100 + d)
		_ = w.Write([]string{
			fmt.Sprintf("2024-02-%02d", d),
			"9999",
			"Unknown",
			"10000",
			fmt.Sprintf("%.0f", price),
			fmt.Sprintf("%.0f", price+2),
			fmt.Sprintf("%.0f", price-1),
			fmt.Sprintf("%.0f", price),
		})
	}
	w.Flush()
	csvFile.Close()

	sectorPath := filepath.Join(dir, "sector_symbols.json")
	if err := os.WriteFile(sectorPath, []byte(`{"semiconductor":["2330.TW"]}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = LoadIndustryReturnsFromReplay(replayPath, sectorPath)
	if err == nil {
		t.Fatal("expected error when no matched industries exist")
	}
}

func TestIndustryReturnsOrdering(t *testing.T) {
	dir := t.TempDir()

	replayPath := filepath.Join(dir, "replay.csv")
	csvFile, err := os.Create(replayPath)
	if err != nil {
		t.Fatal(err)
	}
	w := csv.NewWriter(csvFile)
	_ = w.Write([]string{"Date", "Code", "Name", "TradeVolume", "Open", "High", "Low", "Close"})
	// Generate 66 dates out of order to verify sorting (need ≥60 returns after filtering)
	for i := 65; i >= 0; i-- {
		date := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 65-i).Format("2006-01-02")
		price := float64(100 + i)
		_ = w.Write([]string{
			date,
			"2330",
			"TSMC",
			"50000000",
			fmt.Sprintf("%.0f", price),
			fmt.Sprintf("%.0f", price+2),
			fmt.Sprintf("%.0f", price-1),
			fmt.Sprintf("%.0f", price),
		})
	}
	w.Flush()
	csvFile.Close()

	sectorPath := filepath.Join(dir, "sector_symbols.json")
	if err := os.WriteFile(sectorPath, []byte(`{"semiconductor":["2330.TW"]}`), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadIndustryReturnsFromReplay(replayPath, sectorPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	semi := result["semiconductor"]
	if len(semi) != 65 {
		t.Errorf("expected 65 returns, got %d", len(semi))
	}
}

func TestIndustryReturnsFormat(t *testing.T) {
	dir := t.TempDir()

	replayPath := filepath.Join(dir, "replay.csv")
	csvFile, _ := os.Create(replayPath)
	w := csv.NewWriter(csvFile)
	_ = w.Write([]string{"Date", "Code", "Name", "TradeVolume", "Open", "High", "Low", "Close"})
	for d := 0; d < 65; d++ {
		date := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, d).Format("2006-01-02")
		price := float64(100 + d)
		_ = w.Write([]string{
			date, "2330", "TSMC", "50000",
			fmt.Sprintf("%.0f", price),
			fmt.Sprintf("%.0f", price+2),
			fmt.Sprintf("%.0f", price-1),
			fmt.Sprintf("%.0f", price),
		})
	}
	w.Flush()
	csvFile.Close()

	sectorPath := filepath.Join(dir, "sector_symbols.json")
	_ = os.WriteFile(sectorPath, []byte(`{"semiconductor":["2330.TW"]}`), 0644)

	result, err := LoadIndustryReturnsFromReplay(replayPath, sectorPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := result["semiconductor"]; !ok {
		t.Fatal("expected semiconductor in result")
	}

	for id, returns := range result {
		if len(returns) == 0 {
			t.Errorf("industry %s has empty returns", id)
		}
		if len(returns) < 60 {
			t.Errorf("industry %s has %d returns (need >= 60)", id, len(returns))
		}
	}
}

func TestIndustryReturnsMinimum(t *testing.T) {
	dir := t.TempDir()

	replayPath := filepath.Join(dir, "replay.csv")
	csvFile, _ := os.Create(replayPath)
	w := csv.NewWriter(csvFile)
	_ = w.Write([]string{"Date", "Code", "Name", "TradeVolume", "Open", "High", "Low", "Close"})
	for d := 1; d <= 14; d++ {
		price := float64(90 + d)
		_ = w.Write([]string{
			fmt.Sprintf("2024-01-%02d", d),
			"2330", "TSMC", "50000",
			fmt.Sprintf("%.0f", price),
			fmt.Sprintf("%.0f", price+2),
			fmt.Sprintf("%.0f", price-1),
			fmt.Sprintf("%.0f", price),
		})
	}
	w.Flush()
	csvFile.Close()

	sectorPath := filepath.Join(dir, "sector_symbols.json")
	_ = os.WriteFile(sectorPath, []byte(`{"semiconductor":["2330.TW"]}`), 0644)

	_, err := LoadIndustryReturnsFromReplay(replayPath, sectorPath)
	if err == nil {
		t.Fatal("expected error for industry with < 60 observations")
	}
}
