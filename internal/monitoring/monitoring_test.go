package monitoring

import (
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/live"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
)

// ─── AlertLevel ──────────────────────────────────────────────────────────────

func TestAlertLevel_String(t *testing.T) {
	tests := []struct {
		level AlertLevel
		want  string
	}{
		{AlertLevelInfo, "INFO"},
		{AlertLevelWarning, "WARNING"},
		{AlertLevelError, "ERROR"},
		{AlertLevelCritical, "CRITICAL"},
		{AlertLevel(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("AlertLevel(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

// ─── Monitor ─────────────────────────────────────────────────────────────────

func TestMonitor_AlertDispatchedToHandler(t *testing.T) {
	m := NewMonitor()

	var mu sync.Mutex
	var received []Alert

	m.RegisterHandler(func(a Alert) {
		mu.Lock()
		received = append(received, a)
		mu.Unlock()
	})

	m.Alert(AlertLevelWarning, "test", "hello", nil)

	// 處理器為非同步 goroutine，等待短暫時間
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 alert dispatched, got %d", len(received))
	}
	if received[0].Level != AlertLevelWarning {
		t.Errorf("Level = %v, want WARNING", received[0].Level)
	}
	if received[0].Category != "test" {
		t.Errorf("Category = %q, want test", received[0].Category)
	}
	if received[0].ID == "" {
		t.Error("ID should not be empty")
	}
}

func TestMonitor_ConvenienceMethods(t *testing.T) {
	m := NewMonitor()

	var mu sync.Mutex
	seen := make(map[AlertLevel]int)
	m.RegisterHandler(func(a Alert) {
		mu.Lock()
		seen[a.Level]++
		mu.Unlock()
	})

	m.Info("c", "msg", nil)
	m.Warning("c", "msg", nil)
	m.Error("c", "msg", nil)
	m.Critical("c", "msg", nil)

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 4 {
		t.Fatalf("expected 4 distinct levels, got %d: %v", len(seen), seen)
	}
	for _, level := range []AlertLevel{AlertLevelInfo, AlertLevelWarning, AlertLevelError, AlertLevelCritical} {
		if seen[level] != 1 {
			t.Errorf("level %v seen %d times, want 1", level, seen[level])
		}
	}
}

func TestMonitor_HistoryMaxCap(t *testing.T) {
	m := NewMonitor()
	m.maxHistory = 5

	for i := 0; i < 8; i++ {
		m.Alert(AlertLevelInfo, "cat", "msg", nil)
	}

	history := m.GetHistory(0) // 0 => all
	if len(history) != 5 {
		t.Errorf("history len = %d, want 5 (maxHistory cap)", len(history))
	}
}

func TestMonitor_GetHistory_Limit(t *testing.T) {
	m := NewMonitor()
	for i := 0; i < 10; i++ {
		m.Alert(AlertLevelInfo, "cat", "msg", nil)
	}

	history := m.GetHistory(3)
	if len(history) != 3 {
		t.Errorf("GetHistory(3) len = %d, want 3", len(history))
	}
}

// ─── MetricsCollector ────────────────────────────────────────────────────────

func TestMetricsCollector_RecordCounter_Accumulates(t *testing.T) {
	c := NewMetricsCollector()
	labels := map[string]string{"env": "test"}

	c.RecordCounter("hits", 1, labels)
	c.RecordCounter("hits", 2, labels)
	c.RecordCounter("hits", 3, labels)

	m, ok := c.GetMetric("hits", labels)
	if !ok {
		t.Fatal("metric not found")
	}
	if m.Value != 6 {
		t.Errorf("counter value = %f, want 6", m.Value)
	}
	if m.Type != MetricTypeCounter {
		t.Errorf("type = %v, want counter", m.Type)
	}
}

func TestMetricsCollector_RecordGauge_Overwrites(t *testing.T) {
	c := NewMetricsCollector()

	c.RecordGauge("price", 100, nil)
	c.RecordGauge("price", 200, nil)

	m, ok := c.GetMetric("price", nil)
	if !ok {
		t.Fatal("metric not found")
	}
	if m.Value != 200 {
		t.Errorf("gauge value = %f, want 200", m.Value)
	}
}

func TestMetricsCollector_RecordHistogram(t *testing.T) {
	c := NewMetricsCollector()

	c.RecordHistogram("latency", 1.5, nil)
	c.RecordHistogram("latency", 2.5, nil)

	// RecordHistogram 不更新 metrics map，只更新 histograms
	_, ok := c.GetMetric("latency", nil)
	if ok {
		t.Error("RecordHistogram should not add to the metrics map")
	}
}

func TestMetricsCollector_GetAllMetrics(t *testing.T) {
	c := NewMetricsCollector()

	c.RecordGauge("a", 1, nil)
	c.RecordGauge("b", 2, map[string]string{"k": "v"})

	all := c.GetAllMetrics()
	if len(all) != 2 {
		t.Fatalf("GetAllMetrics len = %d, want 2", len(all))
	}
}

// ─── RuleEngine ──────────────────────────────────────────────────────────────

func TestRuleEngine_EvaluateRules_TriggersAlert(t *testing.T) {
	m := NewMonitor()

	var mu sync.Mutex
	var alerts []Alert
	m.RegisterHandler(func(a Alert) {
		mu.Lock()
		alerts = append(alerts, a)
		mu.Unlock()
	})

	engine := NewRuleEngine(m)
	engine.RegisterRule(AlertRule{
		Name:        "always_fires",
		Description: "test rule",
		Condition: func(*livestore.State) (bool, string) {
			return true, "always triggered"
		},
		Level:    AlertLevelWarning,
		Cooldown: 0,
	})

	engine.evaluateRules(&livestore.State{})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
}

func TestRuleEngine_EvaluateRules_CooldownPreventsRefire(t *testing.T) {
	m := NewMonitor()

	var mu sync.Mutex
	var count int
	m.RegisterHandler(func(Alert) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	engine := NewRuleEngine(m)
	engine.RegisterRule(AlertRule{
		Name: "cooldown_rule",
		Condition: func(*livestore.State) (bool, string) {
			return true, "triggered"
		},
		Level:    AlertLevelInfo,
		Cooldown: 10 * time.Minute, // 長冷卻時間
	})

	// 連續觸發兩次 evaluate
	engine.evaluateRules(&livestore.State{})
	engine.evaluateRules(&livestore.State{})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Errorf("expected 1 alert due to cooldown, got %d", count)
	}
}

func TestRuleEngine_EvaluateRules_ConditionFalse(t *testing.T) {
	m := NewMonitor()

	var mu sync.Mutex
	var alerts []Alert
	m.RegisterHandler(func(a Alert) {
		mu.Lock()
		alerts = append(alerts, a)
		mu.Unlock()
	})

	engine := NewRuleEngine(m)
	engine.RegisterRule(AlertRule{
		Name: "never_fires",
		Condition: func(*livestore.State) (bool, string) {
			return false, ""
		},
		Level:    AlertLevelInfo,
		Cooldown: 0,
	})

	engine.evaluateRules(&livestore.State{})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}

// ─── DefaultRules ─────────────────────────────────────────────────────────────

func TestDefaultRules_PortfolioValueDrop(t *testing.T) {
	rules := DefaultRules()
	var cashRule *AlertRule
	for i := range rules {
		if rules[i].Name == "portfolio_value_drop" {
			cashRule = &rules[i]
			break
		}
	}
	if cashRule == nil {
		t.Fatal("portfolio_value_drop rule not found in DefaultRules")
	}

	tests := []struct {
		name     string
		state    *livestore.State
		wantFire bool
	}{
		{"nil state no fire", nil, false},
		{"cash below threshold fires", &livestore.State{Portfolio: store.livestore.PortfolioState{Cash: 50000}}, true},
		{"cash above threshold no fire", &livestore.State{Portfolio: store.livestore.PortfolioState{Cash: 500000}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fired, _ := cashRule.Condition(tt.state)
			if fired != tt.wantFire {
				t.Errorf("Condition fired=%v, want %v", fired, tt.wantFire)
			}
		})
	}
}

func TestDefaultRules_PositionConcentration(t *testing.T) {
	rules := DefaultRules()
	var rule *AlertRule
	for i := range rules {
		if rules[i].Name == "position_concentration" {
			rule = &rules[i]
			break
		}
	}
	if rule == nil {
		t.Fatal("position_concentration rule not found in DefaultRules")
	}

	manyPositions := make([]domain.Position, 25)
	for i := range manyPositions {
		manyPositions[i] = domain.Position{Symbol: "X"}
	}

	tests := []struct {
		name     string
		state    *livestore.State
		wantFire bool
	}{
		{"nil state", nil, false},
		{"no positions", &livestore.State{}, false},
		{"21 positions fires", &livestore.State{Positions: manyPositions}, true},
		{"5 positions no fire", &livestore.State{Positions: manyPositions[:5]}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fired, _ := rule.Condition(tt.state)
			if fired != tt.wantFire {
				t.Errorf("Condition fired=%v, want %v", fired, tt.wantFire)
			}
		})
	}
}

// ─── TradingMetrics ──────────────────────────────────────────────────────────

func TestTradingMetrics_RecordOrder(t *testing.T) {
	collector := NewMetricsCollector()
	tm := NewTradingMetrics(collector, NewMonitor())

	order := domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 10,
		Price:    785.0,
	}

	tm.RecordOrder(order, "filled")

	// buildKey 對多鍵 map 迭代順序不固定，用 GetAllMetrics 驗證至少記錄了 orders_total
	named := func(name string) []Metric {
		var out []Metric
		for _, m := range collector.GetAllMetrics() {
			if m.Name == name {
				out = append(out, m)
			}
		}
		return out
	}

	orderMetrics := named("orders_total")
	if len(orderMetrics) == 0 {
		t.Fatal("orders_total metric not recorded after RecordOrder")
	}
	valueMetrics := named("order_value")
	if len(valueMetrics) == 0 {
		t.Fatal("order_value metric not recorded after RecordOrder")
	}
	if valueMetrics[0].Value != 785.0*10 {
		t.Errorf("order_value = %f, want %f", valueMetrics[0].Value, 785.0*10)
	}
}
