# 七維錢潮分層模型規格（Seven-Dimension Capital-Flow Model）

> **文件角色**：atlas 資金流分類學、資料語意、計算邊界與研究衝突處理的唯一權威規格。
> **適用範圍**：`internal/capitalflow`、`internal/forecast`、`internal/eventdriven`、`internal/recommender`、`internal/portfolio`、網頁錢潮介面與 atlas-mcp 資金流工具。
> **決策狀態**：設計已由業主接受；E06/F05/F06 實作尚未完成。
> **版本**：1.0
> **日期**：2026-07-17
> **取代語意**：取代「七大勢力皆為同級資金主體」及「五大主體、另兩項僅為待刪除資料」兩種過度簡化說法。

---

## 1. 為什麼需要這份唯一正本

atlas 過去同時存在三套互相衝突的說法：

1. UI、MCP 與部分文件稱為「七大資金勢力」。
2. E05 程式將其改為 `5 subject + 1 leading_indicator + 1 sentiment`，並把後兩項標為 `deprecated`。
3. 實際資料來源只有三項是交易所正式法人分類；官股與散戶是代理，期貨 OI 與 TSM ADR 是訊號而非資金主體。

這種衝突會造成未來 AI Agent：

- 把同一批外資的現貨與期貨部位當成兩個獨立法人；
- 把股數、期貨口數與百分比放在同一分母計算權重；
- 把「API 回傳七筆」誤解成「模型必須有七個同級主體」；
- 只根據當下程式碼重新發明分類，推翻已完成的研究；
- 把歷史 audit／archive 文件當成現行規格；
- 反覆研究同一問題，卻因來源層級不同得到互相衝突的答案。

本文件固定來源優先級、事實與假設邊界、七維資料字典、計算不變式及變更程序。

---

## 2. 權威層級與衝突處理

### 2.1 本議題的來源優先級

同一敘述出現衝突時，依序採用：

1. **交易所／監管機構第一方定義**：TWSE、TAIFEX、中央銀行、金管會、集保結算所等。
2. **本文件的語意與計算契約**：定義 atlas 如何使用第一方資料與代理資料。
3. **專屬資料方法論 spec**：例如 `government-force-proxy.md`、`foreign-flow-forecast.md`；不得與本文件衝突。
4. **程式與 API 實作**：用來判斷目前是否符合規格，不能反過來僅因舊程式存在就改寫規格。
5. **活體輸出**：用於偵測部署漂移或 runtime bug，不是分類學來源。
6. **審計、manifest、CHANGELOG、archive**：是歷史證據，不是現行語意的最高仲裁。
7. **媒體、券商文章、AI 回答**：只能產生待驗證假設，不得直接成為模型規則。

### 2.2 衝突處理規則

- 第一方來源與程式不同：建立缺口 ID 修程式，不得把程式現況寫成金融事實。
- 本文件與專屬 spec 不同：本文件先維持權威；提出包含新證據、反例、影響範圍與驗證方法的變更案，核准後同一變更同步更新本文件與專屬 spec，不得先讓專屬 spec 成為第二份真相。
- 活體 API 與 main branch 不同：視為 build／部署／序列化漂移，必須驗證 binary commit。
- 歷史 audit 與本文件不同：保留 audit 原文，新增「已由何決策取代」連結，不回寫歷史。
- 來源不足：標為「待驗證假設」，不得填入 production 固定門檻或投資承諾。

---

## 3. 研究聲明類型

所有後續文件、程式註解、MCP 描述與 AI 回答必須區分：

| 類型 | 定義 | 可以驅動 production 決策？ |
|------|------|---------------------------|
| **查證事實** | 可由第一方來源、固定 schema 或可重現程式輸出直接核對 | 可以，但仍需資料品質 gate |
| **工程推論** | 從查證事實推導的架構或統計判斷 | 需 invariant／測試支持 |
| **待驗證假設** | 方向合理但尚無 atlas 歷史樣本驗證 | 不可以；只能顯示「校準中」 |
| **歷史觀測** | 特定日期的活體或回測結果 | 只能描述該時間點，不得外推為永恆規則 |

禁止使用「市場常識」「一般都知道」「機構通常會」作為未附來源的事實標記。

---

## 4. 核心決策

### D-CF-01：保留七個觀測維度

atlas 保留：外資、投信、自營商、官股行庫、散戶、外資期貨 OI、TSM ADR。

保留原因是七項提供不同觀測角度；保留不代表七項都是獨立法人，也不代表七項可使用相同權重公式。

### D-CF-02：拒絕七個同級資金主體

外資期貨 OI 是外資在衍生性商品的部位維度；TSM ADR 是跨市場價格／情緒訊號。兩者不是新的台股資金參與者。

### D-CF-03：拒絕簡化成五個同質主體

官股行庫與散戶雖代表具有實務意義的行為群體，但目前沒有與三大法人同品質的官方每日彙總資料：

- 官股使用代理方法；
- 散戶目前使用融資融券變化代理，並非完整自然人交易流。

因此五項也不能假裝是證據品質完全相同的主體。

### D-CF-04：採 3 + 2 + 2 七維分層模型

```text
七維錢潮雷達
├─ 官方法人資金流（3）
│  ├─ 外資
│  ├─ 投信
│  └─ 自營商
├─ 行為代理資金流（2）
│  ├─ 官股行庫代理
│  └─ 散戶代理
└─ 領先／跨市場訊號（2）
   ├─ 外資期貨未平倉
   └─ TSM ADR
```

### D-CF-05：產品名稱與模型名稱分離

