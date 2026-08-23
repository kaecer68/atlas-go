package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Taiwan5SecIndexBar 代表台股指數 5 秒頻率的單筆資料。
type Taiwan5SecIndexBar struct {
	Date  time.Time `json:"date"`
	TAIEX float64   `json:"taiex"`
}

// Taiwan5SecIndexResponse 為 FinMind 5 秒指數 API 的回應結構。
type Taiwan5SecIndexResponse struct {
	Msg    string               `json:"msg"`
	Status int                  `json:"status"`
	Data   []Taiwan5SecIndexBar `json:"data"`
}

// raw5SecIndexBar 對應 FinMind API 原始 JSON 格式，日期為字串。
type raw5SecIndexBar struct {
	Date  string  `json:"date"`
	TAIEX float64 `json:"TAIEX"`
}

// raw5SecIndexResponse 為 API 回應的原始結構，日期尚未解析。
type raw5SecIndexResponse struct {
	Msg    string            `json:"msg"`
	Status int               `json:"status"`
	Data   []raw5SecIndexBar `json:"data"`
}

var twseLocation = time.FixedZone("CST", 8*60*60) // UTC+8

// FetchTaiwan5SecIndex 向 FinMind 取得指定日期的台股指數 5 秒頻率資料。
//
// Deprecated: P1-11 — 此 API 目前沒有 production caller（僅測試使用）。
// 保留但標記 deprecated，並補上與 fetchDataset 一致的 daily-quota gate
// 與 512B 錯誤體收錄（此前直接繞過 quota gate，可突破 14400/day 上限；
// 若未來仍無 caller，應整組移除）。
func (c *FinMindClient) FetchTaiwan5SecIndex(ctx context.Context, date string) ([]Taiwan5SecIndexBar, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("finmind 5sec index: rate limit wait: %w", ErrRateLimited)
	}
	// P1-11: daily-quota gate — same AllowCall check as fetchDataset so this
	// endpoint cannot bypass the 14400/day ceiling.
	if c.quotaTracker != nil && !c.quotaTracker.AllowCall() {
		return nil, fmt.Errorf("finmind 5sec index: %w (used=%d, remaining=%d)", ErrQuotaExhausted, c.quotaTracker.CallsToday(), c.quotaTracker.Remaining())
	}

	endpoint := fmt.Sprintf("%s/data", finmindBaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("finmind 5sec index: create request: %w", err)
	}

	q := req.URL.Query()
	q.Set("dataset", "TaiwanVariousIndicators5Seconds")
	q.Set("start_date", date)
	req.URL.RawQuery = q.Encode()

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("finmind 5sec index: http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// P1-11: bounded error body (FinMind 512B pattern) — previously the
		// upstream reason was dropped and only the status code surfaced.
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		bodyStr := strings.TrimSpace(string(bodyBytes))
		if bodyStr == "" {
			bodyStr = "(empty body)"
		}
		return nil, fmt.Errorf("finmind 5sec index: status %d, body: %s", resp.StatusCode, bodyStr)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("finmind 5sec index: read body: %w", err)
	}

	return parseTaiwan5SecIndexResponse(body)
}

// parseTaiwan5SecIndexResponse 將 API 回應的 JSON 位元組解析為結構化的指數資料。
func parseTaiwan5SecIndexResponse(data []byte) ([]Taiwan5SecIndexBar, error) {
	var raw raw5SecIndexResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("finmind 5sec index: decode response: %w", err)
	}

	if raw.Status != 200 {
		return nil, fmt.Errorf("finmind 5sec index: API error: %s", raw.Msg)
	}

	bars := make([]Taiwan5SecIndexBar, 0, len(raw.Data))
	for _, item := range raw.Data {
		t, err := parseTWseDatetime(item.Date)
		if err != nil {
			continue
		}
		bars = append(bars, Taiwan5SecIndexBar{
			Date:  t,
			TAIEX: item.TAIEX,
		})
	}

	return bars, nil
}

// parseTWseDatetime 解析台灣證交所日期格式，回傳 Asia/Taipei 時區的 time.Time。
// 支援格式：2006-01-02 15:04:05（標準）、2006-01-02 15:04（省略秒）
func parseTWseDatetime(s string) (time.Time, error) {
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, twseLocation); err == nil {
		return t, nil
	}
	return time.ParseInLocation("2006-01-02 15:04", s, twseLocation)
}

// fiveSecIndexLedgerEntry 為寫入 JSONL 的單筆記錄格式。
type fiveSecIndexLedgerEntry struct {
	Date  string  `json:"date"`
	TAIEX float64 `json:"taiex"`
	Type  string  `json:"type"`
}

// Save5SecIndexToLedger 將 5 秒指數資料寫入 JSONL 檔案。
// 每行格式：{"date": "RFC3339", "taiex": float64, "type": "5sec_index"}
func Save5SecIndexToLedger(bars []Taiwan5SecIndexBar, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("save 5sec index: mkdir: %w", err)
	}

	path := filepath.Join(dir, "taiwan_5sec_index.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("save 5sec index: open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	enc := json.NewEncoder(f)
	for _, bar := range bars {
		entry := fiveSecIndexLedgerEntry{
			Date:  bar.Date.Format(time.RFC3339),
			TAIEX: bar.TAIEX,
			Type:  "5sec_index",
		}
		if err := enc.Encode(entry); err != nil {
			return fmt.Errorf("save 5sec index: encode: %w", err)
		}
	}

	return nil
}
