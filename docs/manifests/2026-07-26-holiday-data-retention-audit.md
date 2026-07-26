# Channel Holiday / Off-Hours Data Retention Audit

> **Audit source**: 使用者 (2026-07-26) — DXY 資料在休市時顯示「資料獲取失敗」
> **Scope**: 全部 38 個 Gateway 資訊通道的假日/休市資料保留機制
> **Created**: 2026-07-26
> **Status**: audit-complete

---

## 審計方法

每個通道檢查:
1. Provider 層: 休市時回傳什麼(error / 零值 / 空值 / 最後交易日資料)?
2. Adapter 層: 有無 persist / "retain last good value" 機制?
3. Cache 層: 有無 TTL / 磁碟 persist?
4. 零值防護: 有無 NaN/Inf/零值 guards?

共享機制:
- Gateway `CacheLayer`(TTL=5min,in-memory) — 所有 channel 通用
- Gateway Circuit Breaker: open 時若 cache 仍存在,回 stale/fallback[gateway.go:96-107]
- usCache: 60s in-memory[us_market_cache.go:42-48]

---

## 通道審計表

| Channel | Source | 休市資料遺失風險 | Retain Last 機制 | 現況 |
|---------|--------|:---:|---|---|
| **us_yahoo** | Yahoo 9 macro | ⚠️ HIGH | 僅 60s/5m cache;CB reset 後即清零 | 零值防禦 reject → Symbol='',Value=0;adapter 全失敗仍回傳成功 overwrite cache |
| us_spx | Yahoo ^GSPC | ✅ Safe | Yahoo 1d chart 回最後收盤 | 5d range 確保有歷史 close |
| us_ndx | Yahoo ^IXIC | ✅ Safe | 同上 | 同上 |
| us_dji | Yahoo ^DJI | ✅ Safe | 同上 | 同上 |
| us_nvda | Yahoo NVDA | ✅ Safe | 同上 | 同上 |
| us_aapl | Yahoo AAPL | ✅ Safe | 同上 | 同上 |
| us_msft | Yahoo MSFT | ✅ Safe | 同上 | 同上 |
| tsm_adr | Yahoo TSM | ✅ Safe | 同上 | 同上 |
| sox_index | Yahoo ^SOX | ✅ Safe | 同上 | 同上 |
| **dram_spot_price** | Yahoo DRAM | ⚠️ HIGH | 僅 60s cache | 零值防禦 reject;無磁碟 fallback |
| twse_replay | TWSE | ✅ Safe | TWSE ReturnType=1 回歷史 | 檔案驅動,有 replay fallback |
| twse_capital_flow | TWSE | ✅ Safe | TWSECapitalFlowProvider 有 history | 檔案驅動,有資料持久 |
| twse_margin | TWSE | ✅ Safe | TWSEMarginBalanceProvider 有 storage | 同上 |
| taiex_index | Yahoo ^TWII | ✅ Safe | Yahoo 1d/3mo chart 回最後 | 休市回最近交易日收盤 |
| tw_vol | Yahoo ^TWIIV | ✅ Safe | 同上 | 同上 |
| bdi | Yahoo ^BDIY? | ⚠️ Minor | 模擬 provider | 需確認來源 |
| twse_sector_index | TWSE | ✅ Safe | 檔案驅動 | 有歷史資料 |
| frankfurter_fx | Frankfurter API | ✅ Safe | API /latest 回上交易日 | Forex 24/5 極少休市 |
| exchange_rate | TWSE/forex | ✅ Safe | 檔案 cross-day cache | 有磁碟持久 |
| geopolitical | GDELT/RSS | ✅ Safe | 檔案 persist | 每次 fetch 寫 latest.json |
| geopolitical_taiwan | 同上 | ✅ Safe | 同上 | 同上 |
| tsmc_revenue | FinMind | ✅ Safe | fetch fail → loadLatestSnapshot | 磁碟 cache fallback |
| export_statistics | TWSE CSV | ⚠️ MED | HTTP CSV;records<2→error | 無 local cache |
| government_flow | file-based | ✅ Safe | Stale=true 而非 error | 檔案存在才讀 |
| government_broker | file-based | ✅ Safe | previousTradingDay() | weekend-aware |
| sector_data | file-based | ✅ Safe | graceful degradation | Symbol=='' 下游 ignore |
| janus_regime | computed | ✅ Safe | 僅計算;7-day staleness warn | 無資料源問題 |
| day_trading | TWSE | ✅ Safe | 7-day 迴圈找有效日期 | built-in retry |
| taifex_daily | TAIFEX | ⚠️ MED | 無 cache / 無 persist | 假日直接 error |
| taifex_institutional | TAIFEX | ⚠️ MED | 無 cache / 無 persist | 同上 |
| tej | TEJ API | ❌ depends | 無本地 persist | API key 7 天過期 |
| finmind | FinMind API | ✅ Safe | saveSnapshot + weekend-aware | yesterday() |
| fugle | Fugle API | ✅ Safe | saveSnapshot | 即時 quote |
| fubon | Fubon API | ✅ Safe | saveSnapshot | 即時 quote |
| twse_oddlot | TWSE | ✅ Safe | 7-day 迴圈找有效日期 | built-in retry |
| twse_etf | TWSE | ✅ Safe | 同上 | 同上 |
| twse_sbl | TWSE | ⊘ STUB | always error | G02 stub |
| tdcc_equity_dispersion | TDCC | ⊘ STUB | always inactive | G01 stub |