- **模型正式名稱**：七維錢潮分層模型。
- **散戶介面建議名稱**：七維錢潮雷達。
- `7-Force`／「七大資金勢力」可作向後相容別名，但 UI 必須同時顯示三層角色，不得讓使用者誤認七個同級法人。

---

## 5. 第一方來源登錄表

| Source ID | 第一方來源 | 可證明內容 | 不能證明內容 | URL |
|-----------|------------|------------|--------------|-----|
| `SRC-TWSE-T86` | 臺灣證券交易所「三大法人買賣超日報」 | 現貨外陸資、投信、自營商的逐證券買賣股數分類 | 官股整體流量、完整散戶流量、外資期貨部位 | <https://www.twse.com.tw/zh/page/trading/fund/T86.html> |
| `SRC-TAIFEX-INST` | 臺灣期貨交易所「三大法人－區分各期貨契約」 | 外資、投信、自營商在期貨契約的交易量與未平倉部位 | 新增一種獨立法人；期貨必然領先現貨幾日 | <https://www.taifex.com.tw/cht/3/futContractsDate> |
| `SRC-CBC-FIVE-BANK` | 中央銀行「五大銀行新承做放款」 | 央行該統計使用臺銀、合庫銀、土銀、華銀、一銀口徑 | 股票市場「八大行庫」買賣超口徑 | <https://www.cbc.gov.tw/tw/lp-528-1.html> |
| `SRC-SEC-TSMC` | SEC EDGAR TSMC entity filings | TSMC 在美國市場的申報主體與證券資訊 | TSM ADR 是台股資金主體；ADR 對台股固定命中率 | <https://www.sec.gov/cgi-bin/browse-edgar?action=getcompany&CIK=0001046179> |

### 5.1 第一方來源的使用限制

- T86 欄位是**買賣股數**。目前 `TWSECapitalFlowProvider` 將各證券淨股數相加後除以 `1e8`，得到「億股」近似，不是新台幣億元。
- 不同股票每股價格不同，直接加總股數不能代表經濟金額；若要比較資金規模，需逐證券以價格轉成金額或改用具金額語意的資料源。
- TAIFEX 未平倉量單位是契約口數，不能與 T86 億股直接相加。
- 中央銀行五大銀行放款統計不能直接推導「八大行庫進出台股」。
- SEC／ADR 價格資料能證明跨市場價格訊號，不能證明誰在台股買賣。

### 5.2 Repository 證據錨點

下表記錄本次決策所依賴的實作證據。行號可能隨後續修改位移，符號／章節名稱才是長期定位鍵。

| Evidence ID | 檔案與目前行號 | 符號／章節 | 證明內容 |
|-------------|----------------|------------|----------|
| `EVD-CF-TYPES` | `internal/capitalflow/types.go:15-56` | `ForceName`、`ForceScore`、Force roles | 現行七個 key 與 E05 的 role/deprecated 契約 |
| `EVD-CF-EXTRACT` | `internal/capitalflow/forces.go:9-59` | `ForceExtractor.Extract` | API 仍輸出七筆；repository 意圖為五 subject 加兩訊號 |
| `EVD-CF-FOREIGN` | `internal/capitalflow/forces.go:61-120` | `extractForeign`、`extractFuturesDeprecated`、`extractTSMADR` | 外資現貨、期貨 LeadingZ 與 ADR 的同源／角色關係 |
| `EVD-CF-STATE` | `internal/capitalflow/forces.go:178-184` | `zScore` | 每次 Extract 都向 window push，造成讀取副作用 |
| `EVD-CF-RESONANCE` | `internal/capitalflow/resonance.go:20-89` | `ComputeResonance` | 主判斷過濾 role，但 aligned 重建仍可混入非 subject |
| `EVD-CF-REPORT` | `internal/capitalflow/report.go:13-85,164-179` | `GenerateDailyReport`、`computeQualityScore`、`applyForceWeights` | dominant 遍歷全部七筆、quality 只用三項、weight 跨 raw 單位 |
| `EVD-CF-SERVICE` | `internal/capitalflow/service.go:44-79,114-145` | `QualityScore`、`LatestDaily`、`Summary` | service quality 與 report quality 不是同一公式；API 讀取重跑 Extract |
| `EVD-TWSE-MAP` | `internal/marketdata/twse_capital_flow_provider.go:176-198` | `TWSECapitalFlowProvider.fetchDate` | T86 欄位是股數，現行聚合為除以 `1e8` 的億股 proxy |
| `EVD-GOV-SPEC` | `docs/specs/government-force-proxy-spec.md:8-89` | 官股代理方法論 | 無官方彙總；現行操作員匯入與既有名單矛盾 |
| `EVD-FORECAST-SPEC` | `docs/specs/foreign-flow-forecast-spec.md:20-59,84-89` | v1 features、校準門檻、局限 | 期貨／ADR 是預測特徵；領先性與固定門檻尚未實證 |
| `EVD-UI-BOARD` | `shared_web/static/js/components/seven-force-board.js:4-57` | `renderSevenForceBoard` | 七筆目前同級卡片顯示且呈現 legacy weight |
| `EVD-UI-INTERPRET` | `shared_web/static/js/components/seven-force-interpretations.js:22-102` | `renderSevenForceInterpretations` | 七筆目前共同參與全面偏多／偏空敘事 |
| `EVD-E05-MANIFEST` | `.omo/manifests/2026-07-17-retail-positioning-gap-fix-manifest.md`（進行中） | E05 | E05 的完成宣告、相容策略與預期角色 |

---

## 6. 七維資料字典

