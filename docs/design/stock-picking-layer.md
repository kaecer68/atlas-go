# ATLAS 選股層策略庫 — 設計文件（Phase 4）

> **版本**: v0.1（草案）
> **日期**: 2026-07-27
> **狀態**: 設計階段，尚未實作
> **對應憲章**: `docs/ATLAS_METHODOLOGY.md` Phase 4
> **審計項目**: F5 (DeepSeek 覆核)

---

## 一、背景

ATLAS 方法論憲章目前涵蓋**組合層**（portfolio-level）的策略映射：
- 七時期 → 六策略（all_weather / value / growth / momentum / event_arbitrage / cash_only）
- 時期 → 現金部位建議
- 時期 → RiskLevel 動態調整

但憲章 Phase 4 規劃的**選股層**（stock-picking layer）尚未設計。選股層解決的問題是：
> 給定當前時期與推薦策略類別，如何從全市場中選出具體個股？

---

## 二、設計原則

1. **時期感知**：選股邏輯必須跟隨七時期動態調整。上升期選成長股，低迷期選防禦股。
2. **策略對齊**：選股條件必須與策略類別一致。all_weather → 低波動高股息，momentum → 價格動能強勢。
3. **資金流向驗證**：任何選股結果必須經過 capitalflow 驗證（該個股是否有聰明錢流入）。
4. **分層執行**：先由策略類別決定篩選條件，再由時期調整權重，最後由資金流向驗證。

---

## 三、策略→選股條件映射（草案）

| 策略類別 | 選股條件 | 範例指標 |
|---------|---------|---------|
| **all_weather** | 低 beta + 高股息 + 穩定營收 | Beta < 0.8, Dividend Yield > 4%, ROE > 10% |
| **value** | 低本益比 + 低股價淨值比 + 穩定現金流 | PE < 12, PB < 1.5, FCF Yield > 5% |
| **growth** | 營收成長 + EPS 成長 + 產業前景 | Revenue Growth > 15%, EPS Growth > 20% |
| **momentum** | 價格動能 + 成交量擴大 + 外資買超 | 20日漲幅 > 10%, 成交量 > 20日均量 1.5x |
| **event_arbitrage** | 事件驅動 + 短期錯價 | ETF 調整標的、MSCI 變更、營收驚喜 |
| **cash_only** | 無選股（全現金） | — |

---

## 四、時期權重調整矩陣

| 時期 | all_weather | value | growth | momentum | event_arbitrage |
|------|------------|-------|--------|----------|----------------|
| 低迷 | **1.5x** | **1.3x** | 0.5x | 0.3x | 0.5x |
| 轉折開高 | 0.7x | 0.8x | **1.5x** | **1.3x** | 1.0x |
| 上升 | 0.5x | 0.5x | 1.3x | **1.5x** | 1.0x |
| 高原 | 1.0x | 1.0x | 0.8x | 0.8x | **1.5x** |
| 盤整 | **1.3x** | 1.0x | 0.5x | 0.5x | **1.3x** |
| 轉折下壓 | **1.5x** | 1.0x | 0.3x | 0.2x | 0.3x |
| 黑天鵝 | — | — | — | — | — |

---

## 五、資金流向驗證閘道

每個選股候選必須通過三層驗證：

1. **外資流向**：近 5 日外資買超 > 0（或賣超幅度 < 該股 20 日均量的 10%）
2. **投信流向**：近 5 日投信無連續大賣（單日賣超 < 該股 20 日均量的 5%）
3. **散戶反向**：融資使用率 < 80%（或融資減少中）

> 黑天鵝時期：全部閘道關閉，不選股。

---

## 六、與既有系統的整合路徑

### 既有可復用模組

| 模組 | 用途 |
|------|------|
| `internal/capitalflow/` | 資金流向驗證（ForceRetail / ForceForeign / ForceInstitutional） |
| `internal/methodology/advisor.go` | 時期→策略映射（可擴展為時期→選股條件） |
| `internal/portfolio/period_detector.go` | 七時期判斷 |
| `internal/eventdriven/` | 事件套利標的識別 |
| `internal/stocktools/` | 個股基本面數據 |

### 需要新建的模組

| 模組 | 說明 |
|------|------|
| `internal/stockpicker/` | 選股引擎：接收時期+策略 → 輸出個股清單 |
| `internal/stockpicker/conditions.go` | 策略→選股指標對應（PE/PB/ROE/股息率/動能等） |
| `internal/stockpicker/validator.go` | 資金流向驗證閘道 |

---

## 七、實作優先級

| 階段 | 內容 | 優先級 |
|------|------|--------|
| 1 | 選股條件定義（策略→指標映射表） | P1 |
| 2 | 資金流向驗證閘道（三層過濾） | P1 |
| 3 | 時期權重整合（與 MethodologyAdvisor 對接） | P1 |
| 4 | 回測驗證（歷史數據測試選股績效） | P2 |

---

> **下一步**：本文件審閱通過後，建立 `internal/stockpicker/` 模組骨架，先實作策略→指標映射表（階段 1）。
