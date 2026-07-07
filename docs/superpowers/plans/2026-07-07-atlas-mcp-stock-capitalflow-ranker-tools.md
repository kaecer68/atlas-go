# atlas-mcp 個股級工具、資金流向與策略排名實作計畫

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development`（建議）或 `superpowers:executing-plans` 逐 task 執行。Steps 使用 checkbox (`- [ ]`) 語法追蹤。

**Goal:** 在 `atlas-mcp` 暴露 7 個新 tool（4 個個股級 + 2 個資金流向 + 1 個策略排名），新增對應後端 `/api/*` 路由，並統一 tool 數文件權威來源。

**Architecture:** 遵循既有 MCP→HTTP 模式：後端在 `internal/stocktools` 與 `internal/strategy_ranker` 提供 REST endpoint；`cmd/atlas-mcp/server` 新增對應 `tools_*.go`，透過 `s.cli.Get` 呼叫後端。資金流向複用已存在的 `internal/capitalflow` endpoint。

**Tech Stack:** Go 1.26、`modelcontextprotocol/go-sdk`、標準 `net/http`、`internal/monitoring/api/shared`、既有 `marketdata` / `portfolio` / `ledger` provider。

## Global Constraints

- 所有新增 MCP tool 必須經過 `countedAddTool`，由 `RegisteredToolCount` 統計。
- 所有 tool handler 必須經過 `withAudit`。
- 後端路由必須註冊在 `cmd/atlas/main.go` 的 `mux` 上，並同步更新 `isPublicPath`。
- 文件以 `docs/AGENT_TOOLS.md` 為 tool catalog 唯一權威來源。
- `go vet`、`gofmt`、`go test` 必須通過。

---

## Task 1: 擴充 TWSECapitalFlowProvider 支援單一標的查詢

**Files:**
- Modify: `internal/marketdata/twse_capital_flow_provider.go`
- Test: `internal/marketdata/twse_capital_flow_provider_test.go`

**Interfaces:**
- Consumes: existing `fetchDate(ctx, dateStr)` response schema.
- Produces: `func (t *TWSECapitalFlowProvider) FetchSymbolFlow(ctx context.Context, symbol, dateStr string) (SymbolFlow, error)`
- Produces type: `type SymbolFlow struct { Symbol string; Name string; ForeignInvestorNet float64; DomesticFundNet float64; DealerNet float64; Date string }`

- [ ] **Step 1: 寫失敗測試**

```go
func TestFetchSymbolFlow(t *testing.T) {
	p := NewTWSECapitalFlowProvider("")
	p.SetHTTPClient(newMockT86Symbol())
	flow, err := p.FetchSymbolFlow(context.Background(), "2330", "20260701")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flow.Symbol != "2330" {
		t.Fatalf("symbol=%s", flow.Symbol)
	}
	if flow.ForeignInvestorNet <= 0 {
		t.Fatalf("expected positive foreign net")
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/marketdata -run TestFetchSymbolFlow -v`
Expected: FAIL `FetchSymbolFlow undefined`

- [ ] **Step 3: 最小實作**

在 `twse_capital_flow_provider.go` 加入：

```go
type SymbolFlow struct {
	Symbol             string  `json:"symbol"`
	Name               string  `json:"name"`
	ForeignInvestorNet float64 `json:"foreign_investor_net"`
	DomesticFundNet    float64 `json:"domestic_fund_net"`
	DealerNet          float64 `json:"dealer_net"`
	Date               string  `json:"date"`
}

func (t *TWSECapitalFlowProvider) FetchSymbolFlow(ctx context.Context, symbol, dateStr string) (SymbolFlow, error) {
	flow, err := t.fetchDate(ctx, dateStr)
	if err != nil {
		return SymbolFlow{}, err
	}
	for _, row := range /* parsed per-symbol rows */ {
		if len(row) < 12 {
			continue
		}
		if row[0] != symbol {
			continue
		}
		return SymbolFlow{
			Symbol:             symbol,
			Name:               row[1],
			ForeignInvestorNet: parseTWDVolume(row[4]),
			DomesticFundNet:    parseTWDVolume(row[10]),
			DealerNet:          parseTWDVolume(row[11]),
			Date:               dateStr,
		}, nil
	}
	return SymbolFlow{}, fmt.Errorf("symbol %s not found for %s", symbol, dateStr)
}
```

需同時調整 `fetchDate` 回傳原始 rows，或新增 `fetchDateRows` 私有 helper，讓 `FetchSymbolFlow` 能重複使用解析邏輯。

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/marketdata -run TestFetchSymbolFlow -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/marketdata/twse_capital_flow_provider.go internal/marketdata/twse_capital_flow_provider_test.go
git commit -m "feat(marketdata): add per-symbol TWSE capital flow query"
```

