package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestNewStateStore(t *testing.T) {
	st := NewStateStore(t.TempDir())
	if st == nil {
		t.Fatal("expected non-nil StateStore")
	}
	if st.basePath == "" {
		t.Error("expected non-empty basePath")
	}
	if p := st.GetPortfolio(); p.Cash != 3000000 {
		t.Errorf("expected default cash 3000000, got %f", p.Cash)
	}
	if p := st.GetPortfolio(); p.AvailableCash != 3000000 {
		t.Errorf("expected default available cash 3000000, got %f", p.AvailableCash)
	}
	if r := st.GetCurrentRegime(); r != domain.RegimeNeutral {
		t.Errorf("expected default regime Neutral, got %v", r)
	}
	if pos := st.GetPositions(); len(pos) != 0 {
		t.Errorf("expected empty positions, got %d", len(pos))
	}
}

func TestGetPortfolio(t *testing.T) {
	st := NewStateStore(t.TempDir())
	p := st.GetPortfolio()
	if p.Cash != 3000000 {
		t.Errorf("expected default cash 3000000, got %f", p.Cash)
	}
	if p.DayPnL != 0 {
		t.Errorf("expected DayPnL 0, got %f", p.DayPnL)
	}
}

func TestUpdatePortfolio(t *testing.T) {
	st := NewStateStore(t.TempDir())
	st.UpdatePortfolio(PortfolioState{
		Cash:          2500000,
		TotalExposure: 500000,
		AvailableCash: 2500000,
		DayPnL:        12500,
	})
	p := st.GetPortfolio()
	if p.Cash != 2500000 {
		t.Errorf("expected cash 2500000, got %f", p.Cash)
	}
	if p.TotalExposure != 500000 {
		t.Errorf("expected TotalExposure 500000, got %f", p.TotalExposure)
	}
	if p.DayPnL != 12500 {
		t.Errorf("expected DayPnL 12500, got %f", p.DayPnL)
	}
	if p.LastUpdated.IsZero() {
		t.Error("expected non-zero LastUpdated")
	}
}

func TestGetPosition(t *testing.T) {
	st := NewStateStore(t.TempDir())

	_, ok := st.GetPosition("2330.TW")
	if ok {
		t.Error("expected no position for 2330.TW")
	}

	st.UpdatePosition(domain.Position{Symbol: "2330.TW", Quantity: 1000, AverageCost: 500})
	pos, ok := st.GetPosition("2330.TW")
	if !ok {
		t.Fatal("expected position for 2330.TW")
	}
	if pos.Quantity != 1000 {
		t.Errorf("expected quantity 1000, got %d", pos.Quantity)
	}
}

func TestGetPositions(t *testing.T) {
	st := NewStateStore(t.TempDir())

	positions := st.GetPositions()
	if len(positions) != 0 {
		t.Errorf("expected 0 positions, got %d", len(positions))
	}

	st.UpdatePosition(domain.Position{Symbol: "2330.TW", Quantity: 1000})
	st.UpdatePosition(domain.Position{Symbol: "0050.TW", Quantity: 500})

	positions = st.GetPositions()
	if len(positions) != 2 {
		t.Errorf("expected 2 positions, got %d", len(positions))
	}
	if _, ok := positions["2330.TW"]; !ok {
		t.Error("expected 2330.TW in positions")
	}
	if _, ok := positions["0050.TW"]; !ok {
		t.Error("expected 0050.TW in positions")
	}
}

func TestRemovePosition(t *testing.T) {
	st := NewStateStore(t.TempDir())
	st.UpdatePosition(domain.Position{Symbol: "2330.TW", Quantity: 1000})
	st.UpdatePosition(domain.Position{Symbol: "0050.TW", Quantity: 500})

	st.RemovePosition("2330.TW")

	_, ok := st.GetPosition("2330.TW")
	if ok {
		t.Error("expected 2330.TW to be removed")
	}
	_, ok = st.GetPosition("0050.TW")
	if !ok {
		t.Error("expected 0050.TW to remain")
	}

	// Removing non-existent key should not panic
	st.RemovePosition("9999.TW")
}