| Dimension | 正式角色 | 現行來源 | 現行 raw 單位 | 證據類別 | 主體共識 | 可作領先／確認訊號 |
|-----------|----------|----------|---------------|----------|----------|--------------------|
| `foreign` | `official_actor` | TWSE T86 外陸資淨股數 | 億股 proxy | 第一方衍生 | 是 | 是，作現貨確認 |
| `institutional` | `official_actor` | TWSE T86 投信淨股數 | 億股 proxy | 第一方衍生 | 是 | 否 |
| `dealer` | `official_actor` | TWSE T86 自營商淨股數（自行＋避險） | 億股 proxy | 第一方衍生 | 是，但需標示含避險 | 否 |
| `government` | `behavioral_proxy` | 操作員匯入／未來分點加總 | 現行檔案為新台幣元，API 轉億元 | 代理 | 僅資料品質達標後 | 可作行為確認 |
| `retail` | `behavioral_proxy` | 融資餘額與融券餘額變化 | 百分比加總 | 代理 | 僅資料品質達標後 | 可作反向／擁擠確認 |
| `futures` | `positioning_indicator` | TAIFEX 外資臺股期貨 OI net | 口數 | 第一方衍生 | 否 | 是；領先性仍待回測 |
| `tsm_adr` | `cross_market_signal` | TSM ADR 漲跌 | 百分比 | 跨市場價格 | 否 | 是；命中率仍待回測 |

### 6.1 官股口徑未決事項

現有 `government-force-proxy.md` 有內部矛盾：一處列出 8 家並包含元大，下一行又稱嚴格定義不含元大；同時未明確列入臺灣企銀。此問題未經財政部第一方名單與券商分點對照前：

- 不得宣稱已確定「八大行庫」成員；
- 操作員匯入必須保留原始來源與實際機構清單；
- `government` 維度維持 `data_available=false` 或低證據品質；
- 不得以 0 代表中性意見。

### 6.2 散戶口徑限制

融資、融券、當沖與小額持股都只是散戶行為代理。正式輸出需保留 `proxy_method`；未來 TDCC 股權分散／自然人資料可作另一個驗證維度，但不能直接把特定持股級距永久等同所有散戶。

---

## 7. API 語意契約

七筆 shape 可保留以維持 consumer 相容；每筆至少需提供：

```json
{
  "force": "futures",
  "role": "positioning_indicator",
  "evidence_class": "official_derived",
  "source_id": "SRC-TAIFEX-INST",
  "unit": "contracts",
  "as_of_trading_date": "2026-07-17",
  "data_available": true,
  "sample_count": 60,
  "calibration_status": "calibrating",
  "participates_in_actor_consensus": false,
  "raw_value": -84000,
  "z_score": -1.2,
  "trend": "bearish"
}
```

必要語意：

- `role`：不得用數字位置推測角色。
- `evidence_class`：`official`、`official_derived`、`proxy`、`cross_market`。
- `source_id`：必須能連回來源登錄表或專屬 source registry。
- `unit`：不可只顯示模糊的「億」。
- `as_of_trading_date`：以交易日為準。
- `data_available=false`：代表沒有資料，不等於 neutral。
- `sample_count`：rolling 統計的有效不同交易日數。
- `calibration_status`：`calibrating`、`eligible`、`degraded`。
- `participates_in_actor_consensus`：明確控制模型資格，不以 `deprecated` 代替。

### 7.1 `deprecated` 的處理

`deprecated` 只描述舊 API 表示法是否準備移除，不能用來表達訊號是否有金融資訊價值。futures 與 TSM ADR 都保留為有效觀測維度；是否參與特定計算由 role 與 participation 欄位決定。

### 7.2 legacy `weight`

現行 `weight = abs(raw_value) / sum(abs(raw_value))` 跨越億股、億元、口數與百分比，沒有有效金融語意。

- E06 後不得再對外稱為「勢力權重」。
- 相容期內可保留欄位但必須標示 legacy／不可用，前端不得顯示。
- 日後若需要模型貢獻度，使用經校準的 `contribution` 欄位，且附模型版本、窗口與 confidence。

---

## 8. 狀態更新與 Z-score 契約

### 8.1 讀取不得改變模型

HTTP、MCP、UI 或 recommender 的讀取請求必須是純讀。相同 source snapshot 與交易日，不論讀取幾次，回傳結果必須完全一致。

### 8.2 每個交易日只入窗一次

rolling window 以 `(dimension, as_of_trading_date)` 去重。重抓同一天只能更新該日值，不能新增第二個樣本。

### 8.3 參考窗不得混入缺失值

- `data_available=false` 不得 `push(0)`。
- 假日不補 0。
- 來源恢復後以實際交易日續接。

### 8.4 Z-score 時序

當日分數應使用前 N 個有效、不同交易日作參考，再寫入當日值：

```text
z_i(t) = (x_i(t) - mean(x_i[t-N:t-1])) / std(x_i[t-N:t-1])
```

樣本不足時回傳 `calibration_status=calibrating`，不可假裝 60 日 Z-score 已成熟。

### 8.5 持久化

rolling window 必須跨 process restart 恢復；持久化內容至少包含 dimension、date、raw value、unit、source id 與版本。此要求同時封閉 BK-15。

---

## 9. 分層判斷模型

### 9.1 法人共識 `institutional_consensus`

只使用外資、投信、自營商三個官方分類。自營商包含自行買賣與避險時，資料契約必須揭露，不能把避險倉直接解讀為方向意見。

### 9.2 行為確認 `behavioral_confirmation`

使用官股與散戶代理，但必須乘上 data quality／availability gate。資料不足時輸出 unavailable，不得改變法人共識。

### 9.3 外資部位確認 `foreign_positioning_confirmation`

聯合觀察外資現貨與外資期貨 OI，但不可把兩者當兩個獨立投票。現貨是 observed flow，期貨是 positioning feature。

