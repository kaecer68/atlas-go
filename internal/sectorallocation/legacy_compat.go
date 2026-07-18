package sectorallocation

import (
	"log/slog"

	"github.com/kaecer68/atlas-go/internal/config"
)

// LegacyReadCounter 累計 legacy compat 讀取次數，用於 SA12 sunset 證據。
// 觀察期內 counter > 0（compat 還被使用）；sunset 後必須歸 0。
type LegacyReadCounter struct {
	reads   map[string]int64
	sunset  bool
	logOnce map[string]struct{}
}

// NewLegacyCompatCounterForTest 是測試 helper。
// production code 應透過 composition root 注入。
func NewLegacyCompatCounterForTest() *LegacyReadCounter {
	return &LegacyReadCounter{
		reads:   make(map[string]int64),
		logOnce: make(map[string]struct{}),
	}
}

// Inc 累計 caller 讀取次數；同時 per-key log first occurrence。
func (c *LegacyReadCounter) Inc(caller string) {
	if c == nil {
		return
	}
	c.reads[caller]++
	if _, ok := c.logOnce[caller]; !ok {
		c.logOnce[caller] = struct{}{}
		slog.Info("sector_allocation.legacy_compat_read", slog.String("caller", caller), slog.Int64("count", c.reads[caller]))
	}
}

// Snapshot 回傳目前計數（key = caller, value = count）。
func (c *LegacyReadCounter) Snapshot() map[string]int64 {
	if c == nil {
		return map[string]int64{}
	}
	out := make(map[string]int64, len(c.reads))
	for k, v := range c.reads {
		out[k] = v
	}
	return out
}

// SetSunsetGate 設為 true 表示 SA12 啟動 sunset；後續 Inc 仍累計但
// 觀察閉環可讀取此旗標判斷 compat 是否仍可使用。
func (c *LegacyReadCounter) SetSunsetGate(v bool) { c.sunset = v }

// SunsetGate 回傳目前 sunset 旗標。
func (c *LegacyReadCounter) SunsetGate() bool { return c != nil && c.sunset }

// Reset 清空計數；保留 sunset 旗標。
func (c *LegacyReadCounter) Reset() {
	if c == nil {
		return
	}
	c.reads = make(map[string]int64)
	c.logOnce = make(map[string]struct{})
}

// LegacyCompatReader 封裝讀取 BaseAllocations 的單一入口。
// 每次 Read() 都會觸發 counter 與 first-occurrence log，
// 供 SA11 observation 觀察、SA12 sunset 證據使用。
type LegacyCompatReader struct {
	allocations map[string]float64
	counter     *LegacyReadCounter
}

// NewLegacyCompatReader 包裝既有 cfg 與 counter。
func NewLegacyCompatReader(cfg *config.ParametersConfig, counter *LegacyReadCounter) *LegacyCompatReader {
	allocs := map[string]float64{}
	if cfg != nil {
		for k, v := range cfg.Engine.SectorRotation.BaseAllocations.Value {
			allocs[k] = v
		}
	}
	return &LegacyCompatReader{allocations: allocs, counter: counter}
}

// NewLegacyCompatReaderForTest 是測試 helper；用現有 BaseAllocations default。
func NewLegacyCompatReaderForTest(counter *LegacyReadCounter) *LegacyCompatReader {
	return NewLegacyCompatReader(defaultParametersConfigForTest(), counter)
}

// defaultParametersConfigForTest 回傳 default config 供測試建立 reader。
func defaultParametersConfigForTest() *config.ParametersConfig {
	return config.DefaultParametersConfig()
}

// Read 回傳 legacy BaseAllocations 完整 map 並累計 counter 與 log。
func (r *LegacyCompatReader) Read() map[string]float64 {
	if r == nil {
		return map[string]float64{}
	}
	out := make(map[string]float64, len(r.allocations))
	for k, v := range r.allocations {
		out[k] = v
		if r.counter != nil {
			r.counter.Inc("LegacyCompatReader.Read:" + k)
		}
	}
	return out
}

// L1KeysOnly 過濾成 industry.L1Sectors() 的 20 個 ID；非 L1 不得混入 L1 final target。
// （SA-INV-02 / SA-INV-05：non canonical key 不得進入 L1 final target）
func (r *LegacyCompatReader) L1KeysOnly() map[string]float64 {
	return FilterL1Keys(r.Read())
}

// PromotionGate 回傳 sunset 旗標；false 表示 compat 仍合法使用中。
func (r *LegacyCompatReader) PromotionGate() bool {
	return r != nil && r.counter != nil && r.counter.SunsetGate()
}

// FilterL1Keys 是 helper：把任意 map[string]float64 過濾成只有 industry.L1Sectors() 的 20 個 ID。
func FilterL1Keys(in map[string]float64) map[string]float64 {
	out := make(map[string]float64, 20)
	for k, v := range in {
		// 透過 industry 套件判斷是否為 L1 key；SA03 與 SA04 共同使用。
		if isL1KeyForFilter(k) {
			out[k] = v
		}
	}
	return out
}
