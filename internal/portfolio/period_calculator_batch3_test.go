package portfolio

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Helper: write a single-day sector index file (canonical 8-industry schema).
func writeSectorIndexFile(t *testing.T, dir, date string, industries map[string]float64) {
	t.Helper()
	dateCompact := strings.ReplaceAll(date, "-", "")
	filename := fmt.Sprintf("sector_indices_%s_%s.json", dateCompact, dateCompact)
	out := make(map[string][]map[string]any)
	for industry, ret := range industries {
		out[industry] = []map[string]any{
			{"date": date, "industry": industry, "index": 100.0 + ret, "return_pct": ret},
		}
	}
	data, _ := json.Marshal(out)
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0644); err != nil {
		t.Fatal(err)
	}
}

// ── W2: SectorRotationFlag tests ──

func TestEnrichSectorRotation_NoRotation(t *testing.T) {
	dir := t.TempDir()
	// 10 trading days: same top 3 industries each day → no rotation.
	// Top 3 industries by cumulative return must match between windows
	// for rotation flag to be false. Both windows rank the same top 3.
	industries := map[string]float64{
		"semiconductor": 3.0, "electronics": 2.0, "financials": 1.0,
		"steel": 0.5, "energy": 0.0, "cement": -0.5, "construction": -1.0, "shipping": -2.0,
	}
	prior := map[string]float64{
		"semiconductor": 3.0, "electronics": 2.0, "financials": 1.0,
		"steel": 0.0, "energy": -1.0, "cement": -2.0, "construction": -3.0, "shipping": -4.0,
	}
	dates := []string{
		"2026-07-14", "2026-07-15", "2026-07-16", "2026-07-17", "2026-07-18",
		"2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24", "2026-07-25",
	}
	for idx, d := range dates {
		if idx < 5 {
			writeSectorIndexFile(t, dir, d, prior)
		} else {
			writeSectorIndexFile(t, dir, d, industries)
		}
	}

	var ind PeriodIndicators
	c := NewCalculator()
	c.EnrichSectorRotation(&ind, "2026-07-25", dir)
	if ind.SectorRotationFlag {
		t.Errorf("expected SectorRotationFlag=false (same top 3), got true")
	}
}

func TestEnrichSectorRotation_RotationDetected(t *testing.T) {
	dir := t.TempDir()
	// Prior window top 3: semiconductor, electronics, financials
	prior := map[string]float64{
		"semiconductor": 5.0, "electronics": 4.0, "financials": 3.0,
		"steel": 1.0, "energy": 0.5, "cement": 0.0, "construction": -1.0, "shipping": -2.0,
	}
	// Current window top 3: energy, shipping, cement (completely different)
	current := map[string]float64{
		"energy": 10.0, "shipping": 8.0, "cement": 6.0,
		"semiconductor": 1.0, "electronics": 1.0, "financials": 1.0, "steel": 0.5, "construction": 0.0,
	}
	dates := []string{
		"2026-07-14", "2026-07-15", "2026-07-16", "2026-07-17", "2026-07-18",
		"2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24", "2026-07-25",
	}
	for idx, d := range dates {
		if idx < 5 {
			writeSectorIndexFile(t, dir, d, prior)
		} else {
			writeSectorIndexFile(t, dir, d, current)
		}
	}

	var ind PeriodIndicators
	c := NewCalculator()
	c.EnrichSectorRotation(&ind, "2026-07-25", dir)
	if !ind.SectorRotationFlag {
		t.Errorf("expected SectorRotationFlag=true (top 3 changed), got false")
	}
}

func TestEnrichSectorRotation_InsufficientDays(t *testing.T) {
	dir := t.TempDir()
	// Only 5 days available — less than MinDaysSectorRotationFlag=10.
	industries := map[string]float64{
		"semiconductor": 1.0, "electronics": 1.0, "financials": 1.0, "steel": 1.0, "energy": 1.0,
	}
	for _, d := range []string{
		"2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24", "2026-07-25",
	} {
		writeSectorIndexFile(t, dir, d, industries)
	}

	var ind PeriodIndicators
	c := NewCalculator()
	c.EnrichSectorRotation(&ind, "2026-07-25", dir)
	if ind.SectorRotationFlag {
		t.Errorf("expected SectorRotationFlag=false (insufficient data), got true")
	}
}

func TestEnrichSectorRotation_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	var ind PeriodIndicators
	c := NewCalculator()
	c.EnrichSectorRotation(&ind, "2026-07-25", dir)
	if ind.SectorRotationFlag {
		t.Errorf("expected SectorRotationFlag=false (no data), got true")
	}
}

