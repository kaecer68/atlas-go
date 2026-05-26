package eventlogic

import "sync"

type SelfCorrector struct {
	registry *RuleRegistry
	mu       sync.RWMutex
	fails    map[string]int
	hits     map[string]int
}

func NewCorrector(r *RuleRegistry) *SelfCorrector {
	return &SelfCorrector{registry: r, fails: make(map[string]int), hits: make(map[string]int)}
}
func (c *SelfCorrector) Evaluate(id string, hit bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.registry.GetByID(id)
	if !ok {
		return
	}
	if hit {
		c.fails[id] = 0
		c.hits[id]++
		if c.hits[id] >= 5 && r.Status == StatusDegraded {
			r.Status = StatusActive
			c.hits[id] = 0
			c.fails[id] = 0
		}
	} else {
		c.hits[id] = 0
		c.fails[id]++
		f := c.fails[id]
		if f >= 20 {
			r.Status = StatusExpired
		} else if f >= 10 && r.Status == StatusActive {
			r.Status = StatusDegraded
		}
	}
	_ = c.registry.Update(r)
}
func (c *SelfCorrector) Run(rs []ValidationResult) {
	for _, r := range rs {
		if r.Fired && r.Error == "" {
			c.Evaluate(r.RuleID, r.WasHit)
		}
	}
}
func (c *SelfCorrector) GetStreak(id string) (int, int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.fails[id], c.hits[id]
}