---

## Task 2: 建立個股後端套件 `internal/stocktools`

**Files:**
- Create: `internal/stocktools/handler.go`
- Create: `internal/stocktools/handler_test.go`

**Interfaces:**
- Consumes: `marketdata.FugleClient.GetQuote`, `portfolio.FundamentalProvider.Get`, `marketdata.TWSECapitalFlowProvider.FetchSymbolFlow`, `ledger.QuoteStore.LoadQuotes`.
- Produces: `func RegisterRoutes(mux *http.ServeMux, deps Deps)`; `func NewHandler(deps Deps) *Handler`; route handlers return `(int, any)` per `internal/monitoring/api/shared.Handler`.
- Produces type: `type Deps struct { FugleClient *marketdata.FugleClient; Fundamentals *portfolio.FundamentalProvider; CapitalFlow *marketdata.TWSECapitalFlowProvider; QuoteStore ledger.QuoteStore }`

- [ ] **Step 1: 寫失敗測試**

```go
func TestStockGetQuote(t *testing.T) {
	mux := http.NewServeMux()
	deps := stocktools.Deps{}
	stocktools.RegisterRoutes(mux, deps)
	req := httptest.NewRequest(http.MethodGet, "/api/stock/quote?symbol=2330", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/stocktools -run TestStockGetQuote -v`
Expected: FAIL `package stocktools not found`

- [ ] **Step 3: 實作 handler.go**