func TestEnrichSectorRotation_EmptyDirString(t *testing.T) {
	var ind PeriodIndicators
	c := NewCalculator()
	c.EnrichSectorRotation(&ind, "2026-07-25", "")
	// Should be no-op, no panic, no warn.
	if ind.SectorRotationFlag {
		t.Errorf("expected SectorRotationFlag=false (empty dir string), got true")
	}
}

func TestEnrichSectorRotation_Determinism(t *testing.T) {
	dir := t.TempDir()
	industries := map[string]float64{
		"semiconductor": 1.0, "electronics": 1.0, "financials": 1.0, "steel": 1.0, "energy": 1.0, "cement": 1.0, "construction": 1.0, "shipping": 1.0,
	}
	for _, d := range []string{
		"2026-07-14", "2026-07-15", "2026-07-16", "2026-07-17", "2026-07-18",
		"2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24", "2026-07-25",
	} {
		writeSectorIndexFile(t, dir, d, industries)
	}

	var ind1, ind2 PeriodIndicators
	c := NewCalculator()
	c.EnrichSectorRotation(&ind1, "2026-07-25", dir)
	c.EnrichSectorRotation(&ind2, "2026-07-25", dir)
	if ind1.SectorRotationFlag != ind2.SectorRotationFlag {
		t.Errorf("non-deterministic: %v vs %v", ind1.SectorRotationFlag, ind2.SectorRotationFlag)
	}
}

// ── W3: PublicBankConsecBuyDays tests ──

