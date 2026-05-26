package eventlogic

import "time"

type DiscoveryInput struct {
	NarrativeEvents []NarrativeEventSnapshot
	PriceChanges    []PriceChangeSnapshot
}
type NarrativeEventSnapshot struct {
	Theme      string
	DetectedAt time.Time
}
type PriceChangeSnapshot struct {
	Symbol     string
	Sector     string
	ChangePct  float64
	RecordedAt time.Time
}
type PatternDetector struct{ registry *RuleRegistry }

func NewDetector(r *RuleRegistry) *PatternDetector { return &PatternDetector{registry: r} }

type DiscoverCandidate struct {
	Pattern    string
	Conditions []Condition
	Sectors    []string
	Direction  string
	HitCount   int
	TotalCount int
	HitRate    float64
}

func (d *PatternDetector) DiscoverPatterns(in *DiscoveryInput) []DiscoverCandidate {
	if in == nil || len(in.NarrativeEvents) == 0 || len(in.PriceChanges) == 0 {
		return nil
	}
	const minO, minR, maxD = 5, 0.55, 5.0
	type k struct{ th, se string }
	type s struct{ up, dn int }
	m := make(map[k]*s)
	for _, ev := range in.NarrativeEvents {
		for _, pc := range in.PriceChanges {
			if d := pc.RecordedAt.Sub(ev.DetectedAt).Hours() / 24; d >= 0 && d <= maxD {
				kk := k{ev.Theme, pc.Sector}
				if m[kk] == nil {
					m[kk] = &s{}
				}
				if pc.ChangePct > 0 {
					m[kk].up++
				} else if pc.ChangePct < 0 {
					m[kk].dn++
				}
			}
		}
	}
	var out []DiscoverCandidate
	for kk, ss := range m {
		tot := ss.up + ss.dn
		if tot < minO {
			continue
		}
		if r := float64(ss.up) / float64(tot); r >= minR {
			out = append(out, d.build(kk.th, kk.se, DirUp, ss.up, tot, r))
		} else if r := float64(ss.dn) / float64(tot); r >= minR {
			out = append(out, d.build(kk.th, kk.se, DirDown, ss.dn, tot, r))
		}
	}
	return out
}

func (d *PatternDetector) build(th, se, dir string, hh, tt int, rr float64) DiscoverCandidate {
	return DiscoverCandidate{Pattern: th + " → " + se, Sectors: []string{se}, Direction: dir, Conditions: []Condition{{Field: "NarrativeTheme", Operator: "eq", StringValue: th}}, HitCount: hh, TotalCount: tt, HitRate: rr}
}

func (d *PatternDetector) PromoteCandidate(c *DiscoverCandidate) (*EventRule, error) {
	r := &EventRule{ID: c.Conditions[0].StringValue + "-" + c.Sectors[0], Pattern: c.Pattern, Conditions: c.Conditions, AffectedSectors: c.Sectors, Direction: c.Direction, HitRate: c.HitRate, TotalTests: c.TotalCount, TotalHits: c.HitCount, ConfidenceSource: SourceAutoDiscovered, Status: StatusActive, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	return r, d.registry.Add(r)
}
