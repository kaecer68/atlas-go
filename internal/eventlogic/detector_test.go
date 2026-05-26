package eventlogic

import (
	"testing"
	"time"
)

func TestDEmpty(t *testing.T) {
	d := NewDetector(NewRegistry())
	if len(d.DiscoverPatterns(nil)) != 0 {
		t.Error("nil")
	}
	if len(d.DiscoverPatterns(&DiscoveryInput{})) != 0 {
		t.Error("empty")
	}
}
func TestDCorr(t *testing.T) {
	d := NewDetector(NewRegistry())
	now := time.Now()
	in := &DiscoveryInput{NarrativeEvents: []NarrativeEventSnapshot{{"AI", now}, {"AI", now}, {"AI", now}, {"OIL", now}, {"OIL", now}}, PriceChanges: []PriceChangeSnapshot{{"s1", "semi", 3, now.Add(24 * time.Hour)}, {"s2", "semi", 2, now.Add(24 * time.Hour)}, {"s3", "semi", 1, now.Add(24 * time.Hour)}, {"p1", "petro", -5, now.Add(48 * time.Hour)}, {"p2", "petro", -4, now.Add(48 * time.Hour)}, {"p3", "petro", -3, now.Add(48 * time.Hour)}}}
	cs := d.DiscoverPatterns(in)
	fS, fP := false, false
	for _, c := range cs {
		if c.Sectors[0] == "semi" && c.Direction == DirUp {
			fS = true
		}
		if c.Sectors[0] == "petro" && c.Direction == DirDown {
			fP = true
		}
	}
	if !fS {
		t.Error("AI->semi up")
	}
	if !fP {
		t.Error("OIL->petro down")
	}
}
func TestPromote(t *testing.T) {
	reg := NewRegistry()
	d := NewDetector(reg)
	b := reg.Count()
	r, err := d.PromoteCandidate(&DiscoverCandidate{Pattern: "x", Conditions: []Condition{{Field: "N", Operator: "eq", StringValue: "T"}}, Sectors: []string{"s"}, Direction: DirUp, HitCount: 8, TotalCount: 10, HitRate: 0.8})
	if err != nil || reg.Count() != b+1 || r.ConfidenceSource != SourceAutoDiscovered {
		t.Errorf("err=%v", err)
	}
}