// Helper: write both legacy and brokers files for a single date.
func writeGovFlowDate(t *testing.T, dir, date, totalNet string) {
	t.Helper()
	dateCompact := strings.ReplaceAll(date, "-", "")
	legacy := fmt.Sprintf(`{"date":"%s","total_net":%s,"source":"broker-aggregate"}`, dateCompact, totalNet)
	if err := os.WriteFile(filepath.Join(dir, dateCompact+".json"), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	brokers := fmt.Sprintf(`{"date":"%s","brokers":[{"name":"彰銀","net":%s},{"name":"華南永昌","net":%s}]}`, dateCompact, totalNet, totalNet)
	if err := os.WriteFile(filepath.Join(dir, dateCompact+"_brokers.json"), []byte(brokers), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestEnrichGovernmentBroker_AllLegacyZeroFiles(t *testing.T) {
	dir := t.TempDir()
	// 6 days of legacy YYYYMMDD.json with total_net=0 but NO _brokers.json
	// (P0-3 rule: legacy-only zero files are invalid).
	dates := []string{"2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24", "2026-07-27", "2026-07-28"}
	for _, d := range dates {
		dateCompact := strings.ReplaceAll(d, "-", "")
		legacy := fmt.Sprintf(`{"date":"%s","total_net":0,"source":"broker-aggregate"}`, dateCompact)
		if err := os.WriteFile(filepath.Join(dir, dateCompact+".json"), []byte(legacy), 0644); err != nil {
			t.Fatal(err)
		}
	}

	var ind PeriodIndicators
	c := NewCalculator()
	c.EnrichGovernmentBroker(&ind, "2026-07-28", dir)
	if ind.PublicBankConsecBuyDays != 0 {
		t.Errorf("expected PublicBankConsecBuyDays=0 (all legacy zero files), got %d", ind.PublicBankConsecBuyDays)
	}
}

func TestEnrichGovernmentBroker_ConsecutiveBuyDays(t *testing.T) {
	dir := t.TempDir()
	// 5 days of valid files (both legacy + brokers), all net=positive.
	dates := []string{"2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24", "2026-07-25"}
	for _, d := range dates {
		writeGovFlowDate(t, dir, d, "100000000")
	}

	var ind PeriodIndicators
	c := NewCalculator()
	c.EnrichGovernmentBroker(&ind, "2026-07-25", dir)
	if ind.PublicBankConsecBuyDays != 5 {
		t.Errorf("expected PublicBankConsecBuyDays=5, got %d", ind.PublicBankConsecBuyDays)
	}
}

func TestEnrichGovernmentBroker_StreakBrokenByNegative(t *testing.T) {
	dir := t.TempDir()
	// 5 days: 3 buy, then 1 negative, then 1 buy → streak=1 (most recent only)
	net := []string{"100", "100", "100", "-50", "100"}
	dates := []string{"2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24", "2026-07-25"}
	for i, d := range dates {
		writeGovFlowDate(t, dir, d, net[i])
	}

	var ind PeriodIndicators
	c := NewCalculator()
	c.EnrichGovernmentBroker(&ind, "2026-07-25", dir)
	if ind.PublicBankConsecBuyDays != 1 {
		t.Errorf("expected PublicBankConsecBuyDays=1 (streak reset by -50), got %d", ind.PublicBankConsecBuyDays)
	}
}

func TestEnrichGovernmentBroker_InsufficientValidDates(t *testing.T) {
	dir := t.TempDir()
	// Only 2 valid dates (need 5).
	dates := []string{"2026-07-24", "2026-07-25"}
	for _, d := range dates {
		writeGovFlowDate(t, dir, d, "100")
	}

	var ind PeriodIndicators
	c := NewCalculator()
	c.EnrichGovernmentBroker(&ind, "2026-07-25", dir)
	if ind.PublicBankConsecBuyDays != 0 {
		t.Errorf("expected PublicBankConsecBuyDays=0 (insufficient), got %d", ind.PublicBankConsecBuyDays)
	}
}

func TestEnrichGovernmentBroker_EmptyDirString(t *testing.T) {
	var ind PeriodIndicators
	c := NewCalculator()
	c.EnrichGovernmentBroker(&ind, "2026-07-25", "")
	if ind.PublicBankConsecBuyDays != 0 {
		t.Errorf("expected PublicBankConsecBuyDays=0 (empty dir), got %d", ind.PublicBankConsecBuyDays)
	}
}

func TestEnrichGovernmentBroker_TradingDateExcludes(t *testing.T) {
	dir := t.TempDir()
	// 5 valid dates spanning past + future
	dates := []string{"2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24", "2026-07-25"}
	for _, d := range dates {
		writeGovFlowDate(t, dir, d, "100")
	}

	var ind PeriodIndicators
	c := NewCalculator()
	// Use tradingDate=2026-07-23 — only 3 dates qualify (<= 2026-07-23), below MinDays=5.
	// Honest degradation: streak stays at 0 (unavailable per detector contract).
	c.EnrichGovernmentBroker(&ind, "2026-07-23", dir)
	if ind.PublicBankConsecBuyDays != 0 {
		t.Errorf("expected PublicBankConsecBuyDays=0 (insufficient after tradingDate filter), got %d", ind.PublicBankConsecBuyDays)
	}
}

func TestEnrichBatch3_Combines(t *testing.T) {
	sectorDir := t.TempDir()
	govDir := t.TempDir()

	// Setup sector (10 days, no rotation)
	for _, d := range []string{
		"2026-07-14", "2026-07-15", "2026-07-16", "2026-07-17", "2026-07-18",
		"2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24", "2026-07-25",
	} {
		writeSectorIndexFile(t, sectorDir, d, map[string]float64{
			"semiconductor": 1.0, "electronics": 1.0, "financials": 1.0, "steel": 1.0,
			"energy": 1.0, "cement": 1.0, "construction": 1.0, "shipping": 1.0,
		})
	}
	// Setup gov (5 days, all buy)
	for _, d := range []string{"2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24", "2026-07-25"} {
		writeGovFlowDate(t, govDir, d, "100")
	}

	var ind PeriodIndicators
	c := NewCalculator()
	c.EnrichBatch3(&ind, "2026-07-25", sectorDir, govDir)
	if ind.SectorRotationFlag {
		t.Errorf("expected SectorRotationFlag=false (no rotation), got true")
	}
	if ind.PublicBankConsecBuyDays != 5 {
		t.Errorf("expected PublicBankConsecBuyDays=5, got %d", ind.PublicBankConsecBuyDays)
	}
}

func TestEnrichBatch3_EmptyDirsNoOp(t *testing.T) {
	var ind PeriodIndicators
	c := NewCalculator()
	c.EnrichBatch3(&ind, "2026-07-25", "", "")
	if ind.SectorRotationFlag {
		t.Errorf("expected SectorRotationFlag=false (empty dirs), got true")
	}
	if ind.PublicBankConsecBuyDays != 0 {
		t.Errorf("expected PublicBankConsecBuyDays=0 (empty dirs), got %d", ind.PublicBankConsecBuyDays)
	}
}

// ── W3: Edge: legacy file with negative total_net on most-recent day ──

func TestEnrichGovernmentBroker_NegativeBreaksStreak(t *testing.T) {
	dir := t.TempDir()
	net := []string{"100", "200", "300", "100", "-50"}
	dates := []string{"2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24", "2026-07-25"}
	for i, d := range dates {
		writeGovFlowDate(t, dir, d, net[i])
	}

	var ind PeriodIndicators
	c := NewCalculator()
	c.EnrichGovernmentBroker(&ind, "2026-07-25", dir)
	// Most recent day is -50, so streak=0.
	if ind.PublicBankConsecBuyDays != 0 {
		t.Errorf("expected PublicBankConsecBuyDays=0 (most recent negative), got %d", ind.PublicBankConsecBuyDays)
	}
}
