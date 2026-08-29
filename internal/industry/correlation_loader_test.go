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
	for i := range 20 {
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
	for i := range 10 {
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
	if err := os.WriteFile(replayPath, []byte("Date,Code,Name,TradeVolume,Open,High,Low,Close\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sectorPath := filepath.Join(dir, "sector_symbols.json")
	if err := os.WriteFile(sectorPath, []byte(`{"semiconductor":["2330.TW"]}`), 0o644); err != nil {
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
	if err := os.WriteFile(sectorPath, []byte(`{"semiconductor":["2330.TW"]}`), 0o644); err != nil {
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
	// Write dates out of order to verify sorting
	dates := []string{
		"2024-03-15", "2024-01-02", "2024-02-10", "2024-03-16",
		"2024-01-03", "2024-02-11", "2024-03-17", "2024-01-04",
		"2024-02-12", "2024-03-18", "2024-01-05", "2024-02-13",
		"2024-03-19", "2024-01-06", "2024-02-14", "2024-03-20",
		"2024-01-07",
	}
	for i, date := range dates {
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
	if err := os.WriteFile(sectorPath, []byte(`{"semiconductor":["2330.TW"]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadIndustryReturnsFromReplay(replayPath, sectorPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	semi := result["semiconductor"]
	if len(semi) != 16 {
		t.Errorf("expected 16 returns, got %d", len(semi))
	}
}

func TestIndustryReturnsFormat(t *testing.T) {
	dir := t.TempDir()

	replayPath := filepath.Join(dir, "replay.csv")
	csvFile, _ := os.Create(replayPath)
	w := csv.NewWriter(csvFile)
	_ = w.Write([]string{"Date", "Code", "Name", "TradeVolume", "Open", "High", "Low", "Close"})
	for d := 1; d <= 20; d++ {
		price := float64(100 + d)
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
	_ = os.WriteFile(sectorPath, []byte(`{"semiconductor":["2330.TW"]}`), 0o644)

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
		if len(returns) < 15 {
			t.Errorf("industry %s has %d returns (need >= 15)", id, len(returns))
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
	_ = os.WriteFile(sectorPath, []byte(`{"semiconductor":["2330.TW"]}`), 0o644)

	_, err := LoadIndustryReturnsFromReplay(replayPath, sectorPath)
	if err == nil {
		t.Fatal("expected error for industry with < 15 observations")
	}
}
