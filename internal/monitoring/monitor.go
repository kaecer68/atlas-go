package monitoring

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// AlertLevel 告警级别
type AlertLevel int

const (
	AlertLevelInfo AlertLevel = iota
	AlertLevelWarning
	AlertLevelError
	AlertLevelCritical
)

func (l AlertLevel) String() string {
	switch l {
	case AlertLevelInfo:
		return "INFO"
	case AlertLevelWarning:
		return "WARNING"
	case AlertLevelError:
		return "ERROR"
	case AlertLevelCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// Alert 告警事件
type Alert struct {
	ID        string
	Level     AlertLevel
	Category  string
	Message   string
	Timestamp time.Time
	Metadata  map[string]any
}

// AlertHandler 告警处理器
type AlertHandler func(alert Alert)

// Monitor 监控系统
type Monitor struct {
	handlers   []AlertHandler
	mu         sync.RWMutex
	history    []Alert
	maxHistory int

	alertStore *AlertStore
	notifiers  []Notifier

	// Phase 2A: dedup, auto-handler, regime suppression
	deduplicator  *AlertDeduplicator
	autoHandler   *AutoHandler
	currentRegime domain.Regime
}

// NewMonitor 创建监控系统
func NewMonitor() *Monitor {
	return &Monitor{
		handlers:   make([]AlertHandler, 0),
		history:    make([]Alert, 0, 1000),
		maxHistory: 1000,
	}
}

// RegisterHandler 注册告警处理器
func (m *Monitor) RegisterHandler(handler AlertHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers = append(m.handlers, handler)
}

func (m *Monitor) Alert(level AlertLevel, category string, message string, metadata map[string]any) {
	m.alertWithBreakdown(level, category, message, metadata, nil)
}

func (m *Monitor) AlertWithBreakdown(level AlertLevel, category string, message string, metadata map[string]any, breakdown *domain.AlertBreakdown) {
	m.alertWithBreakdown(level, category, message, metadata, breakdown)
}

func (m *Monitor) alertWithBreakdown(level AlertLevel, category string, message string, metadata map[string]any, breakdown *domain.AlertBreakdown) {
	alert := Alert{
		ID:        generateAlertID(),
		Level:     level,
		Category:  category,
		Message:   message,
		Timestamp: time.Now(),
		Metadata:  metadata,
	}

	m.mu.Lock()
	m.history = append(m.history, alert)
	if len(m.history) > m.maxHistory {
		m.history = m.history[len(m.history)-m.maxHistory:]
	}
	handlers := make([]AlertHandler, len(m.handlers))
	copy(handlers, m.handlers)
	store := m.alertStore
	notifiers := make([]Notifier, len(m.notifiers))
	copy(notifiers, m.notifiers)
	dedup := m.deduplicator
	ah := m.autoHandler
	regime := m.currentRegime
	m.mu.Unlock()

	go func() {
		for _, handler := range handlers {
			go handler(alert)
		}
	}()

	// RISK_OFF suppression: skip save and notify for INFO/WARNING
	if regime == domain.RegimeRiskOff && (level == AlertLevelInfo || level == AlertLevelWarning) {
		if ah != nil {
			ah.Handle(alert)
		}
		return
	}

	record := domain.AlertRecord{
		ID:        alert.ID,
		Timestamp: alert.Timestamp,
		Rule:      category,
		Severity:  level.String(),
		Message:   message,
		Breakdown: breakdown,
		Status:    domain.AlertStatusTriggered,
		Count:     1,
	}

	// Dedup check: skip save+notify if duplicate within window
	if dedup != nil {
		dedupKey := category + ":" + level.String()
		record.DedupKey = dedupKey
		result, err := dedup.Check(dedupKey)
		if err == nil && result.Skip {
			if result.ExistingAlertID != "" && store != nil {
				_ = store.Update(result.ExistingAlertID, func(r *domain.AlertRecord) {
					r.Count = result.NewCount
					r.LastSeen = &record.Timestamp
				})
			}
			return
		}
		// Track immediately to close the race window: previously
		// Track was called async (in the save goroutine), so two
		// concurrent calls could both pass Check before either
		// goroutine ran Track.
		dedup.Track(record.DedupKey)
	}

	if store != nil {
		go func() {
			if err := store.Save(record); err != nil {
				logging.Warn("monitor", "alert_save_failed", logging.Err(err))
				return
			}
			// Auto-handler MUST run AFTER save completes, otherwise
			// auto-acknowledge will race with the save goroutine.
			if ah != nil {
				ah.Handle(alert)
			}
			// Notifiers run after save so the alert exists before
			// external systems try to query it.
			for _, n := range notifiers {
				if !n.IsConfigured() {
					continue
				}
				go func(notif Notifier) {
					if err := notif.Notify(record); err != nil {
						logging.Warn("monitor", "notify_failed", logging.FStr("notifier", notif.Name()), logging.Err(err))
					}
				}(n)
			}
		}()
	} else {
		// No persistent store: run auto-handler inline.
		if ah != nil {
			ah.Handle(alert)
		}
		for _, n := range notifiers {
			if !n.IsConfigured() {
				continue
			}
			go func(notif Notifier) {
				if err := notif.Notify(record); err != nil {
					logging.Warn("monitor", "notify_failed", logging.FStr("notifier", notif.Name()), logging.Err(err))
				}
			}(n)
		}
	}
}

// Info 发送信息级别告警
func (m *Monitor) Info(category string, message string, metadata map[string]any) {
	m.Alert(AlertLevelInfo, category, message, metadata)
}

// Warning 发送警告级别告警
func (m *Monitor) Warning(category string, message string, metadata map[string]any) {
	m.Alert(AlertLevelWarning, category, message, metadata)
}

// Error 发送错误级别告警
func (m *Monitor) Error(category string, message string, metadata map[string]any) {
	m.Alert(AlertLevelError, category, message, metadata)
}

// Critical 发送严重级别告警
func (m *Monitor) Critical(category string, message string, metadata map[string]any) {
	m.Alert(AlertLevelCritical, category, message, metadata)
}

// SetAlertStore sets the persistent alert store.
func (m *Monitor) SetAlertStore(store *AlertStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alertStore = store
}

// SetDeduplicator sets the alert deduplicator for 5-min window dedup.
func (m *Monitor) SetDeduplicator(d *AlertDeduplicator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deduplicator = d
}

// SetAutoHandler sets the auto-handler for severity-based routing.
func (m *Monitor) SetAutoHandler(h *AutoHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoHandler = h
}

// SetRegime sets the current market regime for suppression logic.
func (m *Monitor) SetRegime(r domain.Regime) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentRegime = r
}

// CurrentRegime returns the current market regime.
func (m *Monitor) CurrentRegime() domain.Regime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentRegime
}

