package monitoring

// quotes_freshness 的單元測試。日期全部釘在 taiwanholidays 實測表（2021–2040）
// 已覆蓋的 2026-10 月——10-02（五）是 2026-10-02 實損（2/117）當天，
// 10-05（一）/ 10-06（二）是「週一早晨 universe build」風險場景。

import (
	"errors"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/taiwanholidays"
)

// taipei 固定 +8，避免測試依賴系統 tz database。
var taipei = time.FixedZone("Asia/Taipei", 8*3600)

func taipeiTime(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, taipei)
}

func dateUTC(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func metricValue(t *testing.T, c *MetricsCollector, name string) (float64, bool) {
	t.Helper()
	m, ok := c.GetMetric(name, nil)
	if !ok {
		return 0, false
	}
	return m.Value, true
}

// 前置 Sanity：本檔所有案例建立在一個事實上——這幾天是/不是交易日。
func TestQuotesFreshness_TaipeiCalendarAssumptions(t *testing.T) {
	trading := []time.Time{
		taipeiTime(2026, 10, 1, 12, 0), // 四
		taipeiTime(2026, 10, 2, 12, 0), // 五（實損當天）
		taipeiTime(2026, 10, 5, 12, 0), // 一
		taipeiTime(2026, 10, 6, 12, 0), // 二
	}
	for _, d := range trading {
		if !taiwanholidays.IsTradingDay(d) {
			t.Fatalf("assumption broken: %s should be a trading day", d.Format("2006-01-02"))
		}
	}
	nonTrading := []time.Time{
		taipeiTime(2026, 10, 3, 12, 0), // 六
		taipeiTime(2026, 10, 4, 12, 0), // 日
	}
	for _, d := range nonTrading {
		if taiwanholidays.IsTradingDay(d) {
			t.Fatalf("assumption broken: %s should NOT be a trading day", d.Format("2006-01-02"))
		}
	}
}

func TestExpectedQuoteDate(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"交易日 cutoff 前：要求前一交易日", taipeiTime(2026, 10, 6, 8, 0), taipeiTime(2026, 10, 5, 0, 0)},
		{"交易日 cutoff 前一分：仍要求前一交易日", taipeiTime(2026, 10, 6, 17, 59), taipeiTime(2026, 10, 5, 0, 0)},
		{"交易日 cutoff 整點起：要求當日", taipeiTime(2026, 10, 6, 18, 0), taipeiTime(2026, 10, 6, 0, 0)},
		{"交易日晚上：要求當日", taipeiTime(2026, 10, 2, 20, 0), taipeiTime(2026, 10, 2, 0, 0)},
		{"週六：要求週五", taipeiTime(2026, 10, 3, 12, 0), taipeiTime(2026, 10, 2, 0, 0)},
		{"週日晚上：仍要求週五", taipeiTime(2026, 10, 4, 23, 0), taipeiTime(2026, 10, 2, 0, 0)},
		{"週一早晨（universe build 場景）：要求週五", taipeiTime(2026, 10, 5, 8, 0), taipeiTime(2026, 10, 2, 0, 0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExpectedQuoteDate(tc.now)
			if !got.Equal(tc.want) {
				t.Fatalf("ExpectedQuoteDate(%s) = %s, want %s",
					tc.now.Format("2006-01-02 15:04"), got.Format("2006-01-02"), tc.want.Format("2006-01-02"))
			}
		})
	}
}

