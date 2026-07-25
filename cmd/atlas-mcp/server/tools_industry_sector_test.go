package server

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// newSectorTestServer creates a minimal *server with a file-backed audit writer.
// Sector handlers do not call the HTTP client, so only cfg and audit are needed.
func newSectorTestServer(t *testing.T) (*server, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")
	audit, err := NewAuditWriter(auditPath)
	if err != nil {
		t.Fatalf("audit writer: %v", err)
	}

	s := &server{
		cfg:   Config{AuditLogPath: auditPath},
		audit: audit,
	}
	cleanup := func() { _ = audit.Close() }
	return s, cleanup
}

func TestHandleSectorList_ReturnsAllSectors(t *testing.T) {
	s, done := newSectorTestServer(t)
	defer done()

	_, out, err := s.handleSectorList(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handleSectorList: %v", err)
	}
	if len(out.Sectors) != 38 {
		t.Fatalf("expected 38 sectors, got %d", len(out.Sectors))
	}

	seen := make(map[string]struct{}, len(out.Sectors))
	for _, sec := range out.Sectors {
		if sec.ID == "" {
			t.Fatal("sector ID must not be empty")
		}
		if sec.StockSymbol == nil {
			t.Fatalf("sector %s: StockSymbol slice must be non-nil", sec.ID)
		}
		if _, dup := seen[sec.ID]; dup {
			t.Fatalf("duplicate sector ID: %s", sec.ID)
		}
		seen[sec.ID] = struct{}{}
	}
}

func TestHandleSectorList_L1HaveChineseDisplay(t *testing.T) {
	s, done := newSectorTestServer(t)
	defer done()

	_, out, err := s.handleSectorList(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handleSectorList: %v", err)
	}

	l1Count := 0
	for _, sec := range out.Sectors {
		id := industry.SectorID(sec.ID)
		if !id.IsL1() {
			continue
		}
		l1Count++
		if sec.DisplayZH == "" {
			t.Fatalf("L1 sector %s: missing Chinese display label", sec.ID)
		}
		if sec.DisplayZH == sec.ID {
			t.Fatalf("L1 sector %s: display label should not be the canonical ID", sec.ID)
		}
	}
	if l1Count != 20 {
		t.Fatalf("expected 20 L1 sectors, got %d", l1Count)
	}
}

func TestHandleSectorList_L2FallBackToID(t *testing.T) {
	s, done := newSectorTestServer(t)
	defer done()

	_, out, err := s.handleSectorList(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handleSectorList: %v", err)
	}

	l2Count := 0
	for _, sec := range out.Sectors {
		id := industry.SectorID(sec.ID)
		if !id.IsL2() {
			continue
		}
		l2Count++
		// P3-2: L2 sub-industries now have Chinese labels — verify non-empty, non-ID
		if sec.DisplayZH == "" {
			t.Fatalf("L2 sector %s: DisplayZH is empty", sec.ID)
		}
		if sec.DisplayZH == sec.ID {
			t.Fatalf("L2 sector %s: DisplayZH should be Chinese label, got raw ID %q", sec.ID, sec.DisplayZH)
		}
	}
	if l2Count != 18 {
		t.Fatalf("expected 18 L2 sectors, got %d", l2Count)
	}
}

func TestHandleSectorLookup_BySymbol_2330(t *testing.T) {
	s, done := newSectorTestServer(t)
	defer done()

	_, out, err := s.handleSectorLookup(context.Background(), nil, SectorLookupInput{Symbol: "2330"})
	if err != nil {
		t.Fatalf("handleSectorLookup: %v", err)
	}
	if !out.Found {
		t.Fatal("expected Found=true for symbol 2330")
	}
	if out.Sector == nil {
		t.Fatal("expected sector info")
	}
	if out.Sector.ID != string(industry.SectorSemiconductor) {
		t.Fatalf("expected %s, got %s", industry.SectorSemiconductor, out.Sector.ID)
	}
	if out.Sector.DisplayZH != "半導體" {
		t.Fatalf("expected 半導體, got %s", out.Sector.DisplayZH)
	}
}

