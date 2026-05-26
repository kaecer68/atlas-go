package eventlogic

import (
	"sync"
	"testing"
)

func TestEvalCond(t *testing.T) {
	v := NewValidator(NewRegistry())
	ctx := &ValidationContext{NumericFields: map[string]float64{"x": 10}, StringFields: map[string]string{"s": "AI"}}
	tests := []struct {
		n string
		c Condition
		w bool
	}{{"gt_t", Condition{Field: "x", Operator: "gt", Value: 5}, true}, {"eq_t", Condition{Field: "x", Operator: "eq", Value: 10}, true}, {"str_t", Condition{Field: "s", Operator: "eq", StringValue: "AI"}, true}, {"str_f", Condition{Field: "s", Operator: "eq", StringValue: "OIL"}, false}}
	for _, tt := range tests {
		t.Run(tt.n, func(t *testing.T) {
			if g := v.EvaluateCondition(tt.c, ctx); g != tt.w {
				t.Errorf("%v!=%v", g, tt.w)
			}
		})
	}
}

func TestRecOut(t *testing.T) {
	reg := NewRegistry()
	v := NewValidator(reg)
	reg.Add(NewEventRule("r", "p", []Condition{{"x", "gt", 5, ""}}, []string{"t"}, DirUp))
	for _, h := range []bool{true, false, true, true, false} {
		v.RecordOutcome("r", h)
	}
	u, _ := reg.GetByID("r")
	if u.TotalTests != 5 || u.TotalHits != 3 || u.HitRate != 0.6 {
		t.Errorf("t=%d h=%d r=%f", u.TotalTests, u.TotalHits, u.HitRate)
	}
}

func TestRecConc(t *testing.T) {
	reg := NewRegistry()
	v := NewValidator(reg)
	reg.Add(NewEventRule("c", "p", nil, []string{"t"}, DirUp))
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); v.RecordOutcome("c", true) }()
	}
	wg.Wait()
	u, _ := reg.GetByID("c")
	if u.TotalTests != 50 || u.TotalHits != 50 {
		t.Errorf("t=%d h=%d", u.TotalTests, u.TotalHits)
	}
}
