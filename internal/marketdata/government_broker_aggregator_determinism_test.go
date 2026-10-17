package marketdata

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// sha256File returns the lowercase hex sha256 of path's contents.
func sha256File(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// brokerDetailsFixture builds a deterministic *set* of broker rows whose
// persisted order must not depend on Go map iteration order.
func brokerDetailsFixture() map[detailKey]*detailAccumulator {
	details := make(map[detailKey]*detailAccumulator, 40)
	for i := 0; i < 40; i++ {
		// Interleave the two types and two names so the sort key must be the
		// full (Code, Type, Name) tuple and not just Code: codes 1000..1019
		// each appear twice, once per type.
		code := fmt.Sprintf("%04d", 1000+i%20)
		typ := "gov"
		name := "bank-" + code
		if i >= 20 {
			typ = "ins"
			name = "insurance-" + code
		}
		details[detailKey{Code: code, Name: name, Type: typ}] = &detailAccumulator{
			Code: code, Name: name, Type: typ,
			Buy: int64(100 + i), Sell: int64(i), Net: 100,
		}
	}
	return details
}

// TestWriteBrokerDetails_DeterministicBrokerOrder pins the write contract of
// YYYYMMDD_brokers.json: the brokers[] array is a canonical total order on
// (code, type, name), so the same input always produces byte-identical output
// regardless of Go's randomized map iteration order.
//
// Before the fix the array was appended by ranging over the details map, so two
// runs of the same input produced different bytes (and therefore different
// sha256) — see FU-20261016-33.
func TestWriteBrokerDetails_DeterministicBrokerOrder(t *testing.T) {
	details := brokerDetailsFixture()

	dirA := t.TempDir()
	dirB := t.TempDir()
	if err := NewGovernmentBrokerAggregator(dirA).writeBrokerDetails("20261016", details); err != nil {
		t.Fatalf("write A: %v", err)
	}
	if err := NewGovernmentBrokerAggregator(dirB).writeBrokerDetails("20261016", details); err != nil {
		t.Fatalf("write B: %v", err)
	}

	pathA := filepath.Join(dirA, "20261016_brokers.json")
	pathB := filepath.Join(dirB, "20261016_brokers.json")
	shaA := sha256File(t, pathA)
	shaB := sha256File(t, pathB)
	t.Logf("sha256 A = %s", shaA)
	t.Logf("sha256 B = %s", shaB)

	rawA, err := os.ReadFile(pathA)
	if err != nil {
		t.Fatalf("read A: %v", err)
	}
	rawB, err := os.ReadFile(pathB)
	if err != nil {
		t.Fatalf("read B: %v", err)
	}
	if string(rawA) != string(rawB) {
		var rowsA, rowsB []BrokerDailyDetail
		_ = json.Unmarshal(rawA, &struct {
			Brokers *[]BrokerDailyDetail `json:"brokers"`
		}{Brokers: &rowsA})
		_ = json.Unmarshal(rawB, &struct {
			Brokers *[]BrokerDailyDetail `json:"brokers"`
		}{Brokers: &rowsB})
		t.Fatalf("brokers file is not byte-identical across two runs of the same input (sha256 %s vs %s)\nfirst row order A: %v\nfirst row order B: %v",
			shaA, shaB, codesOf(rowsA), codesOf(rowsB))
	}

	// Content set must be unchanged: every accumulated broker is still present
	// exactly once, and the order is ascending on (code, type, name).
	var payload struct {
		Brokers []BrokerDailyDetail `json:"brokers"`
	}
	if err := json.Unmarshal(rawA, &payload); err != nil {
		t.Fatalf("unmarshal A: %v", err)
	}
	if len(payload.Brokers) != len(details) {
		t.Fatalf("brokers len = %d, want %d", len(payload.Brokers), len(details))
	}
	got := make([]string, 0, len(payload.Brokers))
	want := make([]string, 0, len(details))
	for _, row := range payload.Brokers {
		got = append(got, row.Code+"|"+row.Type+"|"+row.Name)
	}
	for k := range details {
		want = append(want, k.Code+"|"+k.Type+"|"+k.Name)
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("brokers[] is not the canonical (code,type,name) order\n got: %v\nwant: %v", got, want)
	}
}

func codesOf(rows []BrokerDailyDetail) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Code+"/"+r.Type)
	}
	return out
}
