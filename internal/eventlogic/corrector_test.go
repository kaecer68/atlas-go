package eventlogic

import "testing"

func TestDegrade(t *testing.T) {
	reg := NewRegistry()
	c := NewCorrector(reg)
	reg.Add(NewEventRule("d", "p", nil, []string{"t"}, DirUp))
	for i := 0; i < 9; i++ {
		c.Evaluate("d", false)
	}
	u, _ := reg.GetByID("d")
	if u.Status != StatusActive {
		t.Errorf("9:%q", u.Status)
	}
	c.Evaluate("d", false)
	u, _ = reg.GetByID("d")
	if u.Status != StatusDegraded {
		t.Errorf("10:%q", u.Status)
	}
}

func TestExpire(t *testing.T) {
	reg := NewRegistry()
	c := NewCorrector(reg)
	reg.Add(NewEventRule("e", "p", nil, []string{"t"}, DirUp))
	for i := 0; i < 20; i++ {
		c.Evaluate("e", false)
	}
	u, _ := reg.GetByID("e")
	if u.Status != StatusExpired {
		t.Errorf("20:%q", u.Status)
	}
}

func TestRecover(t *testing.T) {
	reg := NewRegistry()
	c := NewCorrector(reg)
	reg.Add(NewEventRule("r", "p", nil, []string{"t"}, DirUp))
	for i := 0; i < 10; i++ {
		c.Evaluate("r", false)
	}
	for i := 0; i < 5; i++ {
		c.Evaluate("r", true)
	}
	u, _ := reg.GetByID("r")
	if u.Status != StatusActive {
		t.Errorf("5h:%q", u.Status)
	}
}

func TestExpImmune(t *testing.T) {
	reg := NewRegistry()
	c := NewCorrector(reg)
	rr := NewEventRule("ei", "p", nil, []string{"t"}, DirUp)
	rr.Status = StatusExpired
	reg.Add(rr)
	for i := 0; i < 100; i++ {
		c.Evaluate("ei", true)
	}
	u, _ := reg.GetByID("ei")
	if u.Status != StatusExpired {
		t.Errorf("%q", u.Status)
	}
}

func TestCRun(t *testing.T) {
	reg := NewRegistry()
	c := NewCorrector(reg)
	reg.Add(NewEventRule("run", "p", nil, []string{"t"}, DirUp))
	rs := make([]ValidationResult, 10)
	for i := 0; i < 10; i++ {
		rs[i] = ValidationResult{RuleID: "run", Fired: true, WasHit: false}
	}
	c.Run(rs)
	u, _ := reg.GetByID("run")
	if u.Status != StatusDegraded {
		t.Errorf("%q", u.Status)
	}
}