## 高風險通道(HIGH)

### 1. us_yahoo (9 個 macro 指標)
- **Provider 問題**: `FetchSnapshot` 在 fetch 前就設 `RecordedAt=now`[yahoo_macro_provider.go:64]
- **Adapter 問題**: `Fetch()` 只用 `RecordedAt>0` 判斷 partial success[adapter_yahoo_macro.go:30-41]
- **觸發情境**: 全部 9 個指標失敗(零值防禦 reject) → RecordedAt 仍然 > 0 → adapter 當作成功 → marshal 寫入 Gateway cache → **覆寫 last-good data**
- **影響**: DXY/l10Y/VIX/Oil/Gold/Silver/Copper 等全部自動歸零→ 前端顯示「資料獲取失敗」
- **Gateway 層**: CB open 後若 cache 還在 → 回 stale/fallback 可以撐 5 分鐘,但 CB reset 或 cache 到期後即不可用

### 2. dram_spot_price
- 僅 60s usCache,無磁碟
- 休市時零值防禦 reject → Symbol=''
- 與 us_yahoo 相同問題模式

---

## 解決方案方向(待討論)

### Option A: Provider 層 — 最後交易日語意(像 Yahoo 衍生)
- Yahoo 1d chart 本身就回最近收盤(包括休日)
- us_yahoo macro 用 `range=1mo` 同樣會回歷史 close
- **現有零值防禦過度嚴格**: 零值防禦(`fetchIndicator` line 176)直接 reject 零值
- **改法**: 改為回最後一個有效的非零值(從 closes 陣列倒著找),而非 reject

### Option B: Adapter 層 — Snapshot persist + stale fallback
- 每個 adapter 保存上一個成功的 snapshot 到磁碟
- Fetch 失敗且 snapshot 有效時,改用磁碟 snapshot
- us_yahoo 需要修正: 不要以 `RecordedAt>0` 判 partial success,改為「至少一個 field 有 Symbol!=''」

### Option C: Adapter 層 — 修正 RecordedAt 判斷
- 最輕量的修正: us_yahoo adapter 不再只需 RecordedAt>0,要檢查是否有任何一個 indicator 有 Symbol
- 這樣全失敗時 adapter 回 error → Gateway CB 計入 failure → 若 CB open → 用 stale cache
- 但 cache 只有 5 分鐘

### Option D: 參考 TaiEX/SPX 模式: 擴大 Yahoo chart range
- 在 FetchSnapshot 遇到零值時,改用 `range=5d` 甚至 `range=1mo` 重試
- 這樣即使今天零值,也能從歷史取得上一個交易日