### 9.4 跨市場確認 `cross_market_confirmation`

TSM ADR 是跨市場價格訊號。未來可加入 SOX、USD/TWD 等，但新增特徵不得改變「誰是資金主體」的分類。

### 9.5 總體 assessment

在完成歷史校準前，不建立固定的單一 overall coefficient。對外優先呈現四個分層結果及其資料品質。若日後組合：

- 使用同單位標準化後的特徵；
- 避免對同一外資行為重複計權；
- 記錄模型版本與參數來源；
- 需通過 out-of-sample 驗證後才能 `eligible`；
- 未達門檻時標示「校準中」，不能影響自動策略。

---

## 10. 待驗證假設登錄

| Hypothesis ID | 假設 | 最低資料 | 驗證方式 | 啟用門檻 | 目前狀態 |
|---------------|------|----------|----------|----------|----------|
| `H-CF-01` | 外資期貨 OI 領先外資現貨 1–3 日 | ≥252 交易日 | lag correlation + Granger／walk-forward | out-of-sample 顯著且方向穩定 | 未驗證 |
| `H-CF-02` | TSM ADR 對隔日台股方向具資訊力 | ≥252 交易日 | 開盤／收盤方向命中率、regime 分層 | rolling hit rate ≥55% 且樣本充分 | 未驗證 |
| `H-CF-03` | 官股代理能改善反轉／護盤辨識 | ≥90 個有效代理日 | 有無官股特徵 A/B | out-of-sample 不劣化且改善明確 | 資料不足 |
| `H-CF-04` | 現行融資融券代理可代表散戶擁擠 | ≥252 交易日 | 與 TDCC／自然人資料交叉驗證 | 相關與方向穩定 | 未驗證 |
| `H-CF-05` | 分層模型優於七項平權模型 | ≥252 交易日 | walk-forward 對照、Brier／hit rate／drawdown | 多指標不劣化 | 未驗證 |

任何 AI Agent 不得把表中的未驗證假設改寫成查證事實。

---

## 11. 2026-07-17 活體稽核快照

此節是歷史觀測，用來證明 E06 的必要性，不是永久市場結論。

### 11.1 活體回應

`capital_flow_daily` 在同一 snapshot 回傳七筆，但沒有 repository E05 已定義的 `role/deprecated/data_available` 欄位；`resonance.aligned` 為：

```json
["foreign", "tsm_adr", "dealer"]
```

表示 TSM ADR 仍被當作同級對齊項，與分層契約不符。

### 11.2 跨單位權重

活體輸出曾顯示：外資 16%、TSM ADR 4%、自營商 41%、散戶 38%。分母混合億股、百分比與其他單位，這些數字不得解讀為資金勢力權重。

### 11.3 讀取副作用

連續讀取 daily 與 summary，同一 raw snapshot 的 `quality_score` 分別為 `-4.64` 與 `-3.16`，Z-score 也改變。repository 顯示 `Extract()` 每次呼叫都 `push()` rolling window，而 API 讀取會重複呼叫 `Extract()`。

這違反「讀取不得改變模型」與「每交易日只入窗一次」。

### 11.4 部署漂移

main branch 的 E05 欄位存在但活體未輸出。E06 驗收必須記錄 runtime binary commit／version，不能只用 `git log` 代替部署證據。

---

## 12. Invariant Tracker

| ID | 不變式 | 驗證要求 |
|----|--------|----------|
| `CF-INV-01` | API 保留七個已知 dimension，且每筆角色明確 | schema + handler test |
| `CF-INV-02` | 三大法人、兩個代理、兩個訊號不得互換角色 | table-driven role test |
| `CF-INV-03` | futures／TSM ADR 不作獨立法人投票 | consensus test |
| `CF-INV-04` | 相同交易日重複讀取結果完全一致 | idempotency test |
| `CF-INV-05` | 每個 dimension 每交易日最多一個 rolling sample | persistence round-trip test |
| `CF-INV-06` | 缺資料不寫入 0、不解讀為 neutral | unavailable test |
| `CF-INV-07` | 不得以跨單位 raw value 計算共同權重 | unit guard test + legacy weight hidden |
| `CF-INV-08` | daily／summary／recommender 對同 snapshot 使用同一 QualityAssessment | end-to-end equality test |
| `CF-INV-09` | actor consensus 的 aligned/opposing 不含 indicator/signal | resonance test |
| `CF-INV-10` | dominant actor 與 dominant signal 分開 | report contract test |
| `CF-INV-11` | runtime API 必須揭露 source、unit、交易日與 data quality | contract + live probe |
| `CF-INV-12` | runtime binary version 必須可與部署 commit 對帳 | health endpoint / deployment check |
| `CF-INV-13` | 未驗證假設只能標示 calibrating，不影響自動策略 | orchestrator feature-gate test |
| `CF-INV-14` | F05 只消費穩定 `CapitalFlowAssessment`，不直接解讀七筆 raw force | dependency / integration test |
| `CF-INV-15` | rolling sample 的 `TradingDate` 必須由 snapshot 自身 `RecordedAt` 推導（Asia/Taipei YYYY-MM-DD），不得由 caller wall-clock 推導；避免 cutoff + last-write-wins 覆寫陷阱 | unit test: stub RecordedAt 強制驗證 key |
| `CF-INV-16` | 非交易日（週末／國定假日）Refresh 必須 skip-and-log，不寫入空樣本、不拋 error；nil calendar 視為交易日但記 warn | unit test: Saturday + IsTaiwanTradingDay → 0 samples |
| `CF-INV-17` | 歷史時間序列 API（如 `/api/capital-flow/historical-snapshot/{date}`）必須對未涵蓋日期回傳 `status: missing` 或 HTTP 404，不得補 0 假資料 | contract test + 端對端 probe |

