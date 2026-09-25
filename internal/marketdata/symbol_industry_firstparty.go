package marketdata

// symbol_industry_firstparty.go — symbol_industry 的第一方來源：把兩份官方
// OpenAPI「上市公司基本資料」轉成一份 canonical 的每股產業快照。
//
// 為什麼需要它（issue #1943）：canonical 產業分類法已經統一（20 個 L1），
// 但 DB 沒有任何 per-stock 產業欄位，產業層級統計只能跑在
// industry.DefaultRepresentativeStocks() 那 ~27 檔上（約佔上市櫃 1988 檔的
// 3.2%）。要讓產業統計有意義，第一步是取得「每一檔股票屬於哪個產業」。
//
// 為什麼走第一方而不是 FinMind：這兩個欄位（上市「產業別」、上櫃
// SecuritiesIndustryCode）本來就是交易所自己發布的原始欄位 —— 交易所是
// source of truth，FinMind 只是鏡像。走第一方零 API key、零配額（2026-09-23
// FinMind 日配額 14400 被用光的教訓），且兩份 payload 只有 1.3MB / 1.0MB。
//
// 上游（2026-09-24 實測可達，皆為「JSON array of objects」）：
//   - 上市 TWSE：https://openapi.twse.com.tw/v1/opendata/t187ap03_L
//     1095 列；本檔只用 公司代號 / 公司名稱 / 產業別 三個欄位。
//   - 上櫃 TPEx：https://www.tpex.org.tw/openapi/v1/mopsfin_t187ap03_O
//     893 列；本檔只用 SecuritiesCompanyCode / CompanyName /
//     SecuritiesIndustryCode 三個欄位。
//
// 映射：每個 2 位數代號都走 #1958 落地的宣告表
//（sectormap.NamespaceTWSESIndustryCode，36 個已宣告代號：22 個有唯一對應、
// 14 個無對應但附理由）。本檔不新增任何分類法、不改 sectormap、不猜測未宣告
// 的代號 —— 猜測正是 #1943 要消滅的那類行為。
//
// 三種狀態的語意（下游必須分辨，不可合併成「有/沒有」）：
//   - mapped：宣告表有唯一 canonical L1。
//   - unmapped：代號已宣告但沒有站得住腳的單一 L1（殘差桶 19/20、已被細分
//     取代的聚合碼 13、通路／服務類 29/30 …）→ 仍寫進 entries，讓呼叫端
//     看到「這檔股票存在，但無法歸類」。
//   - unknown：代號根本沒宣告（上游 drift；例如 91 = TDR，2026-09-24 上市有
//     10 列）→ 回報，絕不自行對映。
//
// 空／失敗語意（#1953 DegradedOnEmpty 的教訓）：空 payload 絕不可當成功。
// 兩邊都成功但 0 列 → ErrNoData 且不寫檔；只有一邊失敗 → 保留另一邊、留
// warn、Sources 只列真正成功的來源；兩邊都失敗 → joined error 且不寫檔
// （舊檔保留，讓 channel 顯示的是「上一次成功的資料」而不是一份假的空檔）。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/sectormap"
)