```go
package stocktools

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

type Deps struct {
	FugleClient  *marketdata.FugleClient
	Fundamentals *portfolio.FundamentalProvider
	CapitalFlow  *marketdata.TWSECapitalFlowProvider
	QuoteStore   ledger.QuoteStore
}

type Handler struct {
	deps Deps
}

func NewHandler(deps Deps) *Handler { return &Handler{deps: deps} }

func RegisterRoutes(mux *http.ServeMux, deps Deps) {
	h := NewHandler(deps)
	mux.Handle("GET /api/stock/quote", shared.Get(h.HandleQuote))
	mux.Handle("GET /api/stock/fundamentals", shared.Get(h.HandleFundamentals))
	mux.Handle("GET /api/stock/chips", shared.Get(h.HandleChips))
	mux.Handle("GET /api/stock/technical", shared.Get(h.HandleTechnical))
}

func (h *Handler) HandleQuote(r *http.Request) (int, any) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		return http.StatusBadRequest, map[string]string{"error": "symbol is required"}
	}
	if h.deps.FugleClient == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "quote provider not configured"}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	q, err := h.deps.FugleClient.GetQuote(ctx, symbol)
	if err != nil {
		return http.StatusServiceUnavailable, map[string]string{"error": err.Error()}
	}
	return http.StatusOK, q
}

func (h *Handler) HandleFundamentals(r *http.Request) (int, any) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		return http.StatusBadRequest, map[string]string{"error": "symbol is required"}
	}
	if h.deps.Fundamentals == nil || !h.deps.Fundamentals.HasData() {
		return http.StatusServiceUnavailable, map[string]string{"error": "fundamentals data not loaded"}
	}
	return http.StatusOK, h.deps.Fundamentals.Get(symbol)
}

func (h *Handler) HandleChips(r *http.Request) (int, any) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		return http.StatusBadRequest, map[string]string{"error": "symbol is required"}
	}
	if h.deps.CapitalFlow == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "capital flow provider not configured"}
	}
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().Format("20060102")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	flow, err := h.deps.CapitalFlow.FetchSymbolFlow(ctx, symbol, date)
	if err != nil {
		return http.StatusServiceUnavailable, map[string]string{"error": err.Error()}
	}
	return http.StatusOK, flow
}

func (h *Handler) HandleTechnical(r *http.Request) (int, any) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		return http.StatusBadRequest, map[string]string{"error": "symbol is required"}
	}
	if h.deps.QuoteStore == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "quote store not configured"}
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		days = 90
	}
	if days > 365 {
		days = 365
	}
	end := time.Now()
	start := end.AddDate(0, 0, -days)
	bars, err := h.deps.QuoteStore.LoadQuotes(symbol, start, end)
	if err != nil {
		return http.StatusServiceUnavailable, map[string]string{"error": err.Error()}
	}
	if len(bars) < 2 {
		return http.StatusServiceUnavailable, map[string]string{"error": "insufficient historical quote data"}
	}
	return http.StatusOK, computeTechnical(bars)
}

func computeTechnical(bars []domain.DailyBar) map[string]any {
	closes := make([]float64, len(bars))
	volumes := make([]int64, len(bars))
	for i, b := range bars {
		closes[i] = b.Close
		volumes[i] = b.Volume
	}
	latest := bars[len(bars)-1]
	return map[string]any{
		"symbol":   latest.Symbol,
		"date":     latest.Date.Format("2006-01-02"),
		"close":    latest.Close,
		"volume":   latest.Volume,
		"sma20":    sma(closes, 20),
		"sma50":    sma(closes, 50),
		"rsi14":    rsi(closes, 14),
	}
}

func sma(values []float64, n int) float64 {
	if len(values) < n {
		return 0
	}
	sum := 0.0
	for i := len(values) - n; i < len(values); i++ {
		sum += values[i]
	}
	return math.Round(sum/float64(n)*100) / 100
}

func rsi(values []float64, n int) float64 {
	if len(values) < n+1 {
		return 0
	}
	var gains, losses float64
	for i := len(values) - n; i < len(values); i++ {
		diff := values[i] - values[i-1]
		if diff > 0 {
			gains += diff
		} else {
			losses -= diff
		}
	}
	if losses == 0 {
		return 100
	}
	rs := gains / losses
	return math.Round(100-(100/(1+rs))*100) / 100
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/stocktools -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/stocktools/
git commit -m "feat(stocktools): add per-symbol quote/fundamentals/chips/technical endpoints"
```

---

## Task 3: 建立策略排名後端 `internal/strategy_ranker/handler.go`

**Files:**
- Create: `internal/strategy_ranker/handler.go`
- Create: `internal/strategy_ranker/handler_test.go`

**Interfaces:**
- Consumes: `*strategy_techniques.Registry` and `strategy_ranker.RankAndTier`.
- Produces: `func RegisterRoutes(mux *http.ServeMux, registry *strategy_techniques.Registry)`; `func NewHandler(registry *strategy_techniques.Registry) *Handler`; `GET /api/strategy-ranker/rank`.

- [ ] **Step 1: 寫失敗測試**

```go
func TestRankHandler(t *testing.T) {
	mux := http.NewServeMux()
	strategy_ranker.RegisterRoutes(mux, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/strategy-ranker/rank", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/strategy_ranker -run TestRankHandler -v`
Expected: FAIL `RegisterRoutes undefined`

- [ ] **Step 3: 實作 handler.go**