---

## 13. E06 → F05 → F06 實作順序

### E06：七維錢潮模型與狀態一致性

1. 依交易日去重並持久化 rolling state。
2. 讓 API/MCP 讀取成為純讀。
3. 擴充 3+2+2 role、source、unit、quality contract。
4. 移除跨單位 raw weight 的對外語意。
5. 分離 actor consensus、behavioral confirmation、foreign positioning、cross-market confirmation。
6. 統一 daily／summary／service QualityAssessment。
7. 建立 runtime version 與 live contract probe。
8. 修正活躍文件與 UI 分層呈現；歷史 audit/archive 不回寫。

### F05：產業權重接通

- `WeightEngine` 為共同 sector 主來源；legacy BaseAllocations 補缺少 sector。
- 避免重複 macro overlay。
- 資金流調整只消費 E06 產生的穩定、帶品質資訊 `CapitalFlowAssessment`。
- E06 未達 eligible 時，feature flag 回退舊路徑並留下可觀測原因。

### F06：真實策略績效與排名

- 記錄實際 StrategyID、SessionID 交易日與真實 TAIEX benchmark。
- ComparisonEngine history 持久化並由 selector/recommender 共用。
- 資料不足顯示 warming-up，不用寫死順序假裝排名。

---

## 14. 未來 AI Agent 必讀檢查表

任何涉及 capital flow、外資預測、sector rotation、推薦或 UI 的 Agent，在研究／修改前必須：

1. 先讀本文件，不以搜尋結果中出現次數最多的說法作決策。
2. 確認要處理的是「資料維度」「資金主體」「代理」「訊號」哪一層。
3. 為每個資料欄位寫出 source、unit、as-of date、availability。
4. 將每個主張標為查證事實、工程推論、待驗證假設或歷史觀測。
5. 不把 T86 股數寫成新台幣億元。
6. 不把期貨 OI 口數與現貨股數相加。
7. 不把 TSM ADR 當資金參與者。
8. 不把融資融券直接等同完整散戶流。
9. 不把官股缺資料的 0 當成中性訊號。
10. 不因 API 回七筆就假設七項參與同一模型。
11. 若新研究與本文件衝突，先提出 source matrix、反例與可驗證變更，不得直接覆寫。
12. 檢查 runtime binary 與 main commit 是否一致，避免拿舊服務否定新程式或反之。

---

## 15. 文件同步與防止再分裂

本文件為分類學與計算語意唯一正本。其他文件只能摘要並連回本文件：

- `docs/reference/product-positioning.md` §7：保留產品摘要與本文件連結。
- `docs/specs/government-force-proxy-spec.md`：只維護官股代理來源與口徑。
- `docs/specs/foreign-flow-forecast-spec.md`：只維護外資預測 target、features 與校準。
- `internal/capitalflow/AGENTS.md`：只列高頻陷阱並連回本文件。
- `docs/reference/tool-catalog.md`、MCP descriptions、investor docs：使用「七維錢潮雷達／3+2+2」摘要。
- audit、manifest、CHANGELOG、archive：保留歷史原文，必要時註明 superseded-by。

若其他活躍文件重新建立一份完整七維定義表，視為重複權威來源，必須刪除重複內容並改成連結。

---

## 16. 決策紀錄

| 日期 | 決策 | 理由 | 取代內容 |
|------|------|------|----------|
| 2026-07-17 | 保留七個觀測維度 | futures 與 TSM ADR 有不同訊息價值，不應刪除 | 「只剩五項」的過度簡化 |
| 2026-07-17 | 採 3+2+2 分層 | 對齊官方分類、代理品質與訊號角色 | 七項平權模型 |
| 2026-07-17 | 禁止跨單位 raw weight | 億股、億元、口數、百分比不可同分母 | `abs(raw)/sum(abs(raw))` 勢力權重語意 |
| 2026-07-17 | 先做 E06，再接 F05/F06 | 活體讀取會改變 Z-score，且部署與 main 語意不一致 | 直接把不穩定 capital flow 接策略層 |

---

## 17. 自審結論

- 文件沒有把未驗證的領先天數、命中率或相關係數寫成事實。
- 七項資料的來源、單位、證據品質與計算資格已分開。
- API 向後相容與金融工程語意已分開。
- 歷史文件、活體輸出與規範文件的權威層級已明定。
- 官股名單矛盾被明確隔離，未猜測填補。
- E06、F05、F06 的依賴順序與 invariant 可直接轉成 implementation plan。

---

## 18. Historical Timeline & Recording Semantics

本章補齊 CL-1（cutoff 覆寫 bug）與 CL-6（recorded_at 語意）對應的契約，作為後續 `/api/capital-flow/historical-snapshot/*` 與 `/api/macro/snapshot/history?days=N` 的設計基礎。

### 18.1 Trading-Date Key 必須 data-driven（CF-INV-15）

`RollingSample.TradingDate` 欄位的決定者為 snapshot 自身的 `RecordedAt`（converted to Asia/Taipei `YYYY-MM-DD`），不得由呼叫端的 wall-clock 或排程時間推導。

理由：caller-driven keying 與 `applyUpsert` last-write-wins 互動時，會形成「cutoff 之前的所有 tick 把同一個 slot 重寫 N 次」的陷阱（已於 2026-07-19 立案 CL-1 證實）。data-driven keying 保證冪等性：同一天資料的任何次數呼叫，最終只留下一筆。

### 18.2 非交易日 Skip-and-Log（CF-INV-16）

Refresh 在寫入前必須檢查 `eventCalendar.IsTaiwanTradingDay(recordTime)`：