func TestHandleSectorLookup_BySymbol_Multiple(t *testing.T) {
	s, done := newSectorTestServer(t)
	defer done()

	cases := []struct {
		symbol   string
		expected industry.SectorID
	}{
		{"2330", industry.SectorSemiconductor},
		{"2454", industry.SectorSemiconductor},
		{"2881", industry.SectorFinancials},
	}

	for _, tc := range cases {
		t.Run(tc.symbol, func(t *testing.T) {
			_, out, err := s.handleSectorLookup(context.Background(), nil, SectorLookupInput{Symbol: tc.symbol})
			if err != nil {
				t.Fatalf("handleSectorLookup: %v", err)
			}
			if !out.Found {
				t.Fatalf("expected Found=true for symbol %s", tc.symbol)
			}
			if out.Sector == nil || out.Sector.ID != string(tc.expected) {
				t.Fatalf("symbol %s: expected %s, got %v", tc.symbol, tc.expected, out.Sector)
			}
		})
	}
}

func TestHandleSectorLookup_ByChineseLabel(t *testing.T) {
	s, done := newSectorTestServer(t)
	defer done()

	cases := []struct {
		label    string
		expected industry.SectorID
	}{
		{"半導體", industry.SectorSemiconductor},
		{"金融", industry.SectorFinancials},
		{"生技醫療", industry.SectorBiotech},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			_, out, err := s.handleSectorLookup(context.Background(), nil, SectorLookupInput{Sector: tc.label})
			if err != nil {
				t.Fatalf("handleSectorLookup: %v", err)
			}
			if !out.Found {
				t.Fatalf("expected Found=true for label %s", tc.label)
			}
			if out.Sector == nil || out.Sector.ID != string(tc.expected) {
				t.Fatalf("label %s: expected %s, got %v", tc.label, tc.expected, out.Sector)
			}
		})
	}
}

func TestHandleSectorLookup_ByCanonicalID(t *testing.T) {
	s, done := newSectorTestServer(t)
	defer done()

	cases := []struct {
		id       string
		expected industry.SectorID
	}{
		{"semiconductor", industry.SectorSemiconductor},
		{"financials", industry.SectorFinancials},
		{"ai_supply_chain", industry.SubIndustryAISupplyChain},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			_, out, err := s.handleSectorLookup(context.Background(), nil, SectorLookupInput{Sector: tc.id})
			if err != nil {
				t.Fatalf("handleSectorLookup: %v", err)
			}
			if !out.Found {
				t.Fatalf("expected Found=true for id %s", tc.id)
			}
			if out.Sector == nil || out.Sector.ID != string(tc.expected) {
				t.Fatalf("id %s: expected %s, got %v", tc.id, tc.expected, out.Sector)
			}
		})
	}
}

func TestHandleSectorLookup_NotFound(t *testing.T) {
	s, done := newSectorTestServer(t)
	defer done()

	_, out, err := s.handleSectorLookup(context.Background(), nil, SectorLookupInput{Symbol: "9999"})
	if err != nil {
		t.Fatalf("handleSectorLookup: %v", err)
	}
	if out.Found {
		t.Fatal("expected Found=false")
	}
	if out.Warning == "" {
		t.Fatal("expected non-empty warning")
	}
	if out.Sector != nil {
		t.Fatal("expected sector to be nil")
	}
}

func TestHandleSectorLookup_EmptyArgsRejected(t *testing.T) {
	s, done := newSectorTestServer(t)
	defer done()

	_, out, err := s.handleSectorLookup(context.Background(), nil, SectorLookupInput{})
	if err != nil {
		t.Fatalf("handleSectorLookup: %v", err)
	}
	if out.Found {
		t.Fatal("expected Found=false")
	}
	if out.Warning == "" {
		t.Fatal("expected non-empty warning")
	}
	if out.Sector != nil {
		t.Fatal("expected sector to be nil")
	}
}