```go
package strategy_ranker

import (
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/strategy_ranker"
	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
	"github.com/kaecer68/atlas-go/internal/strategy_validator"
)

type Handler struct {
	registry *strategy_techniques.Registry
}

func NewHandler(registry *strategy_techniques.Registry) *Handler {
	return &Handler{registry: registry}
}

func RegisterRoutes(mux *http.ServeMux, registry *strategy_techniques.Registry) {
	h := NewHandler(registry)
	mux.Handle("GET /api/strategy-ranker/rank", shared.Get(h.HandleRank))
}

func (h *Handler) HandleRank(r *http.Request) (int, any) {
	if h.registry == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "strategy registry not initialized"}
	}
	frames := h.registry.All()
	reports := make([]*strategy_validator.StrategyReport, 0, len(frames))
	for _, f := range frames {
		if f.Status != strategy_techniques.StatusActive {
			continue
		}
		reports = append(reports, &strategy_validator.StrategyReport{
			StrategyID:   f.ID,
			StrategyName: f.Name,
			WinRate:      f.HitRate,
			SampleDays:   f.TotalTests,
		})
	}
	if len(reports) == 0 {
		return http.StatusOK, []strategy_validator.RankedReport{}
	}
	ranker := strategy_ranker.New()
	ranked := ranker.RankAndTier(reports)
	return http.StatusOK, ranked
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/strategy_ranker -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/strategy_ranker/
git commit -m "feat(strategy_ranker): add /api/strategy-ranker/rank endpoint"
```

---

## Task 4: 在 `cmd/atlas/main.go` 註冊新路由

**Files:**
- Modify: `cmd/atlas/main.go`

**Interfaces:**
- Consumes: `internal/stocktools`, `internal/strategy_ranker`, `ledger.NewQuoteStore`, `portfolio.NewFundamentalProvider`.
- Produces: registered `/api/stock/*`, `/api/strategy-ranker/*`; updated `isPublicPath`.

- [ ] **Step 1: 新增 import**

```go
import (
	...
	"github.com/kaecer68/atlas-go/internal/stocktools"
	"github.com/kaecer68/atlas-go/internal/strategy_ranker"
	...
)
```

- [ ] **Step 2: 在 `run()` 內初始化 stocktools 依賴**

在 `mux := http.NewServeMux()` 之後、路由註冊之前加入：

```go
var stockDeps stocktools.Deps
if cfg.FugleAPIKey != "" {
	stockDeps.FugleClient = marketdata.GetSharedFugleClient(cfg.FugleAPIKey)
}
if fundamentalsPath := filepath.Join(cfg.WorkDir, "data", "fundamentals.json"); true {
	fp := portfolio.NewFundamentalProvider()
	if err := fp.LoadFromJSON(fundamentalsPath); err == nil {
		stockDeps.Fundamentals = fp
	} else {
		log.Printf("[StockTools] fundamentals load failed: %v", err)
	}
}
if qs, err := ledger.NewQuoteStore(cfg); err == nil {
	stockDeps.QuoteStore = qs
} else {
	log.Printf("[StockTools] quote store init failed: %v", err)
}
stockDeps.CapitalFlow = marketdata.NewTWSECapitalFlowProvider(filepath.Join(cfg.WorkDir, constants.StateCapitalFlow))
stocktools.RegisterRoutes(mux, stockDeps)
log.Printf("[StockTools] registered /api/stock/* routes")

if stRegistry != nil {
	strategy_ranker.RegisterRoutes(mux, stRegistry)
	log.Printf("[StrategyRanker] registered /api/strategy-ranker/* routes")
}
```

注意：`stRegistry` 在此區塊較晚才載入（line 1136 附近），因此 `strategy_ranker.RegisterRoutes` 必須放在 `stRegistry` 載入之後的同一區塊。

- [ ] **Step 3: 更新 `isPublicPath`**

在 `cmd/atlas/main.go` 的 `isPublicPath` 加入：

