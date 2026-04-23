package monitoring

import (
	"context"
	"github.com/kaecer68/atlas-go/internal/domain"
	"sync"
	"time"
)

// MetricType 指標類型
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
)

// Metric 指標數據
type Metric struct {
	Name   string
	Value  float64
	Type   MetricType
	Labels map[string]string
}

// MetricsCollector 收集系統運行指標
type MetricsCollector struct {
	mu sync.RWMutex

	// 通用指標存儲
	metrics    map[string]Metric
	histograms map[string][]float64

	// Screening metrics
	screeningTotal    int64
	screeningPassed   int64
	screeningRejected int64

	// Alert metrics
	alertsTriggered    int64
	alertsAcknowledged int64
	alertsByType       map[string]int64
}

// NewMetricsCollector 建立新的指標收集器
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		metrics:      make(map[string]Metric),
		histograms:   make(map[string][]float64),
		alertsByType: make(map[string]int64),
	}
}

// RecordCounter 記錄計數器（累加）
func (m *MetricsCollector) RecordCounter(name string, value float64, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := metricKey(name, labels)
	existing, ok := m.metrics[key]
	if ok {
		existing.Value += value
		m.metrics[key] = existing
	} else {
		m.metrics[key] = Metric{
			Name:   name,
			Value:  value,
			Type:   MetricTypeCounter,
			Labels: labels,
		}
	}
}

// RecordGauge 記錄儀表（覆蓋）
func (m *MetricsCollector) RecordGauge(name string, value float64, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := metricKey(name, labels)
	m.metrics[key] = Metric{
		Name:   name,
		Value:  value,
		Type:   MetricTypeGauge,
		Labels: labels,
	}
}

// RecordHistogram 記錄直方圖
func (m *MetricsCollector) RecordHistogram(name string, value float64, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.histograms[name] = append(m.histograms[name], value)
}

// GetMetric 取得指標
func (m *MetricsCollector) GetMetric(name string, labels map[string]string) (Metric, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := metricKey(name, labels)
	metric, ok := m.metrics[key]
	return metric, ok
}

// GetAllMetrics 取得所有指標
func (m *MetricsCollector) GetAllMetrics() []Metric {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Metric, 0, len(m.metrics))
	for _, metric := range m.metrics {
		result = append(result, metric)
	}
	return result
}

// RecordScreening 記錄篩選結果
func (m *MetricsCollector) RecordScreening(passed, rejected int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.screeningTotal += passed + rejected
	m.screeningPassed += passed
	m.screeningRejected += rejected
}

// GetScreeningRate 取得篩選率
func (m *MetricsCollector) GetScreeningRate() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.screeningTotal == 0 {
		return 0
	}
	return float64(m.screeningPassed) / float64(m.screeningTotal)
}

// RecordAlert 記錄警報觸發
func (m *MetricsCollector) RecordAlert(alertType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alertsTriggered++
	m.alertsByType[alertType]++
}

// RecordAlertAcknowledged 記錄警報確認
func (m *MetricsCollector) RecordAlertAcknowledged() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alertsAcknowledged++
}

// GetAlertTriggerRate 取得警報觸發率
func (m *MetricsCollector) GetAlertTriggerRate() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return float64(m.alertsTriggered)
}

// GetMetricsSnapshot 取得指標快照
func (m *MetricsCollector) GetMetricsSnapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	alertsByTypeCopy := make(map[string]int64)
	for k, v := range m.alertsByType {
		alertsByTypeCopy[k] = v
	}

	return MetricsSnapshot{
		ScreeningTotal:     m.screeningTotal,
		ScreeningPassed:    m.screeningPassed,
		ScreeningRate:      m.GetScreeningRate(),
		AlertsTriggered:    m.alertsTriggered,
		AlertsAcknowledged: m.alertsAcknowledged,
		AlertsByType:       alertsByTypeCopy,
		Timestamp:          time.Now(),
	}
}

// MetricsSnapshot 指標快照
type MetricsSnapshot struct {
	ScreeningTotal     int64            `json:"screening_total"`
	ScreeningPassed    int64            `json:"screening_passed"`
	ScreeningRate      float64          `json:"screening_rate"`
	AlertsTriggered    int64            `json:"alerts_triggered"`
	AlertsAcknowledged int64            `json:"alerts_acknowledged"`
	AlertsByType       map[string]int64 `json:"alerts_by_type"`
	Timestamp          time.Time        `json:"timestamp"`
}

// metricKey 生成指標鍵
func metricKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	// 簡單實現，實際應排序 labels
	return name
}

// TradingMetrics 交易指標
type TradingMetrics struct {
	collector *MetricsCollector
	monitor   *Monitor
}

// NewTradingMetrics 建立交易指標收集器
func NewTradingMetrics(collector *MetricsCollector, monitor *Monitor) *TradingMetrics {
	return &TradingMetrics{
		collector: collector,
		monitor:   monitor,
	}
}

func (tm *TradingMetrics) RecordOrder(order domain.Order, status string) {
	// 記錄訂單總數
	tm.collector.RecordCounter("orders_total", 1, map[string]string{
		"symbol": order.Symbol,
		"side":   string(order.Side),
		"status": status,
	})

	// 記錄訂單價值
	orderValue := float64(order.Quantity) * order.Price
	tm.collector.RecordGauge("order_value", orderValue, map[string]string{
		"symbol": order.Symbol,
		"side":   string(order.Side),
	})
}

// RecordPosition 記錄部位
func (tm *TradingMetrics) RecordPosition(position domain.Position) {
	tm.collector.RecordGauge("position_value", position.MarketValue, map[string]string{
		"symbol": position.Symbol,
	})
}

// RecordPortfolio 記錄投組
func (tm *TradingMetrics) RecordPortfolio(cash, totalValue float64) {
	tm.collector.RecordGauge("portfolio_cash", cash, nil)
	tm.collector.RecordGauge("portfolio_total", totalValue, nil)
}

// RecordCircuitBreakerState 記錄熔斷狀態
func (tm *TradingMetrics) RecordCircuitBreakerState(state string) {
	tm.collector.RecordGauge("circuit_breaker_state", 1, map[string]string{
		"state": state,
	})
}

// RecordRiskEvent 記錄風險事件
func (tm *TradingMetrics) RecordRiskEvent(eventType, symbol string) {
	tm.collector.RecordCounter("risk_events", 1, map[string]string{
		"type":   eventType,
		"symbol": symbol,
	})
}

// RecordCounter 記錄計數器
func (tm *TradingMetrics) RecordCounter(name string, value float64, labels map[string]string) {
	tm.collector.RecordCounter(name, value, labels)
}

// RecordGauge 記錄儀表
func (tm *TradingMetrics) RecordGauge(name string, value float64, labels map[string]string) {
	tm.collector.RecordGauge(name, value, labels)
}

// SystemMetrics 系統指標
type SystemMetrics struct {
	collector *MetricsCollector
	monitor   *Monitor
}

// NewSystemMetrics 建立系統指標收集器
func NewSystemMetrics(collector *MetricsCollector, monitor *Monitor) *SystemMetrics {
	return &SystemMetrics{
		collector: collector,
		monitor:   monitor,
	}
}

// Start 啟動系統指標收集

// Start 啟動系統指標收集
func (sm *SystemMetrics) Start(ctx context.Context) {
	// 啟動背景收集任務
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 定期收集系統指標
			}
		}
	}()
}