const (
	// symbolIndustryChannelID 是通道 id。必須與 internal/symbolindustry
	// （ChannelName）以及 apigateway 的 channel contract 完全一致：state 檔
	// 的讀取端會驗這個欄位，不符就是「不是這個通道的檔」。
	symbolIndustryChannelID = "symbol_industry"

	// symbolIndustryStateFileName 是 state 檔名（<storageDir>/symbol_industry.json）。
	symbolIndustryStateFileName = "symbol_industry.json"

	// 兩個官方端點的路徑片段。base 可被 SetBaseURLs 覆寫（測試），路徑語意
	// 不變，所以獨立成常數。
	symbolIndustryTWSEReportPath = "/opendata/t187ap03_L"
	symbolIndustryTPExReportPath = "/mopsfin_t187ap03_O"

	// 預設 base（2026-09-24 實測可達）。
	symbolIndustryTWSEDefaultBaseURL = "https://openapi.twse.com.tw/v1"
	symbolIndustryTPExDefaultBaseURL = "https://www.tpex.org.tw/openapi/v1"

	// 來源標記，寫進 entries[].source 與 snapshot.sources。刻意含端點名稱：
	// 同一交易所的不同報表是不同的資料契約，只寫 "TWSE" 無法追來源。
	symbolIndustrySourceTWSE = "TWSE:t187ap03_L"
	symbolIndustrySourceTPEx = "TPEx:mopsfin_t187ap03_O"

	// 市場標記。
	symbolIndustryMarketTWSE = "TWSE"
	symbolIndustryMarketTPEx = "TPEx"

	// symbolIndustryMaxBodyBytes 上限：官方回應約 1.3MB（上市）／1.0MB
	// （上櫃），8MB 足以吸收欄位擴充，又能在上游異常時護住記憶體
	// （與 sblMaxBodyBytes 同值同理由）。
	symbolIndustryMaxBodyBytes = 8 << 20

	// MappingStatus 的三個值。刻意不 import internal/symbolindustry：它是
	// 本層的消費端（adapter 讀本檔產出的 state 檔），反過來 import 會造成
	// 循環依賴。字串由契約固定，兩邊的常數由 contract 測試守住。
	symbolIndustryStatusMapped   = "mapped"
	symbolIndustryStatusUnmapped = "unmapped"
	symbolIndustryStatusUnknown  = "unknown"

	// unknown 的兩種原因：上游整列沒有代號 vs 代號不在已宣告的 namespace。
	// 兩者處置不同（前者是上游資料品質，後者是 upstream drift），所以理由
	// 文字必須分辨。
	symbolIndustryReasonNoCode = "upstream row has no industry code"
	symbolIndustryReasonDrift  = "code not declared in namespace twse_industry_code (upstream drift; report, do not map)"
)

// SymbolIndustryEntry is one symbol's canonical industry row.
type SymbolIndustryEntry struct {
	Symbol         string `json:"symbol"`
	CompanyName    string `json:"company_name,omitempty"`
	Market         string `json:"market,omitempty"` // "TWSE" | "TPEx"
	IndustryCode   string `json:"industry_code"`
	IndustryNameZH string `json:"industry_name_zh,omitempty"`
	CanonicalL1    string `json:"canonical_l1,omitempty"` // "" when not mapped
	MappingStatus  string `json:"mapping_status"`         // "mapped" | "unmapped" | "unknown"
	MappingReason  string `json:"mapping_reason,omitempty"`
	Source         string `json:"source,omitempty"` // "TWSE:t187ap03_L" | "TPEx:mopsfin_t187ap03_O"
	AsOf           string `json:"as_of,omitempty"`  // "YYYY-MM-DD"
}

// SymbolIndustryCounts mirrors the per-status tallies. `canonical_l1` is the
// number of distinct canonical L1 sectors reached.
type SymbolIndustryCounts struct {
	Total       int `json:"total"`
	Mapped      int `json:"mapped"`
	Unmapped    int `json:"unmapped"`
	Unknown     int `json:"unknown"`
	CanonicalL1 int `json:"canonical_l1"`
}

// SymbolIndustryCodeDisposition explains one upstream code.
type SymbolIndustryCodeDisposition struct {
	Code   string `json:"code"`
	Name   string `json:"name,omitempty"`
	Count  int    `json:"count"`
	Reason string `json:"reason,omitempty"`
}

// SymbolIndustrySnapshot is the persisted state file.
type SymbolIndustrySnapshot struct {
	Channel       string                          `json:"channel"`    // always "symbol_industry"
	UpdatedAt     string                          `json:"updated_at"` // RFC3339 UTC
	Sources       []string                        `json:"sources"`
	Counts        SymbolIndustryCounts            `json:"counts"`
	L1Counts      map[string]int                  `json:"l1_counts"`
	UnmappedCodes []SymbolIndustryCodeDisposition `json:"unmapped_codes"`
	UnknownCodes  []SymbolIndustryCodeDisposition `json:"unknown_codes"`
	Entries       []SymbolIndustryEntry           `json:"entries"`
}

// SymbolIndustryProvider fetches the two official industry payloads and
// persists one canonical snapshot.
type SymbolIndustryProvider struct {
	// storageDir 為空代表停用持久化（StateFile() 回 ""）。生產是
	// <workDir>/data/state。
	storageDir string
	stateFile  string

	twseBase string
	tpexBase string

	client *http.Client
	// now 可在測試覆寫（見 SetClock），因此 UpdatedAt / AsOf 不會綁死
	// time.Now —— 否則「同一時鐘下兩次 FetchAll 必須逐位元相同」無法驗證。
	now func() time.Time
}