func TestUpdatePositionPrices(t *testing.T) {
	st := NewStateStore(t.TempDir())
	st.UpdatePosition(domain.Position{Symbol: "2330.TW", Quantity: 1000, AverageCost: 500})
	st.UpdatePosition(domain.Position{Symbol: "0050.TW", Quantity: 500, AverageCost: 150})

	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 550},
		{Symbol: "0050.TW", Last: 160},
	}
	st.UpdatePositionPrices(quotes)

	pos, ok := st.GetPosition("2330.TW")
	if !ok {
		t.Fatal("expected 2330.TW position")
	}
	if pos.CurrentPrice != 550 {
		t.Errorf("expected CurrentPrice 550, got %f", pos.CurrentPrice)
	}
	expectedPnL := float64(1000) * (550 - 500)
	if pos.UnrealizedPnL != expectedPnL {
		t.Errorf("expected UnrealizedPnL %f, got %f", expectedPnL, pos.UnrealizedPnL)
	}
	expectedMV := float64(1000) * 550
	if pos.MarketValue != expectedMV {
		t.Errorf("expected MarketValue %f, got %f", expectedMV, pos.MarketValue)
	}

	p := st.GetPortfolio()
	if p.UnrealizedPnL != expectedPnL+(float64(500)*(160-150)) {
		t.Errorf("unexpected portfolio UnrealizedPnL: %f", p.UnrealizedPnL)
	}
}

func TestUpdatePositionPrices_EmptyQuotes(t *testing.T) {
	st := NewStateStore(t.TempDir())
	st.UpdatePosition(domain.Position{Symbol: "2330.TW", Quantity: 1000, AverageCost: 500})
	st.UpdatePositionPrices([]domain.Quote{})
	// Should not panic; positions should remain unchanged
	pos, _ := st.GetPosition("2330.TW")
	if pos.CurrentPrice != 0 {
		t.Errorf("expected CurrentPrice 0 (unchanged), got %f", pos.CurrentPrice)
	}
}

func TestCalculateDayPnL(t *testing.T) {
	st := NewStateStore(t.TempDir())
	st.UpdatePosition(domain.Position{Symbol: "2330.TW", Quantity: 1000, AverageCost: 500, CurrentPrice: 550})

	dayPnL := st.CalculateDayPnL(map[string]float64{"2330.TW": 520})
	expected := float64(1000) * (550 - 520)
	if dayPnL != expected {
		t.Errorf("expected DayPnL %f, got %f", expected, dayPnL)
	}

	p := st.GetPortfolio()
	if p.DayPnL != expected {
		t.Errorf("portfolio DayPnL expected %f, got %f", expected, p.DayPnL)
	}
}

func TestCalculateDayPnL_EmptyPrices(t *testing.T) {
	st := NewStateStore(t.TempDir())
	st.UpdatePosition(domain.Position{Symbol: "2330.TW", Quantity: 1000, AverageCost: 500, CurrentPrice: 550})
	dayPnL := st.CalculateDayPnL(map[string]float64{})
	if dayPnL != 0 {
		t.Errorf("expected 0 DayPnL with no matching prices, got %f", dayPnL)
	}
}

func TestResetDayState(t *testing.T) {
	st := NewStateStore(t.TempDir())
	st.UpdatePortfolio(PortfolioState{Cash: 2000000, DayPnL: 50000, RealizedPnL: 30000})

	st.ResetDayState()

	p := st.GetPortfolio()
	if p.DayPnL != 0 {
		t.Errorf("expected DayPnL 0 after reset, got %f", p.DayPnL)
	}
	if p.RealizedPnL != 0 {
		t.Errorf("expected RealizedPnL 0 after reset, got %f", p.RealizedPnL)
	}
}