// AddNotifier adds a notification dispatcher.
func (m *Monitor) AddNotifier(n Notifier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifiers = append(m.notifiers, n)
}

// GetHistory 获取告警历史
func (m *Monitor) GetHistory(limit int) []Alert {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > len(m.history) {
		limit = len(m.history)
	}

	// 返回最新的记录
	start := max(len(m.history)-limit, 0)

	result := make([]Alert, limit)
	copy(result, m.history[start:])
	return result
}

// GetHistoryByLevel 获取特定级别的告警历史
func (m *Monitor) GetHistoryByLevel(level AlertLevel, limit int) []Alert {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []Alert
	for i := len(m.history) - 1; i >= 0 && len(result) < limit; i-- {
		if m.history[i].Level == level {
			result = append(result, m.history[i])
		}
	}
	return result
}

// generateAlertID 生成告警ID（UUID格式，確保與資料庫UUID欄位相容）
func generateAlertID() string {
	return uuid.New().String()
}

// ConsoleHandler 控制台输出处理器
func ConsoleHandler(alert Alert) {
	prefix := fmt.Sprintf("[%s] [%s] %s:", alert.Timestamp.Format("15:04:05"), alert.Level.String(), alert.Category)
	switch alert.Level {
	case AlertLevelCritical:
		logging.Error("monitor", "alert", "severity", "critical", "category", alert.Category, "message", alert.Message)
		fmt.Printf("\033[31m%s %s\033[0m\n", prefix, alert.Message)
	case AlertLevelError:
		logging.Error("monitor", "alert", "severity", "error", "category", alert.Category, "message", alert.Message)
		fmt.Printf("\033[31m%s %s\033[0m\n", prefix, alert.Message)
	case AlertLevelWarning:
		logging.Warn("monitor", "alert", "severity", "warning", "category", alert.Category, "message", alert.Message)
		fmt.Printf("\033[33m%s %s\033[0m\n", prefix, alert.Message)
	default:
		logging.Info("monitor", "alert", "severity", "info", "category", alert.Category, "message", alert.Message)
		fmt.Printf("%s %s\n", prefix, alert.Message)
	}
}

// CompositeMonitor 组合多个监控器
type CompositeMonitor struct {
	monitors []*Monitor
}

// NewCompositeMonitor 创建组合监控器
func NewCompositeMonitor(monitors ...*Monitor) *CompositeMonitor {
	return &CompositeMonitor{monitors: monitors}
}

// Alert 向所有监控器发送告警
func (c *CompositeMonitor) Alert(level AlertLevel, category string, message string, metadata map[string]any) {
	for _, m := range c.monitors {
		m.Alert(level, category, message, metadata)
	}
}

// Info 发送信息级别告警
func (c *CompositeMonitor) Info(category string, message string, metadata map[string]any) {
	c.Alert(AlertLevelInfo, category, message, metadata)
}

// Warning 发送警告级别告警
func (c *CompositeMonitor) Warning(category string, message string, metadata map[string]any) {
	c.Alert(AlertLevelWarning, category, message, metadata)
}

// Error 发送错误级别告警
func (c *CompositeMonitor) Error(category string, message string, metadata map[string]any) {
	c.Alert(AlertLevelError, category, message, metadata)
}

// Critical 发送严重级别告警
func (c *CompositeMonitor) Critical(category string, message string, metadata map[string]any) {
	c.Alert(AlertLevelCritical, category, message, metadata)
}