// NewSymbolIndustryProvider returns a provider persisting to
// <storageDir>/symbol_industry.json. An empty storageDir disables persistence.
func NewSymbolIndustryProvider(storageDir string) *SymbolIndustryProvider {
	p := &SymbolIndustryProvider{
		storageDir: storageDir,
		twseBase:   symbolIndustryTWSEDefaultBaseURL,
		tpexBase:   symbolIndustryTPExDefaultBaseURL,
	}
	if storageDir != "" {
		p.stateFile = filepath.Join(storageDir, symbolIndustryStateFileName)
	}
	// 與其他 TWSE 通道同一把尺：one attempt 的 timeout 由參數系統決定
	// （marketdata.twse_api_timeout_sec，預設 20s）。
	params := config.GetParametersConfig()
	p.client = httpclient.NewFactory().NewClient(
		time.Duration(params.Marketdata.TWSEAPITimeoutSec.Value) * time.Second)
	return p
}

// Name returns "symbol_industry".
func (p *SymbolIndustryProvider) Name() string { return symbolIndustryChannelID }

// StateFile returns the persisted snapshot path ("" when persistence is
// disabled).
func (p *SymbolIndustryProvider) StateFile() string { return p.stateFile }

// SetHTTPClient overrides the HTTP client (tests).
func (p *SymbolIndustryProvider) SetHTTPClient(c *http.Client) { p.client = c }

// SetBaseURLs overrides the two upstream bases (tests). Pass "" to keep default.
func (p *SymbolIndustryProvider) SetBaseURLs(twseBase, tpexBase string) {
	if strings.TrimSpace(twseBase) != "" {
		p.twseBase = strings.TrimRight(twseBase, "/")
	}
	if strings.TrimSpace(tpexBase) != "" {
		p.tpexBase = strings.TrimRight(tpexBase, "/")
	}
}

// SetClock overrides the time source used for UpdatedAt / AsOf (tests).
func (p *SymbolIndustryProvider) SetClock(now func() time.Time) { p.now = now }

// ─── 上游 payload 形狀 ─────────────────────────────────────────────────────
//
// 只宣告本檔真正用到的欄位：官方 payload 每列有 ~20-30 個欄位，全部宣告會把
// 檔案變成另一份上游 schema 的鏡像，且上游新增欄位時仍要維護。用不到就不宣告
// （json 解碼會忽略未知欄位）。

type twseSymbolIndustryRow struct {
	Symbol string `json:"公司代號"`
	Name   string `json:"公司名稱"`
	Code   string `json:"產業別"`
}

type tpexSymbolIndustryRow struct {
	Symbol string `json:"SecuritiesCompanyCode"`
	Name   string `json:"CompanyName"`
	Code   string `json:"SecuritiesIndustryCode"`
}

// symbolIndustryRawRow 是兩家交易所正規化後的共同形狀：欄位名不同、語意相同。
type symbolIndustryRawRow struct {
	Symbol string
	Name   string
	Code   string
}

// symbolIndustryRawRowsFromTWSE 把 TWSE 的欄位名轉成本檔的欄位名。
func symbolIndustryRawRowsFromTWSE(rows []twseSymbolIndustryRow) []symbolIndustryRawRow {
	out := make([]symbolIndustryRawRow, len(rows))
	for i, r := range rows {
		// Field names/types/order are identical, only the JSON tags differ, so
		// the conversion is the whole adapter (staticcheck S1016).
		out[i] = symbolIndustryRawRow(r)
	}
	return out
}

// symbolIndustryRawRowsFromTPEx 同上，TPEx 版（欄位名完全不同）。
func symbolIndustryRawRowsFromTPEx(rows []tpexSymbolIndustryRow) []symbolIndustryRawRow {
	out := make([]symbolIndustryRawRow, len(rows))
	for i, r := range rows {
		out[i] = symbolIndustryRawRow(r) // see symbolIndustryRawRowsFromTWSE
	}
	return out
}

// ─── FetchAll ─────────────────────────────────────────────────────────────

