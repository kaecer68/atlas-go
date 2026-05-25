# Phase 6 主計畫：從「能做」到「可信」

**日期**: 2026-05-26
**狀態**: 規劃中
**工作區數**: 4 個平行

---

## 核心命題

Phase 6 的原目標是「Real broker integration + Live order management」。
但實地盤點後發現：

1. **模擬 pipeline 跑不穩** — 數據時有時無，執行路徑斷裂，前端看不到錯誤點
2. **沒有自動更新的系統地圖** — 每次 AI 迭代都花幾小時重新盤點
3. **決策鏈不可視** — 後端有 data，前端分散在各頁面，組不成「從事件→決策」的一條線
4. **用戶信心來自可驗證的預測** — 不是 portfolio 總績效，而是日級、產業級、個股級的預測準度

所以 Phase 6 的正確順序是：**先讓系統可見、可信，再接通真金白銀。**

---

## 四大工作區

```
W1: 系統地圖自動化    ← 讓 AI 不再重複盤點
W2: 模擬 pipeline 固化 ← 讓模擬可重複、可稽核、可除錯
W3: 決策可視化鏈      ← 讓用戶看到：事件→標的→進出場
W4: 事件邏輯庫        ← 讓系統從歷史中學習因果，自我精進
```

### 依賴關係

```
W1 (獨立) ──────────────────────────────┐
W2 (獨立) ──────────────────────────────┤
W4 (獨立，但 W3 會 consume 其 API) ──────┤
W3 (依賴 W4 的 API contract) ────────────┤
                                         ↓
                                   整合驗證階段
```

W1/W2/W4 可以完全併行。W3 的前端可以先做 UI 骨架，後端對接等 W4 有 API 再串。

---

## 工作區範圍

| W# | 名稱 | 碰的檔案 | 產出 |
|----|------|---------|------|
| W1 | 系統地圖 | `.omo/maps/`, `scripts/`, git hooks | 自動更新的系統藍圖 |
| W2 | 模擬固化 | `cmd/atlas/`, `internal/orchestrator/`, `internal/sim/` | 可重複模擬 + 稽核軌跡 |
| W3 | 決策可視化鏈 | `web/static/`, 前端 pages/components | 「即時事件→產業→標的→進出場」一條線 |
| W4 | 事件邏輯庫 | `internal/eventlogic/` (新建), `internal/monitoring/api/` | 自我精進的事件因果庫 |

---

## 不需要碰的檔案（保護區）

- `internal/marketdata/` — 前兩個 PR 剛修完，不要動
- `internal/portfolio/` — 同上
- `internal/config/parameters*.go` — 只讀不寫
- `cmd/atlas/main.go` — W2 可加 flag / W1 可加 hook，但不要改現有邏輯

---

## 每個工作區的提示詞

見對應檔案：
- `.omo/phase6/W1-system-map.md`
- `.omo/phase6/W2-simulation-health.md`
- `.omo/phase6/W3-decision-chain.md`
- `.omo/phase6/W4-event-logic-lib.md`

每個提示詞都包含：
1. 任務目標
2. 現狀（from 實際盤點）
3. 要產出的檔案清單
4. 不可碰的檔案
5. 驗證條件
6. 完成報告格式

---

## 整合順序

```
Week 1: W1+W2+W4 併行啟動
Week 2: W3 啟動（等 W4 的 API contract 定義好）
Week 3: 整合測試 → W1 地圖自動包含 W2/W3/W4 變更
```