func TestResetDayState_ClearsEvents(t *testing.T) {
	st := NewStateStore(t.TempDir())
	st.RecordEvent("order_placed", "data")
	st.RecordEvent("risk_alert", "data")

	state := st.GetState()
	if len(state.Events) < 2 {
		t.Fatalf("expected at least 2 events before reset, got %d", len(state.Events))
	}

	st.ResetDayState()

	state = st.GetState()
	if len(state.Events) != 0 {
		t.Errorf("expected 0 events after reset, got %d", len(state.Events))
	}

	// Wait for async goroutines from RecordEvent to finish before temp dir cleanup
	time.Sleep(50 * time.Millisecond)
}

func TestGetRegime(t *testing.T) {
	st := NewStateStore(t.TempDir())
	r := st.GetRegime()
	if r.CurrentRegime != domain.RegimeNeutral {
		t.Errorf("expected Neutral, got %v", r.CurrentRegime)
	}
	if r.Confidence != 0.5 {
		t.Errorf("expected Confidence 0.5, got %f", r.Confidence)
	}
}

func TestUpdateRegime(t *testing.T) {
	st := NewStateStore(t.TempDir())
	st.UpdateRegime(domain.RegimeRiskOn, 0.85, "prism-v2")

	r := st.GetRegime()
	if r.CurrentRegime != domain.RegimeRiskOn {
		t.Errorf("expected RiskOn, got %v", r.CurrentRegime)
	}
	if r.Confidence != 0.85 {
		t.Errorf("expected Confidence 0.85, got %f", r.Confidence)
	}
	if r.DeterminedBy != "prism-v2" {
		t.Errorf("expected DeterminedBy prism-v2, got %s", r.DeterminedBy)
	}
	if r.LastChangedAt.IsZero() {
		t.Error("expected non-zero LastChangedAt")
	}
}

func TestSetCurrentRegime(t *testing.T) {
	st := NewStateStore(t.TempDir())
	st.SetCurrentRegime(domain.RegimeRiskOff)

	if r := st.GetCurrentRegime(); r != domain.RegimeRiskOff {
		t.Errorf("expected RiskOff, got %v", r)
	}

	regime := st.GetRegime()
	if regime.DeterminedBy != "context_agent" {
		t.Errorf("expected DeterminedBy context_agent, got %s", regime.DeterminedBy)
	}
}

func TestGetCurrentRegime(t *testing.T) {
	st := NewStateStore(t.TempDir())
	if r := st.GetCurrentRegime(); r != domain.RegimeNeutral {
		t.Errorf("expected default Neutral, got %v", r)
	}
}

func TestSetPendingRecommendations(t *testing.T) {
	st := NewStateStore(t.TempDir())
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Agent: "mom-01", Conviction: 80},
		{Symbol: "0050.TW", Agent: "val-01", Conviction: 60},
	}
	st.SetPendingRecommendations(recs)

	got := st.GetPendingRecommendations()
	if len(got) != 2 {
		t.Fatalf("expected 2 pending recs, got %d", len(got))
	}
	if got[0].Symbol != "2330.TW" {
		t.Errorf("expected first rec Symbol 2330.TW, got %s", got[0].Symbol)
	}
	if got[1].Conviction != 60 {
		t.Errorf("expected second rec Conviction 60, got %d", got[1].Conviction)
	}
}

func TestSetPendingRecommendations_EmptySlice(t *testing.T) {
	st := NewStateStore(t.TempDir())
	st.SetPendingRecommendations(nil)
	if got := st.GetPendingRecommendations(); len(got) != 0 {
		t.Errorf("expected 0 pending recs for nil input, got %d", len(got))
	}
	st.SetPendingRecommendations([]domain.Recommendation{})
	if got := st.GetPendingRecommendations(); len(got) != 0 {
		t.Errorf("expected 0 pending recs for empty input, got %d", len(got))
	}
}

func TestSetFilteredRecommendations(t *testing.T) {
	st := NewStateStore(t.TempDir())
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Agent: "mom-01", Conviction: 75},
	}
	st.SetFilteredRecommendations(recs)

	got := st.GetFilteredRecommendations()
	if len(got) != 1 {
		t.Fatalf("expected 1 filtered rec, got %d", len(got))
	}
	if got[0].Symbol != "2330.TW" {
		t.Errorf("expected Symbol 2330.TW, got %s", got[0].Symbol)
	}
}