- **是交易日**：走原 UpsertDay 流程。
- **非交易日**：log info 等級的 `skip_non_trading_day` 訊息後 `return nil`。不寫入空樣本（CF-INV-06），不拋 error（避免吵雜 retry）。
- **nil calendar**：log warn 等級的 `refresh_no_calendar` 訊息後視為交易日繼續寫入。Nil calendar 不會 panic，目的是讓測試環境與錯誤 wiring 不會阻斷 hot path，但 observability 必須暴露這個退化。

### 18.3 Historical Snapshot API 契約（CF-INV-17）

CF-INV-17 規範「歷史時間序列 API 必須對未涵蓋日期回傳 `status: missing` 或 HTTP 404，不得補 0 假資料」。實作分兩個 sub-section：

#### 18.3.1 `HandleHistory` 的 opt-in meta 模式（已實作 — `.omo/manifests/2026-07-20-cl5-capital-flow-handlehistory.md`）

既有 `/api/capital-flow/history` handler 採 **opt-in** 設計：保留既有 flat shape `{foreign: [...], government: [...]}` 向後相容 H02 frontend，新增 `?include_meta=true` 開關暴露 status。

**預設行為**（不傳 `include_meta`）：
```json
{
  "foreign":       [{"trading_date":"2026-07-17","raw_value":...,"unit":"億股","source_id":"..."}, ...],
  "institutional": [...],
  "dealer":        [...],
  "government":    [],
  "retail":        [],
  "futures":       [...],
  "tsm_adr":       [...]
}
```
- 既有 H02 frontend `shared_web/static/js/pages/capital-history.js`（commit 04622ab1）以 `currentData[d.key]` 讀取 array，**不受影響**。

**開啟 `?include_meta=true`**：
```json
{
  "samples": {
    "foreign":       [...],
    "institutional": [...],
    "dealer":        [...],
    "government":    [],
    "retail":        [],
    "futures":       [...],
    "tsm_adr":       [...]
  },
  "meta": {
    "status": "partial",
    "missing_dimensions": ["government", "retail"],
    "days_requested": 60,
    "days_returned": 60,
    "data_status": {
      "government": {"data_available": false, "missing_reason": "pre_2018_or_no_provider"},
      "retail":     {"data_available": false, "missing_reason": "..."},
      "foreign":    {"data_available": true}
    }
  }
}
```

**`meta.status` 枚舉**（與 §18.3 point-in-time 共用）：
- `"complete"`：七個官方+代理+指標維度全有資料
- `"partial"`：部分維度有資料（至少 1 個 missing_dimension）
- `"missing"`：所有維度都沒資料（store 完全空）

**`missing_dimensions`**：列出 `samples` map 內對應值為空 slice `[]` 的 dimension key（force 名）。

**`data_status`**：每個 dimension 的 `data_available` 旗標 + 缺失原因（per AGENTS.md「PublicBank 欄位歷史較短」警告，公股行庫 TWSE 約 2018+ 才完整）。

#### 18.3.2 Point-in-time snapshot endpoint（**已實作** — CL-5b / PR #1233, commit `92fe3f74`, 2026-07-20）

`/api/capital-flow/historical-snapshot/{trading_date}` 端點契約如下（由 `internal/capitalflow/handler.go:80` `HandleHistoricalSnapshot` 實作，由 `cmd/atlas/main.go:754` 掛載於 mux）：

- **Response 結構**：`{trading_date, status, dimensions: {<force>: {raw_value, unit, source_id, data_available} | null}, missing_reason?: string}`
- **`status` 枚舉**：`"complete"`（七維度全有）｜`"partial"`（部分維度有資料）｜`"missing"`（當日無資料或非交易日）
- **HTTP 狀態碼**：`200` 不論 status（status 在 body 內）；禁止用 404 偽裝 missing。
- **缺資料語意**：對 `data_available=false` 的維度（如 government 早期 2018 前資料）回 `null` + `"data_available": false`，禁止補 0。
- **Backlog note**：BL-CF-01（store 歷史資料 backfill）仍未 ship — 端點已能用，但目前只能查已 Refresh 寫入 store 的交易日（單日 / 近期）。要看任意過去 252 個交易日仍需 backfill pipeline。

### 18.4 RecordedAt vs filename date 語意分離（CL-6 對應）

兩個欄位**不應混用**：

- **`filename date`**（如 `data/state/macro/2026-07-15.json`）：provider 命名 snapshot 時的「資料所屬日期」。
- **`recorded_at`**（Unix int64）：provider 真正拉到資料的時間戳，可能晚於 filename date（cache 回填、batch ingest 補資料）。

前端與 agent 必須明確知道：
- 「今天的收盤資料」對應 `filename date = today, recorded_at = today 14:30+`。
- 「昨天的歷史回填」對應 `filename date = yesterday, recorded_at = today`。
- 任何 T+1 retrospective 必須以 `filename date` 對齊到該日的結論，不可僅依賴 `recorded_at`。

### 18.5 Capacity Gate（CF-INV-15 補強）

Rolling sample store 的 capacity 對齊 spec §10 `H-CF-05` gate：

| 位置 | 值 | 變更時間 |
|------|------|---------|
| `cmd/atlas/main.go`：`NewFileRollingSampleStore(..., 252)` | **252** | `.omo/manifests/2026-07-20-cl5-capital-flow-handlehistory.md` A01 |
| `internal/capitalflow/service.go`：`const defaultHistoryLimit = 252` | **252** | 同上 |
| `internal/capitalflow/handler.go`：`days := 252` + cap `n > 252` | **252** | 同上 |

