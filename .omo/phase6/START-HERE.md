# Phase 6 啟動指引

## 四個工作區總覽

| W# | 名稱 | 優先 | 可以併行？ | 讀哪個 prompt |
|----|------|------|-----------|-------------|
| W1 | 系統地圖自動化 | 🔴 P0 | ✅ 與 W2/W4 完全獨立 | `.omo/phase6/W1-system-map.md` |
| W2 | 模擬 pipeline 固化 | 🔴 P0 | ✅ 與 W1/W4 完全獨立 | `.omo/phase6/W2-simulation-health.md` |
| W4 | 事件邏輯庫 | 🟡 P1 | ✅ 與 W1/W2 完全獨立 | `.omo/phase6/W4-event-logic-lib.md` |
| W3 | 決策可視化鏈 | 🟡 P1 | ✅ 但需等 W4 定義好 API contract | `.omo/phase6/W3-decision-chain.md` |

## 啟動方式

### 方法 A：手動開四個 terminal

```bash
# Terminal 1
cd /Users/kaecer/workspace/atlas
git checkout main && git pull origin main
git checkout -b feat/phase6-w1-system-map
# 貼上 .omo/phase6/W1-system-map.md 的內容給 AI

# Terminal 2
cd /Users/kaecer/workspace/atlas
git checkout main && git pull origin main
git checkout -b feat/phase6-w2-simulation-health
# 貼上 .omo/phase6/W2-simulation-health.md 的內容給 AI

# Terminal 3
cd /Users/kaecer/workspace/atlas
git checkout main && git pull origin main
git checkout -b feat/phase6-w4-event-logic-lib
# 貼上 .omo/phase6/W4-event-logic-lib.md 的內容給 AI

# Terminal 4 (等 W4 的 API contract 定義好後再開)
cd /Users/kaecer/workspace/atlas
git checkout main && git pull origin main
git checkout -b feat/phase6-w3-decision-chain
# 貼上 .omo/phase6/W3-decision-chain.md 的內容給 AI
```

### 方法 B：用 GSD worktrees（推薦）

```bash
# 如果有 GSD plugin:
/gsd-new-workspace --name phase6-w1 --repos atlas-go --branch feat/phase6-w1-system-map
/gsd-new-workspace --name phase6-w2 --repos atlas-go --branch feat/phase6-w2-simulation-health
/gsd-new-workspace --name phase6-w4 --repos atlas-go --branch feat/phase6-w4-event-logic-lib
```

## 每個 workspace 的 prompt 用法

1. 開好新的 OpenCode CLI session
2. 貼上對應 `.omo/phase6/W*.md` 的**全部內容**
3. 在前面加上：
   ```
   Read .omo/phase6/Master-Plan.md first to understand the overall context,
   then execute the work described below.
   ```
4. 等 AI 產出 `/tmp/w*-report.md` 後，把報告貼回來

## 合併順序

```
W1 完成 → merge PR
W2 完成 → merge PR
W4 完成 → merge PR
W3 完成 → merge PR（可以等 W4 先合，也可以跟著 W4 branch 做）
```

合併後地圖會自動更新（W1 的 hook 機制），下一次盤點不需要人工。

## 工作量估算

| W# | 估算工作量 | 主要風險 |
|----|-----------|---------|
| W1 | 2-3h | AST 分析準確度，需要正確解析 Go import 和 HTTP route |
| W2 | 3-4h | trace log 插入點多，需要確保不改變現有行為 |
| W4 | 4-6h | 全新模組，需要好的初始設計（種子規則正確性） |
| W3 | 3-4h | 前端工作量較大，依賴 W4 的 API contract |

**建議先跑 W1+W2+W4，三個可以完全同步進行。**