// FetchAll fetches both markets, maps every code through
// sectormap.NamespaceTWSESIndustryCode, and persists the snapshot atomically.
func (p *SymbolIndustryProvider) FetchAll(ctx context.Context) (*SymbolIndustrySnapshot, error) {
	twseEntries, twseErr := p.fetchTWSESymbolIndustry(ctx)
	tpexEntries, tpexErr := p.fetchTPExSymbolIndustry(ctx)

	if twseErr != nil && tpexErr != nil {
		// 兩邊都掛 = 真正的上游故障：joined error，且不寫檔。寫一份空檔會讓
		// 下游看到「剛更新過的空資料」，比保留舊檔更糟（#1953）。
		return nil, errors.Join(twseErr, tpexErr)
	}
	if twseErr != nil || tpexErr != nil {
		// 單邊失敗：覆蓋不完整，但仍勝過整份缺資料。留 warn 讓運維看見，且
		// Sources 只列成功來源 —— 讀者不必比對檔名就知道缺哪一半。
		failedSource, failedErr := symbolIndustrySourceTPEx, tpexErr
		if twseErr != nil {
			failedSource, failedErr = symbolIndustrySourceTWSE, twseErr
		}
		logging.Warn(symbolIndustryChannelID, "first_party_partial",
			logging.FStr("failed_source", failedSource),
			logging.Err(failedErr))
	}

	// TWSE 先加入，因此同名代號保留上市列（見 mergeSymbolIndustryEntries）。
	entries := mergeSymbolIndustryEntries(twseEntries, tpexEntries)
	if len(entries) == 0 {
		// 空 payload 絕不可當成功（#1953 DegradedOnEmpty）。若同時有單邊失敗，
		// 把那個失敗也 join 進來，否則診斷資訊會消失。
		noData := fmt.Errorf("symbol_industry: %w", ErrNoData)
		switch {
		case twseErr != nil:
			return nil, errors.Join(twseErr, noData)
		case tpexErr != nil:
			return nil, errors.Join(tpexErr, noData)
		default:
			return nil, noData
		}
	}

	snap := p.buildSnapshot(entries, symbolIndustrySources(twseErr, tpexErr))
	if err := p.persistSnapshot(snap); err != nil {
		return nil, err
	}
	return snap, nil
}

// symbolIndustrySources 回傳本次真正成功的來源（順序固定 TWSE → TPEx，讓
// 檔案內容在相同條件下逐位元穩定）。
func symbolIndustrySources(twseErr, tpexErr error) []string {
	out := make([]string, 0, 2)
	if twseErr == nil {
		out = append(out, symbolIndustrySourceTWSE)
	}
	if tpexErr == nil {
		out = append(out, symbolIndustrySourceTPEx)
	}
	return out
}

// fetchTWSESymbolIndustry 取上市（TWSE t187ap03_L）。
func (p *SymbolIndustryProvider) fetchTWSESymbolIndustry(ctx context.Context) ([]SymbolIndustryEntry, error) {
	rawURL := p.twseBase + symbolIndustryTWSEReportPath
	body, contentType, err := symbolIndustryHTTPGet(ctx, p.client, getTWSESharedLimiter(), rawURL)
	if err != nil {
		return nil, fmt.Errorf("symbol_industry: TWSE %s: %w: %w",
			symbolIndustryTWSEReportPath, ErrUpstream, err)
	}
	var rows []twseSymbolIndustryRow
	if err := DecodeJSON(bytes.NewReader(body), contentType, &rows); err != nil {
		return nil, fmt.Errorf("symbol_industry: TWSE %s decode: %w: %w",
			symbolIndustryTWSEReportPath, ErrSchema, err)
	}
	entries, dropped := symbolIndustryEntriesFromRawRows(
		symbolIndustryRawRowsFromTWSE(rows), symbolIndustryMarketTWSE, symbolIndustrySourceTWSE)
	symbolIndustryLogDroppedRows(symbolIndustrySourceTWSE, dropped)
	return entries, nil
}