**為何 252**：spec §10 `H-CF-05` 要求「分層模型優於七項平權模型」需 ≥252 交易日 walk-forward 對照；252 = 一年 trading days（扣假日）。

**Cap 行為**：`?days=999` 自動 clamp 至 252；不會回 error。

**Backlog 警告**：capacity 提升僅是上限，不會自動回填歷史資料。當前 store 內只有 `2026-07-17` 一筆（post-A01 但 15:30 CST cutoff 前）。歷史 backfill 入 **BL-CF-01**（需 Provider 提供歷史 API 或 replay 機制）。

### 18.7 Session List + Detail API（CL-4 — `.omo/manifests/2026-07-20-cl4-sessions-drilldown.md`）

#### 18.7.1 背景

MCP `universe_get_sessions` 工具（→ `GET /api/dashboard/sessions`）當前只回 4 個 metadata fields（`session_id / recorded_at / regime / outcome_count`），per-hermes 觀察缺 per-strategy data。

深入 audit 揭露：per-strategy outcome data **已存在** 在 SQLite `outcomes` table（欄位：`agent_id / symbol / action / weight / target_price / stop_loss / conviction / regime / timestamp / passed_guards / guard_reason / factor_scores_json / conviction_breakdown_json`），但 `HandleSessions` handler 沒 expose。本節補兩個改進：

- **A1+A2 (List 端)**：每個 session object 加 `top_strategies` 聚合欄位（top N by conviction）
- **B1+B2 (Detail 端)**：新 endpoint `GET /api/dashboard/sessions/{id}` + 新 MCP tool `universe_get_session_detail` 拿完整 per-strategy outcomes

#### 18.7.2 `GET /api/dashboard/sessions` 增強版 response

**Before**（4 fields）：
```json
{
  "sessions": [
    {
      "session_id": "session-20260101-daily",
      "recorded_at": "2026-01-01T...",
      "regime": "RISK_ON",
      "outcome_count": 27
    }
  ]
}
```

**After**（5th field `top_strategies`，向後相容 — 既有 client 讀 4 個 fields 不受影響）：
```json
{
  "sessions": [
    {
      "session_id": "session-20260101-daily",
      "recorded_at": "2026-01-01T...",
      "regime": "RISK_ON",
      "outcome_count": 27,
      "top_strategies": [
        {"agent_id": "earnings-quality-01", "symbol": "3008.TW", "action": "BUY", "conviction": 79.0, "passed_guards": 1},
        {"agent_id": "value-yield-01",      "symbol": "2891.TW", "action": "BUY", "conviction": 75.0, "passed_guards": 1},
        {"agent_id": "financials-desk-01",  "symbol": "2891.TW", "action": "BUY", "conviction": 73.0, "passed_guards": 0}
      ]
    }
  ]
}
```

**Top N 預設**：3 個（per session, by `conviction DESC`）。
**Nil-safe**：若 session 沒 outcomes，`top_strategies = []`（非 null）。

#### 18.7.3 `GET /api/dashboard/sessions/{id}` 新 endpoint

**Request**：`GET /api/dashboard/sessions/session-20260101-daily`

**Response 200**（calls existing `LoadSessionOutcomes(sessionID)`）：
```json
{
  "session_id": "session-20260101-daily",
  "recorded_at": "2026-01-01T...",
  "regime": "RISK_ON",
  "outcome_count": 27,
  "summary": { ... SessionSummary 全部 20+ fields ... },
  "outcomes": [
    {
      "symbol": "3008.TW",
      "agent_id": "earnings-quality-01",
      "action": "BUY",
      "target_price": 250.0,
      "stop_loss": 240.0,
      "conviction": 79.0,
      "regime": "official_actor",
      "timestamp": "2026-01-01T...",
      "passed_guards": 1,
      "guard_reason": "...",
      "factor_scores": {...},
      "conviction_breakdown": {...}
    }
  ]
}
```

**Response 404**：sessionID 不存在。
**Response 503**：ledger store 不可用（既有 `LoadSessions` 503 語意）。

#### 18.7.4 MCP wrapper 對應

| MCP tool | 對應 HTTP | 用途 |
|----------|------------|------|
| `universe_get_sessions` | `GET /api/dashboard/sessions` | list all sessions（**現有**，加 `top_strategies` 摘要） |
| `universe_get_session_detail` | `GET /api/dashboard/sessions/{id}` | **新**：per-session drill-down（完整 outcomes） |

**input schema**：
```go
type UniverseGetSessionDetailInput struct {
    SessionID string `json:"session_id" jsonschema:"the session_id from universe_get_sessions"`
}
```

**output schema**：同 §18.7.3 response。

#### 18.7.5 Whitelist 同步

- `cmd/atlas/main.go isPublicPath` 加 `/api/dashboard/sessions/{` prefix case
- `internal/monitoring/api/shared/handler.go authFreePrefixPaths` 加 `/api/dashboard/sessions/` prefix

（`/api/dashboard` 全 prefix 已 public，新子路徑自動繼承。但 `isPublicPath` 需顯式 case 對應 mux — 對齊既有 pattern。）

#### 18.7.6 設計決策

- **N+1 query**（不單一 window function SQL）：session 數量 bounded (~100)，N+1 perf 足夠；不需新加 `OutcomeStore` interface method
- **SessionMeta 加 `TopStrategies` 欄位**（nil when not requested）：Go 結構 field 向後相容；既有 `LoadSessions` caller 拿 nil 不 panic
- **drill-down 直接呼叫既有 `LoadSessionOutcomes`**（不重複 SQL query）：AGENTS.md「同一件事不可有三種算法」守則
- **404 vs 500**：sessionID 不存在回 404（找不到語意），非 500（系統錯誤語意）

