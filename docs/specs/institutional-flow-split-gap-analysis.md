# 法人資金分流方法論覆核（F1–F4 Gap Analysis）

> **文件角色**：憲章治理追蹤表 §附錄 F「F1–F5：DeepSeek 方法論覆核」的實作交付物（2026-08-07）。
> **結論**：F1–F4 的**資料分流欄位已全部存在**於 `MacroDataSnapshot`；覆核重點是「欄位 → 方法論消費者」的 gap。F1/F2 為欄位存在但評分未消費；F3/F4 已有消費者。
> **對齊**：`docs/ATLAS_CONSTITUTION_AUDIT.md` §附錄 F、`docs/specs/capital-flow-seven-dimension-spec.md`、`docs/specs/government-force-proxy-spec.md`。

---

## 1. 覆核目的

§附錄 F 的 F1–F4 是方法論覆核項目（「DeepSeek 覆核」），非新功能開發。本文件以 codebase 實況驗證四個「法人分流」方法論的落地程度：

| # | 方法論 | 核心問題 |
|---|--------|----------|
| F1 | 外資雙重動機模型 | 外資買賣超是結構性（長期配置）還是投機性（短線操作）？能否分流？ |
| F2 | 自營商大小分流 | 自營商避險（大型、宏觀）與自行買賣（小型、個股）能否分流？ |
| F3 | 投信主動 vs 被動分流 | ETF 被動買盤與主動基金操作能否分流？ |
| F4 | 公股分點追蹤 | 官股行庫資金能否以每日可觀測方式追蹤（BK-13 替代）？ |

## 2. 現況矩陣：分流欄位 vs 方法論消費者

> 欄位定義：`internal/marketdata/macro_provider.go` `MacroDataSnapshot`（20–60 行）。
> 消費者：`internal/capitalflow/forces.go` `ForceExtractor` 評分、`internal/retail/rsi_tw_calculator.go` 零售指標。

| 方法論 | 分流欄位（已存在） | 消費者 | 落地狀態 |
|--------|-------------------|--------|----------|
| F1 外資雙重動機 | `ForeignInvestorNet`（外陸資/結構性）、`ForeignDealerNet`（外資自營商/投機性） | `scoreForeign`（forces.go:126）只用 `ForeignInvestorNet` + `ForeignFuturesOINet`；`ForeignDealerNet` **零消費者** | ⚠️ 欄位在，評分未分流 |
| F2 自營商分流 | `DealerNet`（合計）、`DealerSelfNet`（自行）、`DealerHedgingNet`（避險） | `scoreDealer`（forces.go:237）只用合計 `DealerNet`；分流欄位**零消費者** | ⚠️ 欄位在，評分未分流 |
| F3 投信主動/被動 | `DomesticFundNet`（投信）、`ETFNetSubscription`（ETF 申贖/被動） | `scoreInstitutional`（forces.go:215）用 `DomesticFundNet`；`ETFNetSubscription` 原由 `rsi_tw_calculator.go:362` 消費，**資料源已移除（TWT44U → 404，2026-08-10），subC3 停用** | ⚠️ 受阻（被動資金觀測點失效，淨化訊號暫不可行） |
| F4 公股追蹤 | `GovernmentNet` | `scoreGovernment`（forces.go:266）已消費；`government_flow_provider.go` seam 提供 operator-imported 讀取 | ✅ 已落地（每日總額形式） |

資料流旁證：`internal/monitoring/gateway_adapter.go:299-312` 將 `ForeignDealerNet` / `DealerSelfNet` / `DealerHedgingNet` 從上游傳入 snapshot —— 欄位在資料層完整流動，僅在**評分層**未被使用。

## 3. 逐項分析

### F1 外資雙重動機（結構性 vs 投機性）

**現況**：T86 原始資料（`twse_capital_flow_provider.go:24`）已分 `ForeignInvestorNet`（外陸資，不含外資自營商）與 `ForeignDealerNet`（外資自營商）兩欄。snapshot 保留此分流，但 `scoreForeign` 只用結構性欄位與期貨未平倉。

**gap**：外資自營商（投機性）訊號未進任何評分。外資自營商行為接近 hedge fund 短線操作，與外陸資長期配置動機不同，合併計算會稀釋結構性訊號。

**落地建議**（未來，需回測驗證）：
1. `scoreForeign` 增加投機性分支：`ForeignDealerNet` 與 `ForeignFuturesOINet` 合成「外資投機 Z-score」。
2. 七維錢潮雷達的「外資」維度拆分為「結構性」與「投機性」兩 sub-signal，或僅在投機性極端值時調整權重。
3. 需先跑歷史回測確認分流後 signal 的獨立預測力（對照 `docs/specs/capital-flow-seven-dimension-spec.md` 的 Z-score 框架）。