// fetchTPExSymbolIndustry 取上櫃（TPEx mopsfin_t187ap03_O）。
func (p *SymbolIndustryProvider) fetchTPExSymbolIndustry(ctx context.Context) ([]SymbolIndustryEntry, error) {
	rawURL := p.tpexBase + symbolIndustryTPExReportPath
	body, contentType, err := symbolIndustryHTTPGet(ctx, p.client, getTPExSharedLimiter(), rawURL)
	if err != nil {
		return nil, fmt.Errorf("symbol_industry: TPEx %s: %w: %w",
			symbolIndustryTPExReportPath, ErrUpstream, err)
	}
	var rows []tpexSymbolIndustryRow
	if err := DecodeJSON(bytes.NewReader(body), contentType, &rows); err != nil {
		return nil, fmt.Errorf("symbol_industry: TPEx %s decode: %w: %w",
			symbolIndustryTPExReportPath, ErrSchema, err)
	}
	entries, dropped := symbolIndustryEntriesFromRawRows(
		symbolIndustryRawRowsFromTPEx(rows), symbolIndustryMarketTPEx, symbolIndustrySourceTPEx)
	symbolIndustryLogDroppedRows(symbolIndustrySourceTPEx, dropped)
	return entries, nil
}

// symbolIndustryLogDroppedRows 只在真的丟掉列時出聲。官方 payload 目前沒有
// 彙總列（2026-09-24 實測），所以這個 warn 一旦出現就是上游改了形狀，不該被
// 靜默吃掉（但也沒嚴重到要讓整次 fetch 失敗：能救的列還是要救）。
func symbolIndustryLogDroppedRows(source string, dropped int) {
	if dropped == 0 {
		return
	}
	logging.Warn(symbolIndustryChannelID, "first_party_row_dropped",
		logging.FStr("source", source),
		logging.FInt("rows", dropped),
		logging.FStr("reason", "symbol is empty or not ASCII alnum"))
}

// ─── 映射（upstream code → canonical L1）─────────────────────────────────

// symbolIndustryEntriesFromRawRows 逐列轉成 entry，回傳 (entries, 丟棄列數)。
// 丟棄只發生在「代號不是英數字」時（防禦上游的中文彙總列）；代號合法但產業別
// 空白／未宣告的列一律保留，改成 status=unknown —— 靜默丟掉等於憑空縮小母體。
func symbolIndustryEntriesFromRawRows(rows []symbolIndustryRawRow, market, source string) ([]SymbolIndustryEntry, int) {
	out := make([]SymbolIndustryEntry, 0, len(rows))
	dropped := 0
	for _, r := range rows {
		e, ok := buildSymbolIndustryEntry(r.Symbol, r.Name, r.Code, market, source)
		if !ok {
			dropped++
			continue
		}
		out = append(out, e)
	}
	return out, dropped
}

// buildSymbolIndustryEntry 建一列。ok=false 代表代號不是可用的證券代號。
func buildSymbolIndustryEntry(symbol, name, code, market, source string) (SymbolIndustryEntry, bool) {
	sym := normalizeSymbolIndustrySymbol(symbol)
	if sym == "" {
		return SymbolIndustryEntry{}, false
	}
	entry := SymbolIndustryEntry{
		Symbol:       sym,
		CompanyName:  strings.TrimSpace(name),
		Market:       market,
		IndustryCode: strings.TrimSpace(code),
		Source:       source,
	}

	if entry.IndustryCode == "" {
		// 有股票、沒有產業別：不可丟掉（母體要誠實），也不可猜。
		entry.MappingStatus = symbolIndustryStatusUnknown
		entry.MappingReason = symbolIndustryReasonNoCode
		return entry, true
	}

	// 名稱只取自宣告表；查不到就留空，絕不用其他來源拼一個名字出來。
	if zhName, ok := sectormap.TWSESIndustryCodeName(entry.IndustryCode); ok {
		entry.IndustryNameZH = zhName
	}

	switch m := sectormap.Resolve(sectormap.NamespaceTWSESIndustryCode, entry.IndustryCode); m.Status {
	case sectormap.StatusMapped, sectormap.StatusCanonical:
		// StatusCanonical 目前不會出現（本 namespace 沒有 identity key），但
		// 語意上它與 mapped 同義（有唯一目標），所以一起處理而不是掉到
		// default 被誤標成 unknown。
		entry.MappingStatus = symbolIndustryStatusMapped
		entry.MappingReason = m.Reason
		if l1, ok := m.Primary(); ok {
			entry.CanonicalL1 = l1
		}
	case sectormap.StatusUnmapped:
		entry.MappingStatus = symbolIndustryStatusUnmapped
		entry.MappingReason = m.Reason
	default:
		entry.MappingStatus = symbolIndustryStatusUnknown
		entry.MappingReason = symbolIndustryReasonDrift
	}
	return entry, true
}