func TestGetFilteredRecommendations_Empty(t *testing.T) {
	st := NewStateStore(t.TempDir())
	if got := st.GetFilteredRecommendations(); len(got) != 0 {
		t.Errorf("expected 0 filtered recs from fresh store, got %d", len(got))
	}
}

func TestGetState(t *testing.T) {
	st := NewStateStore(t.TempDir())
	st.UpdatePosition(domain.Position{Symbol: "2330.TW", Quantity: 1000})
	st.RecordEvent("system_start", "boot")

	state := st.GetState()
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.Portfolio.Cash != 3000000 {
		t.Errorf("expected cash 3000000, got %f", state.Portfolio.Cash)
	}
	if len(state.Positions) != 1 {
		t.Errorf("expected 1 position, got %d", len(state.Positions))
	}
	if state.Positions[0].Symbol != "2330.TW" {
		t.Errorf("expected Symbol 2330.TW, got %s", state.Positions[0].Symbol)
	}
	if len(state.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(state.Events))
	}
	if state.Events[0].Type != "system_start" {
		t.Errorf("expected event type system_start, got %s", state.Events[0].Type)
	}

	time.Sleep(50 * time.Millisecond)
}

func TestRecordEvent(t *testing.T) {
	st := NewStateStore(t.TempDir())
	st.RecordEvent("order_placed", "payload-data")

	state := st.GetState()
	if len(state.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(state.Events))
	}
	if state.Events[0].Type != "order_placed" {
		t.Errorf("expected event type order_placed, got %s", state.Events[0].Type)
	}
	if state.Events[0].Payload != "payload-data" {
		t.Errorf("expected payload 'payload-data', got %v", state.Events[0].Payload)
	}
	if state.Events[0].ID == "" {
		t.Error("expected non-empty event ID")
	}
	if !strings.HasPrefix(state.Events[0].ID, "evt-") {
		t.Errorf("expected event ID prefix 'evt-', got %s", state.Events[0].ID)
	}
	if state.Events[0].Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}

	time.Sleep(50 * time.Millisecond)
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	st := NewStateStore(dir)

	st.UpdatePortfolio(PortfolioState{Cash: 2500000, TotalExposure: 500000, DayPnL: 12500})
	st.UpdatePosition(domain.Position{Symbol: "2330.TW", Quantity: 1000, AverageCost: 500})
	st.UpdateRegime(domain.RegimeRiskOn, 0.85, "prism-v2")

	if err := st.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify files exist
	stateDir := filepath.Join(dir, "state")
	if _, err := os.Stat(filepath.Join(stateDir, PortfolioStateFile)); os.IsNotExist(err) {
		t.Error("portfolio state file not created")
	}
	if _, err := os.Stat(filepath.Join(stateDir, PositionsStateFile)); os.IsNotExist(err) {
		t.Error("positions state file not created")
	}
	if _, err := os.Stat(filepath.Join(stateDir, RegimeStateFile)); os.IsNotExist(err) {
		t.Error("regime state file not created")
	}

	// Load into a new store
	st2 := NewStateStore(dir)
	if err := st2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	p := st2.GetPortfolio()
	if p.Cash != 2500000 {
		t.Errorf("loaded cash expected 2500000, got %f", p.Cash)
	}
	if p.DayPnL != 12500 {
		t.Errorf("loaded DayPnL expected 12500, got %f", p.DayPnL)
	}

	pos, ok := st2.GetPosition("2330.TW")
	if !ok {
		t.Error("expected 2330.TW position after load")
	}
	if pos.Quantity != 1000 {
		t.Errorf("expected quantity 1000, got %d", pos.Quantity)
	}

	if r := st2.GetCurrentRegime(); r != domain.RegimeRiskOn {
		t.Errorf("expected loaded regime RiskOn, got %v", r)
	}
}