### 18.6 Historical Regime Observation Store（Wiring Gap 修正 — `.omo/manifests/2026-07-20-cl3-regime-history.md`）

#### 18.6.1 Wiki-vs-Reality 揭露（必讀）

`atlas-wiki/queries/atlas-mcp-capital-flow-history-truth-seeking-2026-07-19.md` §6.4 + §4 CL-3 描述**已過時**（wiki 寫於 2026-07-19，當前 codebase 2026-07-20 已部分 ship；此檔案位於 hermes 私域 `~/workspace/atlas-wiki/queries/...`，**不在本 repo 內**）。**本 §18.6 為準**：

| 維度 | Wiki 描述（過時） | 當前現實（2026-07-20） |
|------|------------------|----------------------|
| 時序存儲 | 「沒有每個交易日一個 regime score 的時序存儲」 | **`regime_history` SQLite 表已有 90 筆真實資料**（4-01 到 6-29） |
| Store 介面 | 「需新建 `RegimeObservationStore`」 | **`HistoricalStore` interface + SQLite impl 已 ship**（`internal/ledger/historical_store.go`） |
| 寫入路徑 | 「JANUS 6h 排程要每天寫」 | **`stage4-loader` 已 backfill**（`cmd/atlas-stage4-loader/main.go:380`）；runtime writer 待 BL-CL3b |
| 真實 score endpoint | `/api/janus/regime-score` | **route 不存在**（本 PR §18.6.3 新增） |

#### 18.6.2 `PipelineService.LoadRegimeHistory` 讀對 store

**Before**（service.go:1073，`InternalMonitoringService.LoadRegimeHistory` 讀 `LoadSessionSummaries` 是 simulation session metadata）：
```go
func (s *PipelineService) LoadRegimeHistory(limit int) (*RegimeHistoryData, error) {
    summaries, err := s.store.LoadSessionSummaries()  // ❌ filesystem sessions
    ...
}
```

**After**（本 PR A01，`WithHistoricalStore` builder pattern + nil-safe fallback）：
```go
type PipelineService struct {
    WorkDir           string
    LedgerDir         string
    store             ledger.OutcomeStore
    historicalStore   ledger.HistoricalStore  // 新欄位
    ...
}

func (s *PipelineService) WithHistoricalStore(hs ledger.HistoricalStore) *PipelineService {
    s.historicalStore = hs
    return s
}

func (s *PipelineService) LoadRegimeHistory(limit int) (*RegimeHistoryData, error) {
    if s.historicalStore != nil {
        return s.loadFromRegimeHistory(s.historicalStore, limit)  // ✅ 真實時序
    }
    // fallback: 既有 LoadSessionSummaries 路徑（向後相容 43 個 test caller）
    return s.loadFromSessionSummaries(s.store, limit)
}
```

**`loadFromRegimeHistory`** 把 `RegimeRow` 轉成 `RegimeSessionEntry`：map `trading_date → date`, `regime → regime`, `source_session_id → session_id`。保留 `current_regime` + `transitions` 計算邏輯。

**Acceptance**：
- 既有 `TestLoadRegimeHistory_*` 全綠（nil-safe fallback 路徑）
- 新增 `TestLoadRegimeHistory_HistoricalStore_OK` PASS

#### 18.6.3 `/api/janus/regime-score` HTTP Endpoint（本 PR B01 新增）

**Endpoint**：`GET /api/janus/regime-score`

**Response 200**：
```json
{
  "score": -21.26,
  "is_synthetic": true
}
```

**`is_synthetic=true`**：當前實作；macro snapshot synthesize composite score（per `janus.Engine.GetCurrentRegimeScore`）。
**`is_synthetic=false`**：PRISM training populated 後回傳真實 Sharpe average（BL-CL3b 範圍）。

**Formula**（canonical，per `internal/janus/composite_score_test.go:18`）：
```
score = tanh(foreignFlow/5e9) * 30 - max(0, VIX-20) * 1.5
```

**公式一致性守則**：MCP wrapper 與 janus engine **必須**使用同一公式。**任何第三方實作都不允許**（per AGENTS.md「同一件事不可有三種算法」陷阱）。本 PR B02 刪除 MCP wrapper `fetchRegimeCompositeScore` 重複實作（原本用 `/5` 而非 `/5e9`，公式錯誤）。

#### 18.6.4 MCP `regime_get_history` 整合（修法後行為）

**修法後流程**：
1. MCP 工具 `regime_get_history(days=N)` 呼叫 `/api/dashboard/regime-history?limit=N`
2. **修法後** 該 endpoint 透過 `PipelineService.LoadRegimeHistory` 讀 `regime_history` SQLite 表，回 `RegimeSessionEntry[]` 真實時序
3. MCP wrapper 對每個 regime 點呼叫 `/api/janus/regime-score` 拿 score（**修法後**該 endpoint 真的存在）
4. 組裝 `RegimePoint{Date, Regime, Score}` 回傳
5. `RegimePoint.Score *int` 用 omitempty — 若 `/api/janus/regime-score` 失敗，Score 欄位 omitted（honest unknown），不報 0

**Before（hermes 觀察的 bug）**：
- `/api/dashboard/regime-history` 回 simulation sessions（不是 regime 時序）
- `/api/janus/regime-score` 永遠 404 → fallback `fetchRegimeCompositeScore` 公式錯誤
- 結果：散亂 events + score=0

**After（本 PR fix）**：
- `/api/dashboard/regime-history` 回 regime_history 表真實時序（90 筆資料）
- `/api/janus/regime-score` 真的存在，回 macro-derived composite score
- 結果：每天 regime label（NEUTRAL/RISK_ON/RISK_OFF/TRANSITIONAL）+ 對應 score
