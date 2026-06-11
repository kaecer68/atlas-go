package monitoring

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
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
	alertTimestamps    []time.Time // 時間序列用於計算 rate (per hour)；每次 RecordAlert 自動 prune 至 24h 內

	// JSONL persistence
	persistencePath string
	persistMu       sync.Mutex
}

// alertRetentionWindow 保留 alert 時間戳的最大窗口，超過則在 RecordAlert / replayFromFile 階段被 prune。
const alertRetentionWindow = 24 * time.Hour

// NewMetricsCollector 建立 in-memory only 指標收集器
func NewMetricsCollector() *MetricsCollector {
	m, _ := NewMetricsCollectorWithPath("")
	return m
}

// NewMetricsCollectorWithPath 建立持久化指標收集器（path 空字串 = in-memory）
func NewMetricsCollectorWithPath(path string) (*MetricsCollector, error) {
	m := &MetricsCollector{
		metrics:         make(map[string]Metric),
		histograms:      make(map[string][]float64),
		alertsByType:    make(map[string]int64),
		persistencePath: path,
	}
	if path == "" {
		return m, nil
	}
	if err := m.replayFromFile(path); err != nil {
		return nil, fmt.Errorf("replay metrics from %s: %w", path, err)
	}
	return m, nil
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

	m.appendRecord(persistenceRecord{
		Type: "counter", Name: name, Value: value, Labels: labels, Timestamp: time.Now(),
	})
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

	m.appendRecord(persistenceRecord{
		Type: "gauge", Name: name, Value: value, Labels: labels, Timestamp: time.Now(),
	})
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

	m.appendRecord(persistenceRecord{
		Type: "screening", Passed: passed, Rejected: rejected, Timestamp: time.Now(),
	})
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

	now := time.Now()
	m.alertTimestamps = append(m.alertTimestamps, now)
	m.pruneAlertTimestamps(now)

	m.appendRecord(persistenceRecord{
		Type: "alert", AlertType: alertType, Timestamp: now,
	})
}

// pruneAlertTimestamps 移除早於 cutoff 的時間戳（callers 必須持有 m.mu 寫鎖）
func (m *MetricsCollector) pruneAlertTimestamps(now time.Time) {
	cutoff := now.Add(-alertRetentionWindow)
	idx := 0
	for idx < len(m.alertTimestamps) && m.alertTimestamps[idx].Before(cutoff) {
		idx++
	}
	if idx > 0 {
		m.alertTimestamps = m.alertTimestamps[idx:]
	}
}

// countTimestampsInWindowLocked counts alertTimestamps at or after cutoff.
// Caller MUST hold m.mu — sync.RWMutex is non-reentrant.
func (m *MetricsCollector) countTimestampsInWindowLocked(cutoff time.Time) int64 {
	count := int64(0)
	for _, ts := range m.alertTimestamps {
		if !ts.Before(cutoff) {
			count++
		}
	}
	return count
}

// GetAlertTriggerCountInWindow 取得指定時間窗口內的警報觸發數
func (m *MetricsCollector) GetAlertTriggerCountInWindow(window time.Duration) int64 {
	if window <= 0 {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.countTimestampsInWindowLocked(time.Now().Add(-window))
}

// GetAlertTriggerRate 取得警報觸發率（每小時），window 為時間窗口。
// 若 window <= 0，回傳 0；若 window 跨越 < 1 秒，視為 1 秒避免除零
func (m *MetricsCollector) GetAlertTriggerRate(window time.Duration) float64 {
	count := m.GetAlertTriggerCountInWindow(window)
	if window <= time.Second {
		return float64(count)
	}
	hours := window.Hours()
	if hours == 0 {
		return 0
	}
	return float64(count) / hours
}

// RecordAlertAcknowledged 記錄警報確認
func (m *MetricsCollector) RecordAlertAcknowledged() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alertsAcknowledged++

	m.appendRecord(persistenceRecord{
		Type: "ack", Timestamp: time.Now(),
	})
}

// GetAlertTriggerCount 取得警報觸發總數
func (m *MetricsCollector) GetAlertTriggerCount() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return float64(m.alertsTriggered)
}

// GetMetricsSnapshot 取得指標快照
func (m *MetricsCollector) GetMetricsSnapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	alertsByTypeCopy := make(map[string]int64)
	maps.Copy(alertsByTypeCopy, m.alertsByType)

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

// metricKey 生成指標鍵（包含 sorted labels，避免同 name 不同 labels 互相覆蓋）
func metricKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labels[k])
	}
	b.WriteByte('}')
	return b.String()
}