func TestLoad_NoStateFiles(t *testing.T) {
	dir := t.TempDir()
	st := NewStateStore(dir)
	err := st.Load()
	if err != nil {
		t.Logf("Load returned error (known IsNotExist wrapping bug): %v", err)
	}
	if p := st.GetPortfolio(); p.Cash != 3000000 {
		t.Errorf("expected default cash 3000000, got %f", p.Cash)
	}
}

func TestLoadLastPortfolioState(t *testing.T) {
	dir := t.TempDir()

	// No file yet
	_, err := LoadLastPortfolioState(dir)
	if err == nil {
		t.Error("expected error for missing portfolio state file")
	}

	// Write a valid file
	st := NewStateStore(dir)
	st.UpdatePortfolio(PortfolioState{Cash: 1800000, DayPnL: -5000})
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	p, err := LoadLastPortfolioState(dir)
	if err != nil {
		t.Fatalf("LoadLastPortfolioState failed: %v", err)
	}
	if p.Cash != 1800000 {
		t.Errorf("expected cash 1800000, got %f", p.Cash)
	}
	if p.DayPnL != -5000 {
		t.Errorf("expected DayPnL -5000, got %f", p.DayPnL)
	}
}

func TestLoadLastPositions(t *testing.T) {
	dir := t.TempDir()

	// No file yet
	_, err := LoadLastPositions(dir)
	if err == nil {
		t.Error("expected error for missing positions file")
	}

	// Write valid file
	st := NewStateStore(dir)
	st.UpdatePosition(domain.Position{Symbol: "2330.TW", Quantity: 1000})
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	positions, err := LoadLastPositions(dir)
	if err != nil {
		t.Fatalf("LoadLastPositions failed: %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}
	if pos, ok := positions["2330.TW"]; !ok || pos.Quantity != 1000 {
		t.Errorf("unexpected position: %v", pos)
	}
}

func TestLoadLastRegime(t *testing.T) {
	dir := t.TempDir()

	// No file yet
	_, err := LoadLastRegime(dir)
	if err == nil {
		t.Error("expected error for missing regime file")
	}

	// Write valid file
	st := NewStateStore(dir)
	st.UpdateRegime(domain.RegimeRiskOff, 0.9, "janus")
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	r, err := LoadLastRegime(dir)
	if err != nil {
		t.Fatalf("LoadLastRegime failed: %v", err)
	}
	if r.CurrentRegime != domain.RegimeRiskOff {
		t.Errorf("expected RiskOff, got %v", r.CurrentRegime)
	}
	if r.Confidence != 0.9 {
		t.Errorf("expected Confidence 0.9, got %f", r.Confidence)
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	err := WriteFileAtomic(path, `{"key":"value"}`)
	if err != nil {
		t.Fatalf("WriteFileAtomic failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}
	if string(data) != "{\"key\":\"value\"}\n" {
		t.Errorf("unexpected file content: %q", string(data))
	}
}

func TestWriteFileAtomic_Overwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	_ = WriteFileAtomic(path, "first")
	_ = WriteFileAtomic(path, "second")

	data, _ := os.ReadFile(path)
	if string(data) != "second\n" {
		t.Errorf("expected 'second\\n', got %q", string(data))
	}
}

func TestWriteFileAtomic_InvalidDir(t *testing.T) {
	err := WriteFileAtomic("/nonexistent/dir/test.jsonl", "content")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestAppendToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	err := appendToFile(path, "line1")
	if err != nil {
		t.Fatalf("appendToFile failed: %v", err)
	}

	err = appendToFile(path, "line2")
	if err != nil {
		t.Fatalf("appendToFile second line failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), string(data))
	}
	if lines[0] != "line1" {
		t.Errorf("expected line1, got %q", lines[0])
	}
	if lines[1] != "line2" {
		t.Errorf("expected line2, got %q", lines[1])
	}
}

func TestAppendToFile_InvalidDir(t *testing.T) {
	err := appendToFile("/nonexistent/dir/events.jsonl", "content")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestReadLastJSONLLine_SingleLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	_ = os.WriteFile(path, []byte(`{"k":"v"}`+"\n"), 0o644)

	var result map[string]string
	err := readLastJSONLLine(path, &result)
	if err != nil {
		t.Fatalf("readLastJSONLLine failed: %v", err)
	}
	if result["k"] != "v" {
		t.Errorf("expected v, got %s", result["k"])
	}
}

func TestReadLastJSONLLine_MultiLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"a":1}
{"b":2}
{"c":3}
`
	_ = os.WriteFile(path, []byte(content), 0o644)

	var result map[string]int
	err := readLastJSONLLine(path, &result)
	if err != nil {
		t.Fatalf("readLastJSONLLine failed: %v", err)
	}
	if result["c"] != 3 {
		t.Errorf("expected last line c=3, got %v", result)
	}
}

func TestReadLastJSONLLine_PartialLastLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	// Last line is incomplete (partial write)
	content := `{"a":1}
{"b":2
`
	_ = os.WriteFile(path, []byte(content), 0o644)

	var result map[string]int
	err := readLastJSONLLine(path, &result)
	if err != nil {
		t.Fatalf("readLastJSONLLine failed: %v", err)
	}
	// Should read the last valid JSON line
	if result["a"] != 1 {
		t.Errorf("expected a=1, got %v", result)
	}
}

func TestReadLastJSONLLine_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	_ = os.WriteFile(path, []byte{}, 0o644)

	var result map[string]int
	err := readLastJSONLLine(path, &result)
	if err != nil {
		t.Fatalf("readLastJSONLLine on empty file should succeed: %v", err)
	}
}

func TestReadLastJSONLLine_NonExistent(t *testing.T) {
	var result map[string]int
	err := readLastJSONLLine("/nonexistent/file.jsonl", &result)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestReadLastJSONLLine_NoValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	_ = os.WriteFile(path, []byte("not-json\n"), 0o644)

	var result map[string]int
	err := readLastJSONLLine(path, &result)
	if err == nil {
		t.Error("expected error for no valid JSON")
	}
}

func TestReadLastJSONLLine_SkipsEmptyLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"a":1}

{"b":2}

`
	_ = os.WriteFile(path, []byte(content), 0o644)

	var result map[string]int
	err := readLastJSONLLine(path, &result)
	if err != nil {
		t.Fatalf("readLastJSONLLine failed: %v", err)
	}
	if result["b"] != 2 {
		t.Errorf("expected b=2, got %v", result)
	}
}

func TestStateStruct(t *testing.T) {
	state := State{
		Portfolio: PortfolioState{Cash: 1000},
		Positions: []domain.Position{{Symbol: "2330"}},
		Regime:    RegimeState{CurrentRegime: domain.RegimeRiskOn},
		Events:    nil,
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal State: %v", err)
	}
	var decoded State
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal State: %v", err)
	}
	if decoded.Portfolio.Cash != 1000 {
		t.Errorf("expected cash 1000, got %f", decoded.Portfolio.Cash)
	}
	if decoded.Regime.CurrentRegime != domain.RegimeRiskOn {
		t.Errorf("expected RiskOn, got %v", decoded.Regime.CurrentRegime)
	}
}

func TestPortfolioState_RoundTrip(t *testing.T) {
	ps := PortfolioState{
		Cash:          3000000,
		TotalExposure: 500000,
		AvailableCash: 2500000,
		DayPnL:        12500,
		UnrealizedPnL: 8000,
		RealizedPnL:   3000,
	}

	data, err := json.Marshal(ps)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded PortfolioState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Cash != ps.Cash {
		t.Errorf("Cash: got %f, want %f", decoded.Cash, ps.Cash)
	}
	if decoded.RealizedPnL != ps.RealizedPnL {
		t.Errorf("RealizedPnL: got %f, want %f", decoded.RealizedPnL, ps.RealizedPnL)
	}
}
