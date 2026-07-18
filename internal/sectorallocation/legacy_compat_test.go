package sectorallocation_test

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

func TestLegacyCompatReader_PopulatesRawMap(t *testing.T) {
	rcr := sectorallocation.NewLegacyCompatCounterForTest()
	reader := sectorallocation.NewLegacyCompatReaderForTest(rcr)
	out := reader.Read()
	if len(out) < 10 {
		t.Fatalf("legacy compat reader must populate at least 10 keys, got %d", len(out))
	}
	if out["semiconductor"] <= 0 {
		t.Fatal("legacy compat reader must return at least semiconductor key")
	}
}

func TestLegacyCompatReader_LogsAndCounts(t *testing.T) {
	rcr := sectorallocation.NewLegacyCompatCounterForTest()
	reader := sectorallocation.NewLegacyCompatReaderForTest(rcr)
	_ = reader.Read()
	snap := rcr.Snapshot()
	if len(snap) == 0 {
		t.Fatal("legacy compat reader must record counter on Read()")
	}
}

func TestLegacyCompatReader_PromotionGateFalseByDefault(t *testing.T) {
	rcr := sectorallocation.NewLegacyCompatCounterForTest()
	reader := sectorallocation.NewLegacyCompatReaderForTest(rcr)
	if reader.PromotionGate() {
		t.Fatal("legacy compat reader promotion gate must be false by default (SA12 sunset gate)")
	}
}

func TestLegacyCompatCounter_IncAndSnapshot(t *testing.T) {
	c := sectorallocation.NewLegacyCompatCounterForTest()
	c.Inc("test_caller")
	c.Inc("test_caller")
	c.Inc("other_caller")
	snap := c.Snapshot()
	if snap["test_caller"] != 2 {
		t.Fatalf("counter snapshot expected test_caller=2, got %d", snap["test_caller"])
	}
	if snap["other_caller"] != 1 {
		t.Fatalf("counter snapshot expected other_caller=1, got %d", snap["other_caller"])
	}
}

func TestLegacyCompatCounter_Reset(t *testing.T) {
	c := sectorallocation.NewLegacyCompatCounterForTest()
	c.Inc("test_caller")
	c.Reset()
	snap := c.Snapshot()
	if len(snap) != 0 {
		t.Fatalf("after Reset(), counter snapshot must be empty, got %v", snap)
	}
}