func TestObserveQuotesFreshness_Decisions(t *testing.T) {
	cases := []struct {
		name       string
		maxDate    time.Time
		queryErr   error
		now        time.Time
		wantRun    bool
		wantFresh  bool
		wantReason string
	}{
		{
			name:       "交易日晚上、當日 quotes 已落地 ⇒ 新鮮",
			maxDate:    dateUTC(2026, 10, 2),
			now:        taipeiTime(2026, 10, 2, 20, 0),
			wantRun:    true,
			wantFresh:  true,
			wantReason: "fresh",
		},
		{
			name:       "10-02 實損形狀：交易日晚上 max(date) 仍停在前一交易日 ⇒ 不新鮮",
			maxDate:    dateUTC(2026, 10, 1),
			now:        taipeiTime(2026, 10, 2, 20, 0),
			wantRun:    true,
			wantFresh:  false,
			wantReason: "max_date_before_expected",
		},
		{
			name:       "週一早晨 universe build 場景：週五 quotes 還在 ⇒ 新鮮",
			maxDate:    dateUTC(2026, 10, 2),
			now:        taipeiTime(2026, 10, 5, 8, 0),
			wantRun:    true,
			wantFresh:  true,
			wantReason: "fresh",
		},
		{
			name:       "週一早晨、連週五 quotes 都缺席 ⇒ 不新鮮（build 之前就該觸發）",
			maxDate:    dateUTC(2026, 10, 1),
			now:        taipeiTime(2026, 10, 5, 8, 0),
			wantRun:    true,
			wantFresh:  false,
			wantReason: "max_date_before_expected",
		},
		{
			name:       "週末：max(date)=週五 ⇒ 新鮮（不誤報）",
			maxDate:    dateUTC(2026, 10, 2),
			now:        taipeiTime(2026, 10, 4, 15, 0),
			wantRun:    true,
			wantFresh:  true,
			wantReason: "fresh",
		},
		{
			name:       "查詢失敗 ⇒ run_ok=0、fail-closed 不新鮮",
			maxDate:    time.Time{},
			queryErr:   errors.New("connection reset"),
			now:        taipeiTime(2026, 10, 2, 20, 0),
			wantRun:    false,
			wantFresh:  false,
			wantReason: "query_failed",
		},
		{
			name:       "空表 ⇒ 可評估但不新鮮（quotes_table_empty）",
			maxDate:    time.Time{},
			now:        taipeiTime(2026, 10, 2, 20, 0),
			wantRun:    true,
			wantFresh:  false,
			wantReason: "quotes_table_empty",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewMetricsCollector()
			obs := ObserveQuotesFreshness(c, tc.maxDate, tc.queryErr, tc.now)
			if obs.RunOK != tc.wantRun {
				t.Errorf("RunOK = %v, want %v", obs.RunOK, tc.wantRun)
			}
			if obs.Fresh != tc.wantFresh {
				t.Errorf("Fresh = %v, want %v", obs.Fresh, tc.wantFresh)
			}
			if obs.Reason != tc.wantReason {
				t.Errorf("Reason = %q, want %q", obs.Reason, tc.wantReason)
			}

			runOK, ok := metricValue(t, c, MetricQuotesFreshnessRunOK)
			if !ok || runOK != boolFloat(tc.wantRun) {
				t.Errorf("%s = %v (ok=%v), want %v", MetricQuotesFreshnessRunOK, runOK, ok, boolFloat(tc.wantRun))
			}
			freshV, ok := metricValue(t, c, MetricQuotesFreshnessOK)
			if !ok || freshV != boolFloat(tc.wantFresh) {
				t.Errorf("%s = %v (ok=%v), want %v", MetricQuotesFreshnessOK, freshV, ok, boolFloat(tc.wantFresh))
			}
			checked, ok := metricValue(t, c, MetricQuotesFreshnessCheckedTimestamp)
			if !ok || checked != float64(tc.now.Unix()) {
				t.Errorf("%s = %v (ok=%v), want %d", MetricQuotesFreshnessCheckedTimestamp, checked, ok, tc.now.Unix())
			}

			maxV, maxOK := metricValue(t, c, MetricQuotesMaxDateTimestamp)
			if tc.wantRun && !tc.maxDate.IsZero() {
				// max_date 以「該日期 Asia/Taipei 午夜」的 Unix 秒輸出。
				d := tc.maxDate.In(taipei)
				midnight := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, taipei)
				if !maxOK || maxV != float64(midnight.Unix()) {
					t.Errorf("%s = %v (ok=%v), want %d", MetricQuotesMaxDateTimestamp, maxV, maxOK, midnight.Unix())
				}
			} else if tc.queryErr != nil {
				// 查詢失敗：不得輸出（假造）max_date；留下的是凍結值語意由 _run_ok 守。
				if maxOK {
					t.Errorf("%s should not be emitted on query failure, got %v", MetricQuotesMaxDateTimestamp, maxV)
				}
			}
		})
	}
}

// 凍結樣本語意的行為釘：查詢失敗後 max_date 序列若先前存在（舊值）不會被清掉
// （gauge last-write-wins、不清序列），但 _run_ok=0 是讀它的前置條件。
func TestObserveQuotesFreshness_FrozenMaxDateSemantics(t *testing.T) {
	c := NewMetricsCollector()
	// 第一輪成功：max_date 存在。
	ObserveQuotesFreshness(c, dateUTC(2026, 10, 2), nil, taipeiTime(2026, 10, 2, 20, 0))
	if _, ok := c.GetMetric(MetricQuotesMaxDateTimestamp, nil); !ok {
		t.Fatalf("%s should exist after a successful round", MetricQuotesMaxDateTimestamp)
	}
	// 第二輪查詢失敗：max_date 舊值**仍在**（不清序列），但 run_ok 翻 0。
	obs := ObserveQuotesFreshness(c, time.Time{}, errors.New("down"), taipeiTime(2026, 10, 2, 20, 5))
	if _, ok := c.GetMetric(MetricQuotesMaxDateTimestamp, nil); !ok {
		t.Fatalf("%s must survive (frozen) after a failed round — clearing it would hide staleness", MetricQuotesMaxDateTimestamp)
	}
	if obs.RunOK {
		t.Fatalf("RunOK should be false after query failure")
	}
}

func boolFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
