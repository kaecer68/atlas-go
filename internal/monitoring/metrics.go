package monitoring

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// MetricType 指标类型
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
)

// Metric 指标
type Metric struct {
	Name      string
	Type      MetricType
	Value     float64
	Labels    map[string]string
	Timestamp time.Time
}

// MetricsCollector 指标收集器
type MetricsCollector struct {
	metrics    map[string]Metric
	histograms map[string][]float64
	mu         sync.RWMutex
}

// NewMetricsCollector 创建指标收集器
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		metrics:    make(map[string]Metric),
		histograms: make(map[string][]float64),
	}
}

// RecordCounter 记录计数器
func (c *MetricsCollector) RecordCounter(name string, value float64, labels map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.buildKey(name, labels)
	if existing, ok := c.metrics[key]; ok {
		c.metrics[key] = Metric{
			Name:      name,
			Type:      MetricTypeCounter,
			Value:     existing.Value + value,
			Labels:    labels,
			Timestamp: time.Now(),
		}
	} else {
		c.metrics[key] = Metric{
			Name:      name,
			Type:      MetricTypeCounter,
			Value:     value,
			Labels:    labels,
			Timestamp: time.Now(),
		}
	}
}

// RecordGauge 记录仪表盘
func (c *MetricsCollector) RecordGauge(name string, value float64, labels map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.buildKey(name, labels)
	c.metrics[key] = Metric{
		Name:      name,
		Type:      MetricTypeGauge,
		Value:     value,
		Labels:    labels,
		Timestamp: time.Now(),
	}
}

// RecordHistogram 记录直方图
func (c *MetricsCollector) RecordHistogram(name string, value float64, labels map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.buildKey(name, labels)
	c.histograms[key] = append(c.histograms[key], value)
}

// GetMetric 获取指标
func (c *MetricsCollector) GetMetric(name string, labels map[string]string) (Metric, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := c.buildKey(name, labels)
	metric, ok := c.metrics[key]
	return metric, ok
}

// GetAllMetrics 获取所有指标
func (c *MetricsCollector) GetAllMetrics() []Metric {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]Metric, 0, len(c.metrics))
	for _, metric := range c.metrics {
		result = append(result, metric)
	}
	return result
}

// buildKey 构建指标键
func (c *MetricsCollector) buildKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}

	key := name
	for k, v := range labels {
		key = fmt.Sprintf("%s:%s=%s", key, k, v)
	}
	return key
}

// SystemMetrics 系统指标
type SystemMetrics struct {
	collector *MetricsCollector
	monitor   *Monitor
	interval  time.Duration
}

// NewSystemMetrics 创建系统指标收集器
func NewSystemMetrics(collector *MetricsCollector, monitor *Monitor) *SystemMetrics {
	return &SystemMetrics{
		collector: collector,
		monitor:   monitor,
		interval:  60 * time.Second,
	}
}

// Start 启动系统指标收集
func (s *SystemMetrics) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.collect()
		}
	}
}

// collect 收集系统指标
func (s *SystemMetrics) collect() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	s.collector.RecordGauge("system_memory_usage_mb", float64(ms.Sys)/1024/1024, nil)
	s.collector.RecordGauge("system_goroutines", float64(runtime.NumGoroutine()), nil)
	s.monitor.Info("metrics", "System metrics collected", nil)
}

// TradingMetrics 交易指标
type TradingMetrics struct {
	collector *MetricsCollector
	monitor   *Monitor
}

// NewTradingMetrics 创建交易指标收集器
func NewTradingMetrics(collector *MetricsCollector, monitor *Monitor) *TradingMetrics {
	return &TradingMetrics{
		collector: collector,
		monitor:   monitor,
	}
}

// RecordOrder 记录订单
func (t *TradingMetrics) RecordOrder(order domain.Order, status string) {
	labels := map[string]string{
		"symbol": order.Symbol,
		"side":   string(order.Side),
		"status": status,
	}

	t.collector.RecordCounter("orders_total", 1, labels)
	t.collector.RecordGauge("order_value", order.Price*float64(order.Quantity), labels)
}

// RecordPosition 记录持仓
func (t *TradingMetrics) RecordPosition(position domain.Position) {
	labels := map[string]string{
		"symbol": position.Symbol,
	}

	t.collector.RecordGauge("position_value", position.MarketValue, labels)
	t.collector.RecordGauge("position_pnl", position.UnrealizedPnL, labels)
}

// RecordPortfolio 记录投资组合
func (t *TradingMetrics) RecordPortfolio(cash float64, totalValue float64) {
	t.collector.RecordGauge("portfolio_cash", cash, nil)
	t.collector.RecordGauge("portfolio_total_value", totalValue, nil)
}

// RecordCircuitBreakerState 记录断路器状态
func (t *TradingMetrics) RecordCircuitBreakerState(state string) {
	value := 0.0
	switch state {
	case "paused":
		value = 1.0
	case "halted":
		value = 2.0
	}
	t.collector.RecordGauge("circuit_breaker_state", value, map[string]string{
		"state": state,
	})
}

// RecordRiskEvent 记录风险事件
func (t *TradingMetrics) RecordRiskEvent(eventType string, symbol string) {
	t.collector.RecordCounter("risk_events_total", 1, map[string]string{
		"type":   eventType,
		"symbol": symbol,
	})
}

// RecordCounter 记录通用计数器
func (t *TradingMetrics) RecordCounter(name string, value float64, labels map[string]string) {
	t.collector.RecordCounter(name, value, labels)
}

// RecordGauge 记录通用仪表盘
func (t *TradingMetrics) RecordGauge(name string, value float64, labels map[string]string) {
	t.collector.RecordGauge(name, value, labels)
}