// normalizeSymbolIndustrySymbol 正規化代號：去空白，且只接受 ASCII 英數字。
// 後者是防禦性檢查 —— 交易所報表偶有「合計」這類中文彙總列，那種列不是股票。
func normalizeSymbolIndustrySymbol(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
		default:
			return ""
		}
	}
	return s
}

// mergeSymbolIndustryEntries 合併兩個市場並去重（上市優先），最後依代號排序。
//
// 2026-09-24 實測：上市 1095 檔與上櫃 893 檔代號集合交集為 0，但契約仍明寫
// 上市優先 —— 上游哪天真的重疊時，這行決定的是「哪一列活下來」，不能靠
// payload 順序碰運氣。
func mergeSymbolIndustryEntries(twseEntries, tpexEntries []SymbolIndustryEntry) []SymbolIndustryEntry {
	merged := make([]SymbolIndustryEntry, 0, len(twseEntries)+len(tpexEntries))
	seen := make(map[string]struct{}, len(twseEntries)+len(tpexEntries))
	for _, group := range [][]SymbolIndustryEntry{twseEntries, tpexEntries} {
		for _, e := range group {
			if _, dup := seen[e.Symbol]; dup {
				continue
			}
			seen[e.Symbol] = struct{}{}
			merged = append(merged, e)
		}
	}
	// 排序讓檔案內容與上游列序無關：上游改排序不會造成無意義的 diff，且
	// 「同一時鐘下兩次 FetchAll 逐位元相同」才成立。
	sort.Slice(merged, func(i, j int) bool { return merged[i].Symbol < merged[j].Symbol })
	return merged
}

// ─── Snapshot 組裝 ────────────────────────────────────────────────────────

// symbolIndustryCodeAgg 是「同一個代號」的累計（數量 + 名稱 + 理由）。
type symbolIndustryCodeAgg struct {
	name   string
	count  int
	reason string
}

func (p *SymbolIndustryProvider) buildSnapshot(entries []SymbolIndustryEntry, sources []string) *SymbolIndustrySnapshot {
	now := p.nowUTC()
	snap := &SymbolIndustrySnapshot{
		Channel:       symbolIndustryChannelID,
		UpdatedAt:     now.Format(time.RFC3339),
		Sources:       sources,
		L1Counts:      make(map[string]int),
		UnmappedCodes: make([]SymbolIndustryCodeDisposition, 0),
		UnknownCodes:  make([]SymbolIndustryCodeDisposition, 0),
		Entries:       entries,
	}

	asOf := now.Format("2006-01-02")
	unmapped := make(map[string]*symbolIndustryCodeAgg)
	unknown := make(map[string]*symbolIndustryCodeAgg)

	for i := range snap.Entries {
		e := &snap.Entries[i]
		e.AsOf = asOf
		snap.Counts.Total++
		switch e.MappingStatus {
		case symbolIndustryStatusMapped:
			snap.Counts.Mapped++
			// L1Counts 只算「真的歸到某個 L1」的股票：mapped 但 CanonicalL1
			// 為空（理論上不該發生）不可計入，否則 L1 覆蓋數會虛胖。
			if e.CanonicalL1 != "" {
				snap.L1Counts[e.CanonicalL1]++
			}
		case symbolIndustryStatusUnmapped:
			snap.Counts.Unmapped++
			accumulateSymbolIndustryCode(unmapped, e)
		default:
			// 未知狀態一律算 unknown（不放過任何一列），理由已在 entry 上。
			snap.Counts.Unknown++
			accumulateSymbolIndustryCode(unknown, e)
		}
	}

	snap.Counts.CanonicalL1 = len(snap.L1Counts)
	snap.UnmappedCodes = symbolIndustryCodeDispositions(unmapped)
	snap.UnknownCodes = symbolIndustryCodeDispositions(unknown)
	return snap
}

func accumulateSymbolIndustryCode(agg map[string]*symbolIndustryCodeAgg, e *SymbolIndustryEntry) {
	a, ok := agg[e.IndustryCode]
	if !ok {
		a = &symbolIndustryCodeAgg{name: e.IndustryNameZH, reason: e.MappingReason}
		agg[e.IndustryCode] = a
	}
	a.count++
}

