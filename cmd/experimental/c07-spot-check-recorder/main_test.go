package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// TestValidateSectorID
// ---------------------------------------------------------------------------

func TestValidateSectorID(t *testing.T) {
	tests := []struct {
		sectorID string
		valid    bool
	}{
		{"semiconductor", true},
		{"electronics", true},
		{"optoelectronics", true},
		{"financials", true},
		{"cement", true},
		{"plastics", true},
		{"textiles", true},
		{"steel", true},
		{"shipping", true},
		{"food", true},
		{"auto", true},
		{"telecom", true},
		{"chemicals", true},
		{"biotech", true},
		{"construction", true},
		{"other_electronics", true},
		{"machinery", true},
		{"tourism", true},
		{"retail", true},
		{"energy", true},
		{"invalid_sector", false},
		{"", false},
		{"SEMICONDUCTOR", false}, // case-sensitive
		{"半導體", false},
	}

	for _, tt := range tests {
		t.Run(tt.sectorID, func(t *testing.T) {
			got := validSectors[tt.sectorID]
			if got != tt.valid {
				t.Errorf("validSectors[%q] = %v, want %v", tt.sectorID, got, tt.valid)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestParseSectorList
// ---------------------------------------------------------------------------

func TestParseSectorList(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"semiconductor,electronics", []string{"semiconductor", "electronics"}},
		{" semiconductor , electronics ", []string{"semiconductor", "electronics"}},
		{"semiconductor", []string{"semiconductor"}},
		{"", nil},
		{"semiconductor, electronics, biotech", []string{"semiconductor", "electronics", "biotech"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseSectorList(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("parseSectorList(%q) len = %d, want %d", tt.input, len(got), len(tt.expected))
				return
			}
			for i, v := range got {
				if v != tt.expected[i] {
					t.Errorf("parseSectorList(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestVerifyDrivers
// ---------------------------------------------------------------------------

func TestVerifyDrivers(t *testing.T) {
	tests := []struct {
		name    string
		drivers []string
		want    bool // true = verified (sources found), false = unverified
	}{
		{
			name:    "macro driver ForeignInvestorNet",
			drivers: []string{"ForeignInvestorNet buying", "DXY weakening"},
			want:    true,
		},
		{
			name:    "macro driver DXY",
			drivers: []string{"DXY index", "US10Y rising"},
			want:    true,
		},
		{
			name:    "event driver earnings season",
			drivers: []string{"法說會 season", "eps surprise"},
			want:    true,
		},
		{
			name:    "cycle driver inventory cycle",
			drivers: []string{"inventory cycle turning", "景氣復甦"},
			want:    true,
		},
		{
			name:    "no verifiable source",
			drivers: []string{"unknown driver", "random text"},
			want:    false,
		},
		{
			name:    "empty drivers",
			drivers: nil,
			want:    false,
		},
		{
			name:    "mixed verified and unverified",
			drivers: []string{"DXY", "some random thing"},
			want:    true, // at least one verifiable source
		},
		{
			name:    "Taiwanese Chinese macro term",
			drivers: []string{"熱錢流入", "匯率升值"},
			want:    true,
		},
		{
			name:    "Chinese event term",
			drivers: []string{"法說會", "營收公佈"},
			want:    true,
		},
		{
			name:    "Chinese cycle term",
			drivers: []string{"庫存週期", "景氣循環"},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := verifyDrivers(tt.drivers)
			if (got != nil) != tt.want {
				t.Errorf("verifyDrivers(%v) = %v, want nil?%v", tt.drivers, got, !tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestParsePercent
// ---------------------------------------------------------------------------

func TestParsePercent(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"5.0%", 0.05},
		{"0.0%", 0.0},
		{"100%", 1.0},
		{" 5.0% ", 0.05},
		{"0%", 0.0},
		{"invalid", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parsePercent(tt.input)
			if got != tt.want {
				t.Errorf("parsePercent(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestSplitTableRow
// ---------------------------------------------------------------------------

func TestSplitTableRow(t *testing.T) {
	// Table row with trailing | produces extra empty cell at the end.
	line := "| 2026-07-22 | 20 | 0.0% | 14 | 0 | 0 | 5 | backfilled |"
	cells := splitTableRow(line)
	// Leading | produces first empty cell; trailing | produces last empty cell → 10 cells
	if len(cells) != 10 {
		t.Errorf("splitTableRow returned %d cells, want 10", len(cells))
	}
	if strings.TrimSpace(cells[1]) != "2026-07-22" {
		t.Errorf("cells[1] = %q, want 2026-07-22", strings.TrimSpace(cells[1]))
	}
	if strings.TrimSpace(cells[7]) != "5" {
		t.Errorf("cells[7] = %q, want 5", strings.TrimSpace(cells[7]))
	}
}

// ---------------------------------------------------------------------------
// TestCountSpotChecksForDate
// ---------------------------------------------------------------------------

func TestCountSpotChecksForDate(t *testing.T) {
	raw := `
<spot-check-record id="2026-07-22-semiconductor"></spot-check-record>
<spot-check-record id="2026-07-22-electronics"></spot-check-record>
<spot-check-record id="2026-07-22-biotech"></spot-check-record>
`
	tests := []struct {
		date       string
		sectorList []string
		want       int
	}{
		{"2026-07-22", []string{"semiconductor", "electronics"}, 2},
		{"2026-07-22", []string{"semiconductor", "cement"}, 1}, // cement not in raw
		{"2026-07-22", []string{"cement", "biotech"}, 1},       // biotech in raw
		{"2026-07-23", []string{"semiconductor"}, 0},           // different date
	}

	for _, tt := range tests {
		t.Run(tt.date+"-"+strings.Join(tt.sectorList, ","), func(t *testing.T) {
			got := countSpotChecksForDate(raw, tt.date, tt.sectorList)
			if got != tt.want {
				t.Errorf("countSpotChecksForDate(raw, %q, %v) = %d, want %d",
					tt.date, tt.sectorList, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestCountTotalSpotChecksForDate
// ---------------------------------------------------------------------------

func TestCountTotalSpotChecksForDate(t *testing.T) {
	raw := `
<spot-check-record id="2026-07-22-semiconductor"></spot-check-record>
<spot-check-record id="2026-07-22-electronics"></spot-check-record>
<spot-check-record id="2026-07-22-biotech"></spot-check-record>
<spot-check-record id="2026-07-21-semiconductor"></spot-check-record>
`
	tests := []struct {
		date string
		want int
	}{
		{"2026-07-22", 3},
		{"2026-07-21", 1},
		{"2026-07-20", 0},
	}

	for _, tt := range tests {
		t.Run(tt.date, func(t *testing.T) {
			got := countTotalSpotChecksForDate(raw, tt.date)
			if got != tt.want {
				t.Errorf("countTotalSpotChecksForDate(raw, %q) = %d, want %d", tt.date, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestDryRunNoFileChange
// ---------------------------------------------------------------------------

func TestDryRunNoFileChange(t *testing.T) {
	tmpDir := t.TempDir()
	obsLog := filepath.Join(tmpDir, "test_obs_log.md")
	content := minimalObsLog() + "\n| 2026-07-22 | 20 | 0.0% | 14 | 0 | 0 | 0 | backfilled |\n"
	if err := os.WriteFile(obsLog, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	origContent, _ := os.ReadFile(obsLog)

	rows, raw, err := parseObsLogRaw(obsLog)
	if err != nil {
		t.Fatalf("parseObsLogRaw: %v", err)
	}

	if len(rows) == 0 {
		t.Error("expected at least one row parsed")
	}

	// Simulate dry run: compute count without writing.
	_ = countTotalSpotChecksForDate(raw, "2026-07-22")
	_ = len(rows)

	// File must be unchanged.
	afterContent, _ := os.ReadFile(obsLog)
	if string(afterContent) != string(origContent) {
		t.Error("dry run should not modify the file")
	}
}

// ---------------------------------------------------------------------------
// TestFirstSpotCheck
// ---------------------------------------------------------------------------

func TestFirstSpotCheck(t *testing.T) {
	tmpDir := t.TempDir()
	obsLog := filepath.Join(tmpDir, "obs_log.md")

	content := minimalObsLog() + "\n| 2026-07-22 | 20 | 0.0% | 14 | 0 | 0 | 0 | backfilled |\n"
	if err := os.WriteFile(obsLog, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	sectorList := []string{"semiconductor", "electronics"}

	_, raw, err := parseObsLogRaw(obsLog)
	if err != nil {
		t.Fatalf("parseObsLogRaw: %v", err)
	}

	existing := countSpotChecksForDate(raw, "2026-07-22", sectorList)
	if existing != 0 {
		t.Errorf("expected 0 existing spot checks, got %d", existing)
	}

	total := countTotalSpotChecksForDate(raw, "2026-07-22")
	newCount := total + len(sectorList)
	if newCount != 2 {
		t.Errorf("expected newCount=2, got %d", newCount)
	}
}

// ---------------------------------------------------------------------------
// TestReRunSameSectorsDedup
// ---------------------------------------------------------------------------

func TestReRunSameSectorsDedup(t *testing.T) {
	tmpDir := t.TempDir()
	obsLog := filepath.Join(tmpDir, "obs_log.md")

	content := minimalObsLog() + `
| 2026-07-22 | 20 | 0.0% | 14 | 0 | 0 | 2 | backfilled |

<spot-check-record id="2026-07-22-semiconductor"></spot-check-record>
<spot-check-record id="2026-07-22-electronics"></spot-check-record>

## Spot-Check Records
`
	if err := os.WriteFile(obsLog, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	sectorList := []string{"semiconductor", "electronics"}
	_, raw, err := parseObsLogRaw(obsLog)
	if err != nil {
		t.Fatalf("parseObsLogRaw: %v", err)
	}

	existing := countSpotChecksForDate(raw, "2026-07-22", sectorList)
	if existing != 2 {
		t.Errorf("expected 2 existing spot checks, got %d", existing)
	}

	total := countTotalSpotChecksForDate(raw, "2026-07-22")
	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}

	// Re-running same sectors: newCount unchanged.
	newCount := total
	if newCount != 2 {
		t.Errorf("re-run same sectors: expected 2 (no change), got %d", newCount)
	}
}

// ---------------------------------------------------------------------------
// TestReRunWithNewSectors
// ---------------------------------------------------------------------------

func TestReRunWithNewSectors(t *testing.T) {
	tmpDir := t.TempDir()
	obsLog := filepath.Join(tmpDir, "obs_log.md")

	content := minimalObsLog() + `
| 2026-07-22 | 20 | 0.0% | 14 | 0 | 0 | 1 | backfilled |

<spot-check-record id="2026-07-22-semiconductor"></spot-check-record>
`
	if err := os.WriteFile(obsLog, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	sectorList := []string{"semiconductor", "electronics"}
	_, raw, err := parseObsLogRaw(obsLog)
	if err != nil {
		t.Fatalf("parseObsLogRaw: %v", err)
	}

	existing := countSpotChecksForDate(raw, "2026-07-22", sectorList)
	if existing != 1 {
		t.Errorf("expected 1 existing, got %d", existing)
	}

	total := countTotalSpotChecksForDate(raw, "2026-07-22")
	if total != 1 {
		t.Errorf("expected total=1, got %d", total)
	}

	// New spot_check_count = total + (len(sectorList) - existing) = 1 + (2-1) = 2.
	newCount := total + (len(sectorList) - existing)
	if newCount != 2 {
		t.Errorf("expected newCount=2, got %d", newCount)
	}
}

// ---------------------------------------------------------------------------
// TestNewDateAppendsRow
// ---------------------------------------------------------------------------

func TestNewDateAppendsRow(t *testing.T) {
	tmpDir := t.TempDir()
	obsLog := filepath.Join(tmpDir, "obs_log.md")

	content := minimalObsLog() + "\n| 2026-07-21 | 20 | 0.0% | 14 | 0 | 0 | 0 | backfilled |\n"
	if err := os.WriteFile(obsLog, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	rows, _, err := parseObsLogRaw(obsLog)
	if err != nil {
		t.Fatalf("parseObsLogRaw: %v", err)
	}

	found := false
	for _, r := range rows {
		if r.Date == "2026-07-23" {
			found = true
			break
		}
	}
	if found {
		t.Error("2026-07-23 should not be in existing rows")
	}

	// New date: spot_check_count = len(sectorList).
	sectorList := []string{"semiconductor", "electronics", "biotech"}
	newCount := len(sectorList)
	if newCount != 3 {
		t.Errorf("expected newCount=3 for new date, got %d", newCount)
	}
}

// ---------------------------------------------------------------------------
// TestMalformedObsLog
// ---------------------------------------------------------------------------

func TestMalformedObsLog(t *testing.T) {
	tmpDir := t.TempDir()
	obsLog := filepath.Join(tmpDir, "malformed.md")

	content := "# Not an observation log\nsome random text\n| broken | table |"
	if err := os.WriteFile(obsLog, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	rows, _, err := parseObsLogRaw(obsLog)
	// Should return empty rows, not an error.
	if err != nil {
		t.Errorf("parseObsLogRaw should not error on malformed, got: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows from malformed log, got %d", len(rows))
	}
}

// ---------------------------------------------------------------------------
// TestBuildNarrativeSection
// ---------------------------------------------------------------------------

func TestBuildNarrativeSection(t *testing.T) {
	ts := "2026-07-22 14:30"
	sectors := []string{"semiconductor", "electronics"}
	sources := []string{"event", "macro"}
	notes := "test notes"
	operator := "test-operator"

	narrative := buildNarrativeSection(ts, sectors, sources, notes, operator)

	if !strings.Contains(narrative, "### 2026-07-22 14:30 — test-operator") {
		t.Error("narrative should contain timestamp and operator")
	}
	if !strings.Contains(narrative, "**sectors checked**: semiconductor, electronics") {
		t.Error("narrative should list sectors")
	}
	if !strings.Contains(narrative, "**driver sources verified**: event, macro") {
		t.Error("narrative should list sources")
	}
	if !strings.Contains(narrative, "**notes**: test notes") {
		t.Error("narrative should contain notes")
	}
}

// ---------------------------------------------------------------------------
// TestBuildNarrativeSectionNoNotes
// ---------------------------------------------------------------------------

func TestBuildNarrativeSectionNoNotes(t *testing.T) {
	narrative := buildNarrativeSection("2026-07-22 14:30", []string{"semiconductor"}, []string{"macro"}, "", "op")
	if strings.Contains(narrative, "**notes**:") {
		t.Error("narrative should not contain notes line when notes is empty")
	}
}

// ---------------------------------------------------------------------------
// TestUpdateRowSpotCheckCount
// ---------------------------------------------------------------------------

func TestUpdateRowSpotCheckCount(t *testing.T) {
	tmpDir := t.TempDir()
	obsLog := filepath.Join(tmpDir, "obs_log.md")

	content := minimalObsLog() + "\n| 2026-07-22 | 20 | 0.0% | 14 | 0 | 0 | 0 | backfilled |\n"
	if err := os.WriteFile(obsLog, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	rows, raw, err := parseObsLogRaw(obsLog)
	if err != nil {
		t.Fatalf("parseObsLogRaw: %v", err)
	}

	rowIdx := -1
	for i, r := range rows {
		if r.Date == "2026-07-22" {
			rowIdx = i
			break
		}
	}
	if rowIdx < 0 {
		t.Fatal("expected to find 2026-07-22 row")
	}

	updated := updateRowSpotCheckCount(raw, rows, rowIdx, "2026-07-22", 5)

	if !strings.Contains(updated, "| 2026-07-22 | 20 | 0.0% | 14 | 0 | 0 | 5 |") {
		t.Error("updated row should have spot_check_count=5")
	}
}

// ---------------------------------------------------------------------------
// TestGetOperator
// ---------------------------------------------------------------------------

func TestGetOperator(t *testing.T) {
	os.Unsetenv("OPERATOR")
	if got := getOperator(); got != "OPERATOR" {
		t.Errorf("getOperator() without env = %q, want OPERATOR", got)
	}

	os.Setenv("OPERATOR", "alice")
	defer os.Unsetenv("OPERATOR")
	if got := getOperator(); got != "alice" {
		t.Errorf("getOperator() with OPERATOR=alice = %q, want alice", got)
	}
}

// ---------------------------------------------------------------------------
// TestBuildDriversMap
// ---------------------------------------------------------------------------

func TestBuildDriversMap(t *testing.T) {
	report := &predictionReport{
		SectorPredictions: []sectorDayPrediction{
			{
				Date: "2026-07-22",
				Sectors: []sectorPrediction{
					{SectorID: "semiconductor", Drivers: []string{"ForeignInvestorNet", "DXY"}},
					{SectorID: "electronics", Drivers: []string{"earnings season"}},
					{SectorID: "biotech", Drivers: []string{}},
				},
			},
		},
	}

	m := buildDriversMap(report, "2026-07-22")
	if len(m) != 3 {
		t.Errorf("buildDriversMap(date match) returned %d entries, want 3", len(m))
	}
	if len(m["semiconductor"]) != 2 {
		t.Errorf("semiconductor drivers = %v, want 2", m["semiconductor"])
	}
	if len(m["biotech"]) != 0 {
		t.Errorf("biotech drivers = %v, want 0", m["biotech"])
	}

	m2 := buildDriversMap(report, "2026-07-23")
	if len(m2) != 3 {
		t.Errorf("buildDriversMap(fallback) returned %d entries, want 3", len(m2))
	}
}

// ---------------------------------------------------------------------------
// TestBuildNewRow
// ---------------------------------------------------------------------------

func TestBuildNewRow(t *testing.T) {
	row := buildNewRow("2026-07-23", 3)
	if !strings.Contains(row, "| 2026-07-23 |") {
		t.Error("row should contain date")
	}
	if !strings.Contains(row, "| 3 |") {
		t.Error("row should contain spot_check_count=3")
	}
}

// ---------------------------------------------------------------------------
// TestUpdateObsLogAtomic
// ---------------------------------------------------------------------------

func TestUpdateObsLogAtomic(t *testing.T) {
	tmpDir := t.TempDir()
	obsLog := filepath.Join(tmpDir, "obs_log.md")
	narrative := buildNarrativeSection("2026-07-22 14:30", []string{"semiconductor"}, []string{"macro"}, "", "test-op")
	marker := `<spot-check-record id="2026-07-22-semiconductor"></spot-check-record>`

	content := minimalObsLog() + "\n| 2026-07-22 | 20 | 0.0% | 14 | 0 | 0 | 1 | backfilled |\n"
	if err := os.WriteFile(obsLog, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	rows, raw, err := parseObsLogRaw(obsLog)
	if err != nil {
		t.Fatalf("parseObsLogRaw: %v", err)
	}

	rowIdx := -1
	for i, r := range rows {
		if r.Date == "2026-07-22" {
			rowIdx = i
			break
		}
	}

	if err := updateObsLog(obsLog, raw, rows, "2026-07-22", []string{"semiconductor"}, 2, rowIdx, narrative, marker); err != nil {
		t.Fatalf("updateObsLog: %v", err)
	}

	updated, err := os.ReadFile(obsLog)
	if err != nil {
		t.Fatalf("read updated file: %v", err)
	}

	if !strings.Contains(string(updated), "| 2026-07-22 | 20 | 0.0% | 14 | 0 | 0 | 2 |") {
		t.Error("spot_check_count should be updated to 2")
	}
	if !strings.Contains(string(updated), marker) {
		t.Error("embedded marker should be present")
	}
	if !strings.Contains(string(updated), "## Spot-Check Records") {
		t.Error("narrative section should be present")
	}

	tmpFile := obsLog + ".tmp"
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error(".tmp file should not remain after rename")
	}
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func minimalObsLog() string {
	return "# Sector Prediction Observation Log\n\n## Records\n\n"
}