### F2 自營商大小分流

**現況**：T86 已分 `DealerSelfNet`（自行買賣）與 `DealerHedgingNet`（避險）。snapshot 保留分流，`scoreDealer` 只用合計。

**gap**：避險（大型、宏觀、衍生性連動）與自行（小型、個股操作）動機不同。合計計算會把避險操作的對沖行為誤判為方向性訊號。

**落地建議**（未來）：
1. `scoreDealer` 拆分：避險欄位納入宏觀/衍生性分析（與外資期貨 OI 併看），自行欄位保留為個股層訊號。
2. 「大型可納宏觀，小型用 AI 分點」的 AI 分點部分需 broker-branch 資料（見 F4），目前不可行。

### F3 投信主動 vs 被動分流

**現況**：`DomesticFundNet`（投信買賣超）進投信評分。**`ETFNetSubscription` 資料源已移除**（TWSE TWT44U 彙總報表 → HTTP 307 → 404，2026-08-10 容器內實測）。ETF 投資人資訊（NAV/PCF/折溢價）仍公開於 ETFortune，但**申購贖回淨額**無等價公開替代（OpenAPI opendata 44 個 dataset 無此項、FinMind 僅 ETF 持股）。原消費者 `rsi_tw_calculator` subC3 已停用（回 0 + IsFallback）。

**gap 評估**：被動資金觀測點失效，F3「投信買賣超 − ETF 被動成分」的淨化訊號**暫不可行**（需先恢復被動資金觀測，見 known_issues `twse_etf_upstream_60d`）。

**落地建議**（受阻，解除條件）：
1. 找到申購贖回淨額的公開替代資料源（TWSE OpenAPI opendata 目前 44 個 dataset 無此項；FinMind 僅 ETF 持股）。
2. 恢復 `ETFNetSubscription` 填充後，`scoreInstitutional` 以 `DomesticFundNet − ETF 被動成分` 為主動訊號。

### F4 公股分點追蹤

**現況**：`scoreGovernment`（forces.go:266）已消費 `GovernmentNet`；`government_flow_provider.go` 提供每日讀取 seam（operator-imported / broker-aggregate / media-curated 三來源）；`docs/specs/government-force-proxy-spec.md` 已定義代理方法論（8 家公股行庫分點加總為首選路徑）。

**落地狀態**：✅ 已以「每日總額」形式落地。CAPTCHA 啟用後 F4 由 fallback 升級為主要方案（§附錄 F 已註記），但**分點層級**資料仍需 broker-aggregate 來源（BK-13/14 backlog）。

**殘留**：分點層級追蹤（8 家行庫旗下券商逐日加總）未實作——缺資料源，非程式 gap。

### F5 選股層策略庫（Phase 4）

不在本文件範圍：純設計項目，待 T27 選股層策略庫。

## 4. 建議決策

| 項 | 決策 | 理由 |
|----|------|------|
| F1 | **暫緩落地**，維持欄位存在；下次方法論 wave 以回測驗證投機性 sub-signal 獨立預測力 | 改變評分行為需先有統計證據；`ForeignDealerNet` 資料已就緒，落地成本低但驗證成本高 |
| F2 | **暫緩落地**；避險/自行分流併入 F1 同一回測 wave | 同上；AI 分點部分受資料源限制 |
| F3 | **受阻**：被動資金觀測點失效（TWT44U → 404），淨化訊號暫不可行 | 需先恢復 ETF 申贖資料源（見 known_issues `twse_etf_upstream_60d`）；屬增量改進 |
| F4 | **維持現狀**（每日總額已落地）；分點層級待 BK-13/14 資料源 | seam 已備，缺資料非程式問題 |

## 5. 覆核結果（§附錄 F 對應更新）

- **F1**：⬜ → ✅ 已覆核（欄位存在、消費者 gap 已文件化、落地建議已列）
- **F2**：⬜ → ✅ 已覆核（同上）
- **F3**：⬜ → ✅ 已覆核（部分落地確認 + 淨化建議）
- **F4**：⬜ → ✅ 已覆核（已落地確認 + 分點殘留說明）
- **F5**：⬜ 維持（待 T27）

> 覆核 = 驗證資料分流欄位與方法論消費者的對齊狀態，並文件化落地路徑。**不改變任何評分行為**；「暫緩落地」項目由 §附錄 F 下次審計（v1.2）追蹤。

---

> **最後更新**：2026-08-07。commit 見 git log（PR #1490）。