// symbolIndustryCodeDispositions 把累計 map 轉成排序後（依 code 遞增）的清單。
// map 迭代順序隨機，不排序就不能逐位元比對。空代號列也會出現在清單裡
// （Code=""），理由欄分辨它是「上游沒給代號」。
func symbolIndustryCodeDispositions(agg map[string]*symbolIndustryCodeAgg) []SymbolIndustryCodeDisposition {
	out := make([]SymbolIndustryCodeDisposition, 0, len(agg))
	for code, a := range agg {
		out = append(out, SymbolIndustryCodeDisposition{
			Code:   code,
			Name:   a.name,
			Count:  a.count,
			Reason: a.reason,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

func (p *SymbolIndustryProvider) nowUTC() time.Time {
	clock := p.now
	if clock == nil {
		clock = time.Now
	}
	return clock().UTC()
}

// ─── HTTP ────────────────────────────────────────────────────────────────

// symbolIndustryHTTPGet 取回官方 JSON body（含共用 token bucket 與大小上限）。
// limiter 由呼叫端注入：上市／上櫃各自沿用既有的共用桶
// （getTWSESharedLimiter / getTPExSharedLimiter），本檔不新建 limiter ——
// 每個 provider 各建一個桶會集體超過官方政策（twse_openapi.go P1-13）。
func symbolIndustryHTTPGet(ctx context.Context, client *http.Client, limiter *rate.Limiter, rawURL string) ([]byte, string, error) {
	if limiter != nil {
		if err := limiter.Wait(ctx); err != nil {
			return nil, "", fmt.Errorf("rate limit wait: %w", err)
		}
	}
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, symbolIndustryMaxBodyBytes))
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("http status %d", resp.StatusCode)
	}
	return body, resp.Header.Get("Content-Type"), nil
}

// ─── 持久化 ──────────────────────────────────────────────────────────────

// persistSnapshot 原子寫入（tmp + rename）。
//
// 為什麼一定原子：這個檔會被 gateway（DataState）與排程任務同時讀。直接
// WriteFile 若在寫到一半被 kill，留下的是半個 JSON —— 讀取端會把「解析失敗」
// 當成 warn，但更糟的情況是它剛好是合法 JSON 卻砍在半路（截斷的陣列）。tmp +
// rename 讓讀者只會看到舊的完整檔或新的完整檔。
func (p *SymbolIndustryProvider) persistSnapshot(snap *SymbolIndustrySnapshot) error {
	if p.stateFile == "" {
		return nil
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("symbol_industry: marshal snapshot: %w", err)
	}
	dir := filepath.Dir(p.stateFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("symbol_industry: mkdir %s: %w", dir, err)
	}
	tmpPath := p.stateFile + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("symbol_industry: write %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, p.stateFile); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("symbol_industry: rename %s: %w", tmpPath, err)
	}
	logging.Info(symbolIndustryChannelID, "state_saved",
		logging.FInt("total", snap.Counts.Total),
		logging.FInt("mapped", snap.Counts.Mapped),
		logging.FInt("unmapped", snap.Counts.Unmapped),
		logging.FInt("unknown", snap.Counts.Unknown),
		logging.FInt("canonical_l1", snap.Counts.CanonicalL1),
		logging.FStr("path", p.stateFile))
	return nil
}

// LoadSnapshot reads the last persisted snapshot (no network).
func (p *SymbolIndustryProvider) LoadSnapshot() (*SymbolIndustrySnapshot, error) {
	if p.stateFile == "" {
		return nil, errors.New("symbol_industry: persistence disabled (empty storage dir)")
	}
	raw, err := os.ReadFile(p.stateFile)
	if err != nil {
		return nil, fmt.Errorf("symbol_industry: read state file %s: %w", p.stateFile, err)
	}
	var snap SymbolIndustrySnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, fmt.Errorf("symbol_industry: parse state file %s: %w", p.stateFile, err)
	}
	// 通道名不符 = 這不是本通道的檔（例如 storageDir 指到別的目錄）。與
	// internal/symbolindustry.ReadSnapshot 同一條規則，避免同一個檔在兩層
	// 有兩種判準。
	if snap.Channel != symbolIndustryChannelID {
		return nil, fmt.Errorf("symbol_industry: state file %s has channel %q, want %q",
			p.stateFile, snap.Channel, symbolIndustryChannelID)
	}
	return &snap, nil
}