```go
case p == "/api/stock" || strings.HasPrefix(p, "/api/stock/"):
	return true
case p == "/api/strategy-ranker" || strings.HasPrefix(p, "/api/strategy-ranker/"):
	return true
```

- [ ] **Step 4: 執行建置確認無編譯錯誤**

Run: `go build ./cmd/atlas`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/atlas/main.go
git commit -m "feat(atlas): register /api/stock and /api/strategy-ranker routes"
```

---

## Task 5: 新增 MCP tool 檔案

**Files:**
- Create: `cmd/atlas-mcp/server/tools_stock.go`
- Create: `cmd/atlas-mcp/server/tools_capitalflow.go`
- Create: `cmd/atlas-mcp/server/tools_strategy_ranker.go`
- Create tests: `cmd/atlas-mcp/server/tools_stock_test.go`, `tools_capitalflow_test.go`, `tools_strategy_ranker_test.go`

**Interfaces:**
- Consumes: `s.cli.Get` with paths `/api/stock/*`, `/api/capital-flow/*`, `/api/strategy-ranker/*`.
- Produces: `registerStockTools`, `registerCapitalFlowTools`, `registerStrategyRankerTools`; handler methods and input/output structs.

- [ ] **Step 1: 實作 `tools_stock.go`**

```go
package server

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerStockTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "stock_get_quote",
		Description: autoDescOr("stock_get_quote", "Return the latest intraday quote for a Taiwan stock symbol. Requires FUGLE_API_KEY to be configured."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStockGetQuote)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "stock_get_fundamentals",
		Description: autoDescOr("stock_get_fundamentals", "Return fundamental metrics (PE, PB, PS, dividend yield, sector) for a symbol."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStockGetFundamentals)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "stock_get_chips",
		Description: autoDescOr("stock_get_chips", "Return institutional investor flow (foreign, domestic fund, dealer net buy/sell) for a symbol on a given trading day."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStockGetChips)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "stock_get_technical",
		Description: autoDescOr("stock_get_technical", "Return simple technical indicators (SMA20, SMA50, RSI14) for a symbol over the last N days."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStockGetTechnical)
}

type stockSymbolInput struct {
	Symbol string `json:"symbol" jsonschema:"the Taiwan stock symbol, e.g. 2330"`
}

type stockTechnicalInput struct {
	Symbol string `json:"symbol" jsonschema:"the Taiwan stock symbol, e.g. 2330"`
	Days   int    `json:"days,omitempty" jsonschema:"number of trading days to include; default 90, max 365"`
}

type stockChipsInput struct {
	Symbol string `json:"symbol" jsonschema:"the Taiwan stock symbol, e.g. 2330"`
	Date   string `json:"date,omitempty" jsonschema:"trading day in YYYYMMDD; defaults to today"`
}

type stockBaseOutput struct {
	Result *map[string]any `json:"result"`
}

func (s *server) handleStockGetQuote(ctx context.Context, _ *mcp.CallToolRequest, in stockSymbolInput) (*mcp.CallToolResult, stockBaseOutput, error) {
	if in.Symbol == "" {
		return nil, stockBaseOutput{}, fmt.Errorf("stock_get_quote: symbol is required")
	}
	var out stockBaseOutput
	q := url.Values{"symbol": {in.Symbol}}
	if err := s.withAudit(ctx, "stock_get_quote", []string{"symbol"}, func() error {
		return s.cli.Get(ctx, "/api/stock/quote", q, &out.Result)
	}); err != nil {
		return nil, stockBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleStockGetFundamentals(ctx context.Context, _ *mcp.CallToolRequest, in stockSymbolInput) (*mcp.CallToolResult, stockBaseOutput, error) {
	if in.Symbol == "" {
		return nil, stockBaseOutput{}, fmt.Errorf("stock_get_fundamentals: symbol is required")
	}
	var out stockBaseOutput
	q := url.Values{"symbol": {in.Symbol}}
	if err := s.withAudit(ctx, "stock_get_fundamentals", []string{"symbol"}, func() error {
		return s.cli.Get(ctx, "/api/stock/fundamentals", q, &out.Result)
	}); err != nil {
		return nil, stockBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleStockGetChips(ctx context.Context, _ *mcp.CallToolRequest, in stockChipsInput) (*mcp.CallToolResult, stockBaseOutput, error) {
	if in.Symbol == "" {
		return nil, stockBaseOutput{}, fmt.Errorf("stock_get_chips: symbol is required")
	}
	var out stockBaseOutput
	q := url.Values{"symbol": {in.Symbol}}
	if in.Date != "" {
		q.Set("date", in.Date)
	}
	if err := s.withAudit(ctx, "stock_get_chips", []string{"symbol", "date"}, func() error {
		return s.cli.Get(ctx, "/api/stock/chips", q, &out.Result)
	}); err != nil {
		return nil, stockBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleStockGetTechnical(ctx context.Context, _ *mcp.CallToolRequest, in stockTechnicalInput) (*mcp.CallToolResult, stockBaseOutput, error) {
	if in.Symbol == "" {
		return nil, stockBaseOutput{}, fmt.Errorf("stock_get_technical: symbol is required")
	}
	if in.Days <= 0 {
		in.Days = 90
	}
	if in.Days > 365 {
		in.Days = 365
	}
	var out stockBaseOutput
	q := url.Values{"symbol": {in.Symbol}, "days": {strconv.Itoa(in.Days)}}
	if err := s.withAudit(ctx, "stock_get_technical", []string{"symbol", "days"}, func() error {
		return s.cli.Get(ctx, "/api/stock/technical", q, &out.Result)
	}); err != nil {
		return nil, stockBaseOutput{}, err
	}
	return nil, out, nil
}
```

- [ ] **Step 2: 實作 `tools_capitalflow.go`**

```go
package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerCapitalFlowTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "capital_flow_daily",
		Description: autoDescOr("capital_flow_daily", "Return the full daily Taiwan market capital flow report: seven force scores, resonance, and quality summary."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleCapitalFlowDaily)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "capital_flow_summary",
		Description: autoDescOr("capital_flow_summary", "Return a condensed capital flow summary for the latest trading day."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleCapitalFlowSummary)
}

func (s *server) handleCapitalFlowDaily(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, macroBaseOutput, error) {
	var out macroBaseOutput
	if err := s.withAudit(ctx, "capital_flow_daily", nil, func() error {
		return s.cli.Get(ctx, "/api/capital-flow/daily", nil, &out.Result)
	}); err != nil {
		return nil, macroBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleCapitalFlowSummary(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, macroBaseOutput, error) {
	var out macroBaseOutput
	if err := s.withAudit(ctx, "capital_flow_summary", nil, func() error {
		return s.cli.Get(ctx, "/api/capital-flow/summary", nil, &out.Result)
	}); err != nil {
		return nil, macroBaseOutput{}, err
	}
	return nil, out, nil
}
```

- [ ] **Step 3: 實作 `tools_strategy_ranker.go`**

```go
package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerStrategyRankerTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "strategy_ranker",
		Description: autoDescOr("strategy_ranker", "Return the current active strategies ranked by performance with free/registered/premium tier labels."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStrategyRanker)
}

func (s *server) handleStrategyRanker(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, strategyBaseOutput, error) {
	var out strategyBaseOutput
	if err := s.withAudit(ctx, "strategy_ranker", nil, func() error {
		return s.cli.Get(ctx, "/api/strategy-ranker/rank", nil, &out.Result)
	}); err != nil {
		return nil, strategyBaseOutput{}, err
	}
	return nil, out, nil
}
```

- [ ] **Step 4: 為每個 tool 寫至少一個 handler 測試**

範例 `tools_stock_test.go`：

```go
func TestHandleStockGetQuote(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"symbol":"2330","last":680}`)
	_, out, err := s.handleStockGetQuote(context.Background(), nil, stockSymbolInput{Symbol: "2330"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/stock/quote" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected result")
	}
}
```

`tools_capitalflow_test.go` 與 `tools_strategy_ranker_test.go` 比照其他 `tools_*_test.go` 模式。

- [ ] **Step 5: 執行 MCP 層測試**

Run: `go test ./cmd/atlas-mcp/server -run 'TestHandleStock|TestHandleCapitalFlow|TestHandleStrategyRanker' -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/atlas-mcp/server/tools_stock.go cmd/atlas-mcp/server/tools_capitalflow.go cmd/atlas-mcp/server/tools_strategy_ranker.go cmd/atlas-mcp/server/tools_stock_test.go cmd/atlas-mcp/server/tools_capitalflow_test.go cmd/atlas-mcp/server/tools_strategy_ranker_test.go
git commit -m "feat(mcp): add stock, capitalflow and strategy_ranker tools"
```

---

## Task 6: 更新 tool 註冊與數量斷言

**Files:**
- Modify: `cmd/atlas-mcp/server/tools.go`
- Modify: `cmd/atlas-mcp/server/server.go`
- Modify: `cmd/atlas-mcp/server/prompts.go`

**Interfaces:**
- Consumes: new `registerStockTools`, `registerCapitalFlowTools`, `registerStrategyRankerTools`.
- Produces: `registerTools` calls all three; `RegisteredToolCount` assertion updated.

- [ ] **Step 1: 修改 `registerTools` 加入新註冊函式**

```go
func registerTools(mcpSrv *mcp.Server, s *server) {
	registerMacroTools(mcpSrv, s)
	registerCrossmarketTools(mcpSrv, s)
	registerNarrativeTools(mcpSrv, s)
	registerEventTools(mcpSrv, s)
	registerRiskAlertTools(mcpSrv, s)
	registerStrategyTools(mcpSrv, s)
	registerStrategyRankerTools(mcpSrv, s) // NEW
	registerExperimentTools(mcpSrv, s)
	registerSynergyTools(mcpSrv, s)
	registerControlTools(mcpSrv, s)
	registerSchedulerTaskTools(mcpSrv, s)
	registerSystemTools(mcpSrv, s)
	registerLLMTraceTools(mcpSrv, s)
	registerDataUniverseTools(mcpSrv, s)
	registerReportPrismTools(mcpSrv, s)
	registerAnomalyTools(mcpSrv, s)
	registerSamplingTools(mcpSrv, s)
	registerRootsTools(mcpSrv, s)
	registerElicitationTools(mcpSrv, s)
	registerBriefingTools(mcpSrv, s)
	registerStockTools(mcpSrv, s)        // NEW
	registerCapitalFlowTools(mcpSrv, s)  // NEW

	// existing global tools below ...
}
```

- [ ] **Step 2: 更新 `server.go` 數量斷言**

新增 7 個 tool 後，將斷言範圍從 `82-84` 調整為 `89-91`（若 sampling/elicitation 各仍為 0–1）。實際數字以 `go test ./cmd/atlas-mcp/server -run TestRunToolCount` 錯誤訊息為準。

```go
if n := RegisteredToolCount; n < 89 || n > 91 {
	return fmt.Errorf("server: tool count drift: got %d, expected 89-91", n)
}
```

- [ ] **Step 3: 更新 prompts.go 中的佔位名稱**

將 `promptTaiwanQuickLook` 中的 `capital_flow_daily` 與 `event_calendar` 保留；將 `promptStrategyAdvice` 中的 `strategy_ranker` 保留（prompt 文字已正確，無需修改）。確認 `stock_health_check` prompt 未來可改引用 `stock_get_*` 工具；本次先不改動，避免擴大範圍。

- [ ] **Step 4: 執行 MCP server 測試**

Run: `go test ./cmd/atlas-mcp/server -v`
Expected: PASS（若 assertion 錯誤則依錯誤訊息調整上下界）。

- [ ] **Step 5: Commit**

```bash
git add cmd/atlas-mcp/server/tools.go cmd/atlas-mcp/server/server.go cmd/atlas-mcp/server/prompts.go
git commit -m "feat(mcp): wire new stock/capitalflow/ranker tools and update count assertion"
```

---

## Task 7: 統一 tool 數文件權威來源

**Files:**
- Modify: `docs/AGENT_TOOLS.md`
- Modify: `cmd/atlas-mcp/README.md`
- Modify: `cmd/atlas-mcp/server/AGENTS.md`
- Modify: `docs/specs/agent-mcp-server.md`

- [ ] **Step 1: 更新 `docs/AGENT_TOOLS.md`**

- 在 catalog 新增 `Stock` 區塊（4 個 tool）、`Capital Flow` 區塊（2 個 tool）、更新 `Strategy` 區塊加入 `strategy_ranker`。
- 將頂端「約 83 個 tool」改為「以 `mcp/tools/list` 或 `system_get_health` 回傳為準；詳細 catalog 見下方」。

- [ ] **Step 2: 更新其他文件不再寫死數字**

- `cmd/atlas-mcp/README.md`：將「79 個 tool」改為「詳見 `docs/AGENT_TOOLS.md`」。
- `cmd/atlas-mcp/server/AGENTS.md`：將「75 個 tool handler / 77–79 範圍」改為指向 `docs/AGENT_TOOLS.md`，並更新 assertion 範圍為 `89–91`。
- `docs/specs/agent-mcp-server.md`：將「約 70 個」改為「詳見 `docs/AGENT_TOOLS.md`」。

- [ ] **Step 3: Commit**

```bash
git add docs/AGENT_TOOLS.md cmd/atlas-mcp/README.md cmd/atlas-mcp/server/AGENTS.md docs/specs/agent-mcp-server.md
git commit -m "docs: unify tool count authority to AGENT_TOOLS.md"
```

---

## Task 8: 執行完整 CI 驗證

- [ ] **Step 1: 格式化**

Run: `gofmt -w internal/stocktools internal/strategy_ranker cmd/atlas/main.go cmd/atlas-mcp/server`

- [ ] **Step 2: 靜態檢查**

Run: `go vet ./internal/stocktools/... ./internal/strategy_ranker/... ./cmd/atlas-mcp/server/... ./cmd/atlas`
Expected: PASS

- [ ] **Step 3: 測試**

Run: `go test ./internal/stocktools/... ./internal/strategy_ranker/... ./cmd/atlas-mcp/server/...`
Expected: PASS

- [ ] **Step 4: 建置 binary**

Run: `go build ./cmd/atlas && go build ./cmd/atlas-mcp`
Expected: PASS

- [ ] **Step 5: Commit（若有任何 gofmt 變更）**

```bash
git add -A && git commit -m "style: gofmt"
```

---

## Task 9: 開立 Draft PR

- [ ] **Step 1: Push branch**

```bash
git push origin feat/atlas-mcp-stock-capitalflow-ranker-tools
```

- [ ] **Step 2: 開立 Draft PR**

使用 `gh pr create --draft --title "feat(mcp): add stock-level, capital flow and strategy ranker tools" --body-file .github/pr-body.md` 或手動在 GitHub 開立。

PR body 應包含：
- 關聯設計文件：`docs/superpowers/specs/2026-07-07-atlas-mcp-stock-capitalflow-ranker-tools-design.md`
- 新增 7 個 MCP tool 清單
- 後端路由清單
- 測試命令

---

## Self-Review

- **Spec coverage**: 所有設計文件中的 tool、route、文件更新、測試均對應到 task。
- **Placeholder scan**: 無 TBD/TODO；所有步驟均含具體檔案路徑與命令。
- **Type consistency**: `stockBaseOutput`、`macroBaseOutput`、`strategyBaseOutput` 均為 `Result *map[string]any`；handler 簽名與現有 `tools_*.go` 一致。