type persistenceRecord struct {
	Type      string            `json:"type"`
	Passed    int64             `json:"passed,omitempty"`
	Rejected  int64             `json:"rejected,omitempty"`
	AlertType string            `json:"alert_type,omitempty"`
	Name      string            `json:"name,omitempty"`
	Value     float64           `json:"value,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

func (m *MetricsCollector) appendRecord(rec persistenceRecord) {
	if m.persistencePath == "" {
		return
	}
	m.persistMu.Lock()
	defer m.persistMu.Unlock()
	f, err := os.OpenFile(m.persistencePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_ = json.NewEncoder(f).Encode(rec)
}

func (m *MetricsCollector) replayFromFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var rec persistenceRecord
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			continue
		}
		m.applyRecord(rec)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	m.pruneAlertTimestamps(time.Now())
	return nil
}

func (m *MetricsCollector) applyRecord(rec persistenceRecord) {
	switch rec.Type {
	case "screening":
		m.screeningTotal += rec.Passed + rec.Rejected
		m.screeningPassed += rec.Passed
		m.screeningRejected += rec.Rejected
	case "alert":
		m.alertsTriggered++
		m.alertsByType[rec.AlertType]++
		m.alertTimestamps = append(m.alertTimestamps, rec.Timestamp)
	case "ack":
		m.alertsAcknowledged++
	case "counter":
		key := metricKey(rec.Name, rec.Labels)
		existing, ok := m.metrics[key]
		if ok {
			existing.Value += rec.Value
			m.metrics[key] = existing
		} else {
			m.metrics[key] = Metric{Name: rec.Name, Value: rec.Value, Type: MetricTypeCounter, Labels: rec.Labels}
		}
	case "gauge":
		key := metricKey(rec.Name, rec.Labels)
		m.metrics[key] = Metric{Name: rec.Name, Value: rec.Value, Type: MetricTypeGauge, Labels: rec.Labels}
	}
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
	tm.collector.RecordCounter("orders_total", 1, map[string]string{
		"symbol": order.Symbol,
		"side":   string(order.Side),
		"status": status,
	})

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

// Start is deprecated. System metrics are collected by the TaskManager
// `metrics_snapshot` task (60s interval) which calls GetMetricsSnapshot().
func (sm *SystemMetrics) Start(ctx context.Context) {
}

type AlertThreshold struct {
	MinScreeningRate        float64
	MaxAlertTriggerRate     float64
	MaxUnacknowledgedAlerts int64
}

func DefaultAlertThreshold() AlertThreshold {
	params := config.GetParametersConfig().Alert
	return AlertThreshold{
		MinScreeningRate:        params.MinScreeningRate.Value,
		MaxAlertTriggerRate:     params.MaxAlertTriggerRate.Value,
		MaxUnacknowledgedAlerts: int64(params.MaxUnacknowledgedAlerts.Value),
	}
}

func (m *MetricsCollector) CheckThresholds(threshold AlertThreshold) []ThresholdViolation {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var violations []ThresholdViolation

	if m.screeningTotal > 0 {
		rate := float64(m.screeningPassed) / float64(m.screeningTotal)
		if rate < threshold.MinScreeningRate {
			violations = append(violations, ThresholdViolation{
				Metric:    "screening_rate",
				Current:   rate,
				Threshold: threshold.MinScreeningRate,
				Severity:  "warning",
				Message:   fmt.Sprintf("篩選率過低: %.1f%% (閾值: %.1f%%)", rate*100, threshold.MinScreeningRate*100),
			})
		}
	}

	// Per-hour alert count：視窗 = 1h，故 count 即為 per-hour rate，
	// 與 threshold.MaxAlertTriggerRate 語意一致。複用 countTimestampsInWindowLocked
	// 以避免與 GetAlertTriggerCountInWindow 重複實作。
	rateCount := m.countTimestampsInWindowLocked(time.Now().Add(-time.Hour))
	if rateCount > int64(threshold.MaxAlertTriggerRate) {
		violations = append(violations, ThresholdViolation{
			Metric:    "alert_trigger_rate",
			Current:   float64(rateCount),
			Threshold: threshold.MaxAlertTriggerRate,
			Severity:  "critical",
			Message:   fmt.Sprintf("警報觸發率過高: %d/hr (閾值: %.0f/hr)", rateCount, threshold.MaxAlertTriggerRate),
		})
	}

	unacknowledged := m.alertsTriggered - m.alertsAcknowledged
	if unacknowledged > threshold.MaxUnacknowledgedAlerts {
		violations = append(violations, ThresholdViolation{
			Metric:    "unacknowledged_alerts",
			Current:   float64(unacknowledged),
			Threshold: float64(threshold.MaxUnacknowledgedAlerts),
			Severity:  "warning",
			Message:   fmt.Sprintf("未確認警報過多: %d (閾值: %d)", unacknowledged, threshold.MaxUnacknowledgedAlerts),
		})
	}

	return violations
}

// ThresholdViolation 閾值違規
type ThresholdViolation struct {
	Metric    string  `json:"metric"`
	Current   float64 `json:"current"`
	Threshold float64 `json:"threshold"`
	Severity  string  `json:"severity"`
	Message   string  `json:"message"`
}

// MetricsHistory 指標歷史記錄
type MetricsHistory struct {
	mu        sync.RWMutex
	snapshots []MetricsSnapshot
	maxSize   int
}

// NewMetricsHistory 建立指標歷史記錄
func NewMetricsHistory(maxSize int) *MetricsHistory {
	return &MetricsHistory{
		snapshots: make([]MetricsSnapshot, 0, maxSize),
		maxSize:   maxSize,
	}
}

// AddSnapshot 添加指標快照
func (h *MetricsHistory) AddSnapshot(snapshot MetricsSnapshot) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.snapshots = append(h.snapshots, snapshot)
	if len(h.snapshots) > h.maxSize {
		h.snapshots = h.snapshots[len(h.snapshots)-h.maxSize:]
	}
}

// GetTrend 取得指標趨勢
func (h *MetricsHistory) GetTrend(metric string) []TrendPoint {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var trend []TrendPoint
	for _, snapshot := range h.snapshots {
		var value float64
		switch metric {
		case "screening_rate":
			value = snapshot.ScreeningRate
		case "alerts_triggered":
			value = float64(snapshot.AlertsTriggered)
		case "alerts_acknowledged":
			value = float64(snapshot.AlertsAcknowledged)
		default:
			continue
		}
		trend = append(trend, TrendPoint{
			Timestamp: snapshot.Timestamp,
			Value:     value,
		})
	}
	return trend
}

// TrendPoint 趨勢點
type TrendPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}
