package symbolindustry

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// snapshotPayload is the wire shape written by the symbol_industry channel,
// including an extra reporting block the reader must ignore.
const snapshotPayload = `{
  "channel": "symbol_industry",
  "updated_at": "2026-09-24T07:30:00Z",
  "sources": ["TWSE:t187ap03_L", "TPEx:t187ap03_L"],
  "counts": {
    "total": 3,
    "mapped": 2,
    "unmapped": 1,
    "unknown": 0,
    "canonical_l1": 2
  },
  "extended_reporting_block": {"note": "ignored by the reader", "latency_ms": 41},
  "entries": [
    {
      "symbol": "2330",
      "company_name": "台積電",
      "market": "TWSE",
      "industry_code": "24",
      "industry_name_zh": "半導體業",
      "canonical_l1": "semiconductors",
      "mapping_status": "mapped",
      "mapping_reason": "twse-code-24",
      "source": "TWSE:t187ap03_L",
      "as_of": "2026-09-24",
      "updated_at": "2026-09-24T07:30:00Z"
    },
    {
      "symbol": "6488",
      "company_name": "環球晶",
      "market": "TPEx",
      "industry_code": "24",
      "industry_name_zh": "半導體業",
      "canonical_l1": "semiconductors",
      "mapping_status": "mapped",
      "mapping_reason": "tpex-code-24",
      "source": "TPEx:t187ap03_L",
      "as_of": "2026-09-24"
    },
    {
      "symbol": "1303",
      "company_name": "南亞",
      "market": "TWSE",
      "industry_code": "03",
      "industry_name_zh": "塑膠工業",
      "canonical_l1": "",
      "mapping_status": "unmapped",
      "mapping_reason": "no-canonical-l1-for-code-03",
      "source": "TWSE:t187ap03_L",
      "as_of": "2026-09-24"
    }
  ]
}`

func writeTempSnapshot(t *testing.T, payload string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "symbol_industry.json")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	return path
}

func TestReadSnapshotParses(t *testing.T) {
	path := writeTempSnapshot(t, snapshotPayload)

	snap, err := ReadSnapshot(path)
	if err != nil {
		t.Fatalf("ReadSnapshot: %v", err)
	}
	if snap.Channel != "symbol_industry" {
		t.Fatalf("Channel = %q", snap.Channel)
	}
	if snap.UpdatedAt != "2026-09-24T07:30:00Z" {
		t.Fatalf("UpdatedAt = %q", snap.UpdatedAt)
	}
	if len(snap.Sources) != 2 || snap.Sources[0] != "TWSE:t187ap03_L" {
		t.Fatalf("Sources = %+v", snap.Sources)
	}
	want := SnapshotCounts{Total: 3, Mapped: 2, Unmapped: 1, Unknown: 0, CanonicalL1: 2}
	if snap.Counts != want {
		t.Fatalf("Counts = %+v, want %+v", snap.Counts, want)
	}
	if len(snap.Entries) != 3 {
		t.Fatalf("Entries len = %d, want 3", len(snap.Entries))
	}

	bySymbol := make(map[string]Entry, len(snap.Entries))
	for _, e := range snap.Entries {
		bySymbol[e.Symbol] = e
	}

	tsmc := bySymbol["2330"]
	if tsmc.CompanyName != "台積電" || tsmc.Market != "TWSE" ||
		tsmc.IndustryCode != "24" || tsmc.IndustryNameZH != "半導體業" ||
		tsmc.CanonicalL1 != "semiconductors" ||
		tsmc.MappingStatus != StatusMapped ||
		tsmc.MappingReason != "twse-code-24" ||
		tsmc.Source != "TWSE:t187ap03_L" || tsmc.AsOf != "2026-09-24" {
		t.Fatalf("2330 entry = %+v", tsmc)
	}
	if tsmc.UpdatedAt.IsZero() {
		t.Fatal("2330 UpdatedAt is zero")
	}

	gwc := bySymbol["6488"]
	if gwc.Market != "TPEx" || gwc.CanonicalL1 != "semiconductors" {
		t.Fatalf("6488 entry = %+v", gwc)
	}
	if !gwc.UpdatedAt.IsZero() {
		t.Fatalf("6488 UpdatedAt = %v, want zero (absent in payload)", gwc.UpdatedAt)
	}

	unmapped := bySymbol["1303"]
	if unmapped.MappingStatus != StatusUnmapped {
		t.Fatalf("1303 MappingStatus = %q, want %q", unmapped.MappingStatus, StatusUnmapped)
	}
	if unmapped.CanonicalL1 != "" {
		t.Fatalf("1303 CanonicalL1 = %q, want empty", unmapped.CanonicalL1)
	}
}

func TestReadSnapshotMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	_, err := ReadSnapshot(path)
	if err == nil {
		t.Fatal("ReadSnapshot(missing) = nil error, want error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadSnapshot(missing) error = %v, want wrapping os.ErrNotExist", err)
	}
}

func TestReadSnapshotWrongChannel(t *testing.T) {
	path := writeTempSnapshot(t, `{"channel":"other_channel","entries":[{"symbol":"2330"}]}`)
	if _, err := ReadSnapshot(path); err == nil {
		t.Fatal("ReadSnapshot(wrong channel) = nil error, want error")
	}
}

func TestReadSnapshotEmptyEntries(t *testing.T) {
	path := writeTempSnapshot(t, `{"channel":"symbol_industry","entries":[]}`)
	if _, err := ReadSnapshot(path); err == nil {
		t.Fatal("ReadSnapshot(empty entries) = nil error, want error")
	}
}

func TestReadSnapshotDedupesSymbols(t *testing.T) {
	path := writeTempSnapshot(t, `{
		"channel": "symbol_industry",
		"entries": [
			{"symbol": "2330", "canonical_l1": "semiconductors", "mapping_status": "mapped"},
			{"symbol": "2330", "canonical_l1": "wrong", "mapping_status": "unmapped"},
			{"symbol": "", "canonical_l1": "ghost", "mapping_status": "mapped"},
			{"symbol": "1303", "canonical_l1": "", "mapping_status": "unmapped"}
		]
	}`)

	snap, err := ReadSnapshot(path)
	if err != nil {
		t.Fatalf("ReadSnapshot: %v", err)
	}
	if len(snap.Entries) != 2 {
		t.Fatalf("Entries len = %d, want 2 (%+v)", len(snap.Entries), snap.Entries)
	}
	if snap.Entries[0].Symbol != "2330" || snap.Entries[1].Symbol != "1303" {
		t.Fatalf("Entries = %+v, want first-seen order 2330, 1303", snap.Entries)
	}
	if snap.Entries[0].CanonicalL1 != "semiconductors" {
		t.Fatalf("dedupe kept the wrong copy: %+v", snap.Entries[0])
	}
}
