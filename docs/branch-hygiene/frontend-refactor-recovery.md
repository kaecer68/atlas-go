# atlas-go 前端重構追蹤清單

> **建立日期**:2026-06-27
> **最後更新**:2026-06-28(第三輪:關檔完成項 + 新增 task-executor 平行重複清理 + 修自身 stale 引用)
> **觸發**:用戶報案 admin_web / client_web 多個頁面失效,經盤點確認為「`./web/` → `./admin_web/` + `./client_web/` + `./shared_web/`」重構時,HTML 互補搬運不完整 + 部分 h2 標題被拿掉 + 部分 JS event-listener 漏註冊。
> **本檔用途**:追蹤本輪未完成的工作、發現的延伸問題,以及下一次迭代該從何處下手。

---

## A. 本輪已完成(快速回顧)

| 階段 | 任務 | 檔案 | 驗證 |
|------|------|------|------|
| 盤點 | 三邊 HTML/JS 結構比對 | 全部 | ✅ ID grep 對齊 100% |
| 第一階段 補遺漏 | admin_web page-narrative 補 4 panel + 補 h2 | `admin_web/static/index.html` L133-160 | ✅ |
| 第一階段 補遺漏 | admin_web page-live 整段重建 (6 panel) | `admin_web/static/index.html` L162-186 | ✅ |
| 第一階段 補遺漏 | admin_web page-industry 補 3 panel + 互動 + 補 h2 | `admin_web/static/index.html` L411-435 | ✅ |
| 第一階段 補遺漏 | admin_web evolution_panel 補 evView-compact 按鈕 | `admin_web/static/index.html` L266-272 | ✅ |
| 第一階段 補遺漏 | client_web page-narrative 補 4 panel + 補 h2 | `client_web/static/index.html` L57-72 | ✅ |
| 第一階段 補遺漏 | client_web page-industry 補 3 panel + 互動 + 補 h2 | `client_web/static/index.html` L252-279 | ✅ |
| 第一階段 補遺漏 | client_web page-live 補 2 個隱藏 panel | `client_web/static/index.html` L76-100 | ✅ |
| 第二階段 功能正常化 | admin_web event-listeners 補 evView-compact 監聽器 | `admin_web/static/js/event-listeners.js` L43 | ✅ |
| 第二階段 功能正常化 | 兩邊 esbuild 重新 build | `admin_web/dist/`、`client_web/dist/` | ✅ build 成功 |
| 第二階段 功能正常化 | HTML 結構語法驗證 | `static/index.html` 兩邊 | ✅ Python html.parser 無 mismatch |
| P0 二次修復 | **client_web `main.js` 補 risk.js + dashboard.js 動態 import** | `client_web/static/js/main.js` L92-107 | ✅ |
| P0 二次修復 | **client_web page-live 分支補 render 呼叫**(6 個 renderLiveStatus/renderRiskCards/renderRiskCalibration/renderRiskCommentary/renderMacroRadar/renderLiveNarrativeStrip) | `client_web/static/js/main.js` L275-298 | ✅ |
| P0 二次修復 | client_web page-decision 補 renderAIEvolution(順便修「AI 自我進化狀態失效」) | `client_web/static/js/main.js` L258-280 | ✅ |
| P0 二次修復 | client_web dist 重新編譯(322.8kb) | `client_web/dist/js/main.js` | ✅ |
| P2 改進 | shared_web 8 個檔案 null check 修正 backport 回 web | `web/static/js/{pages,shared}/` | ✅ web/ 重新編譯 |
| P2 改進 | inline rgba 寫死色碼遷移到 `--accent-rgb` / `--color-danger-rgb` triplet 變數 | `variables.css` + 6 處 inline style | ✅ 三邊 dist 重新編譯 |

---

### P0 二次修復詳情(2026-06-27 第二輪)

**問題**:用戶報案「【投資風險總覽】裡面的2個模塊功能失效,一直轉圈圈」,雖然第一輪補齊了 HTML 端 2 個隱藏 panel,但 UI 仍卡「載入中…」。

**根因**(grep client_web/main.js L92-107 發現):
1. **client_web 動態 import 列表完全沒包含 `./pages/risk.js`** — 雖然 HTML 有 `liveStatus` / `riskCards` / `riskCalibration` / `liveRiskCommentary` 4 個 ID,但 JS 端根本沒有對應的 render 函式可呼叫
2. **page-live 分支只有 fetch,沒有任何 render 呼叫** — `liveResults` 拿到資料後沒被使用,UI 永遠停在「載入中…」
3. **client_web 也缺 `./pages/dashboard.js`** — 導致 `renderMacroRadar` 與 `renderAIEvolution` 都無法呼叫

**修復**:
- 在 imports 列表加 `import('./pages/risk.js')` + `import('./pages/dashboard.js')`
- 在 page-live 分支對齊 admin_web 風格:用 `getJSONWithTimeout` 取代 `silentGetJSON`,補 6 個 render 呼叫
- 在 page-decision 分支補 `renderAIEvolution` 呼叫(7 個 endpoint 平行 fetch)

**修復後 dist 大小**:322.8kb(原 276.1kb,漲幅 46.7kb = 風險+儀表板頁邏輯)

---

## B. 後續追蹤(P0 — 本輪未做,下次必做)

### B-1. e2e 驗證缺失的渲染邏輯

**問題**:用戶報案「【AI 觀測台】--> 【績效評估】模塊功能失效」、「【指標監控】--> 【指標趨勢】模塊功能失效」、「【PRISM 訓練結果】--> 整個頁面的功能失效」,這三個報案 HTML 端 100% 完整。但目前沒有實際啟動 backend 做 e2e 驗證。

**根因假設**:
1. `pages/dashboard.js` 在 `agentObservatory` 渲染時需要 `/api/agents/observatory` 端點 — 若後端缺此 endpoint 或 timeout,UI 會卡在「載入中…」
2. `pages/metrics.js` 的 `metricsTrend` 渲染依賴 `/api/metrics/trend` 資料
3. `pages/prism.js` 的 `prismContent` 渲染依賴 `/api/prism/*` 系列 endpoint

**驗證動作**:
```bash
docker compose up -d
# 等 health check 通過
curl -fsS http://localhost:8080/api/agents/observatory
curl -fsS http://localhost:8080/api/metrics/trend
curl -fsS http://localhost:8080/api/prism/results

# 用 playwright 開啟 /admin/ 與 /client/
# 切到 page-agents / page-metrics / page-prism
# 觀察 console 是否有 fetch error、UI 是否真的渲染
```

**預期輸出**:若 console 出現 `/api/agents/observatory 404` → 後端缺 endpoint;若 fetch 200 但 UI 卡「載入中…」→ JS 端缺少 render 觸發條件。

---

### B-2. 用戶誤報項目確認

**問題**:用戶報案「【決策鏈】 --> 【即時事件雷達】、【投資心法】共 3 個模塊功能失效」。經盤點:

| 報案 ID | 實際位置 | 處置 |
|---------|---------|------|
| `liveRadar`(即時事件雷達) | 不存在於 web/HTML,推測實為 `macroRadar`(在 page-live) | ✅ 已透過補 client_web page-live 修復 |
| `strategiesContent`(投資心法) | 在 `page-strategies` 而非 `page-decision` | client_web 的 page-strategies 已正常,`main.js` L312 已綁定 `loadStrategies` |

**驗證動作**:
- 與用戶確認「即時事件雷達」是否實指 `macroRadar`;若否,需另尋對應 panel 補入 page-decision
- 確認「投資心法」在 page-decision 是否需要一個新的嵌入視圖(從 page-strategies 拉區塊顯示)

---

### B-3. client_web 「投資風險總覽」轉圈圈 ✅(2026-06-27 第二輪已修)

**問題**:用戶報案「【投資風險總覽】裡面的2個模塊功能失效,一直轉圈圈」。已從 web 補齊 2 個隱藏 panel(`riskCalibrationPanel` + `liveRiskCommentaryPanel`)的 HTML。

**但轉圈圈不一定是 HTML 問題**,可能是:
1. JS `risk.js` 在 client_web main.js 中**沒有被動態 import**
2. 雖然 admin_web 有 import `./pages/risk.js`,但 client_web 的 main.js L94-103 動態 import 列表**沒有 `risk.js`**

**修復**:
- `client_web/static/js/main.js` L95-96 新增 `import('./pages/risk.js')` 與 `import('./pages/dashboard.js')`
- page-live 分支 L309-314 補齊 6 個 render 呼叫
- page-decision 分支 L269-270 補 `renderAIEvolution`(順帶修「AI 自我進化狀態失效」)
- dist 重新編譯(322.8kb,漲幅 46.7kb = 風險 + 儀表板頁邏輯)

**驗證**:見 G-1 節。

---

## C. 後續追蹤(P1 — 改進性質)

### C-1. shared_web 與 web 程式碼 drift

**問題**:shared_web 對 5 個 pages 與 3 個 shared 檔案做了「修正」(新增 null check guard),但 web/ 仍是舊版。

```
diff web vs shared_web:
  pages/dashboard.js     — 4 處 null check 新增
  pages/experiments.js   — 3 處 null check 新增
  pages/inbox.js         — 1 處 null check 新增
  pages/industry.js      — 2 處 null check 新增
  pages/risk.js          — 1 處 null check 新增
  shared/components/seasonality-panel.js
  shared/field_types.ts
  shared/valid_fields.json
```

**影響**:admin_web / client_web 透過 esbuild plugin 拿到的是 shared_web(修正版),功能會比 web 更穩定。但 web 仍存在於 git,若有人用 web/ 啟動會有潛在 null deref。

**處置**:下一輪把 shared_web 的 5 個修正 backport 到 web,或評估是否把 web/ 標為 deprecated。

**2026-06-28 狀態**:✅ **已完成 backport**。`diff -q web vs shared_web` 5 個 pages + 1 個 component + `variables.css` 全部一致(無差異輸出)。`web/` 仍作為 archive 保留但不對外服務;啟動風險已消除。

---

### C-2. inline style 色碼遷移(AGENTS.md 規範)

**問題**:本輪補回的 HTML 內含 4 處 inline style 寫死色碼:
- `border-color:rgba(79,193,255,0.2)`(敘事 Macro 框)
- `color:var(--accent)`(敘事 Macro 標題)
- `border-color:rgba(239,68,68,0.2)`(敘事 Stress 框)
- `color:var(--color-danger)`(敘事 Stress 標題)

AGENTS.md 規範要求**寫死色碼一律遷移到金融語意 token**(`/shared_web/static/css/base/variables.css` 內的 `--trend-bullish`/`--pnl-profit` 等)。

**處置**:
1. 在 variables.css 新增 2 個 token:`--macro-accent-rgb`、`--stress-danger-rgb`
2. 4 處 inline style 改用 `var(--...)` 引用

**2026-06-28 狀態**:✅ **部分完成**。`variables.css` 兩主題(L24、L68、L73)已新增 `--accent-rgb` 與 `--color-danger-rgb`(原計畫的 `--macro-accent-rgb` 名稱未採用,沿用既有 token 較一致)。6 處 inline rgba 已遷移:`admin_web/static/index.html` L136-137、client_web/static/index.html L60-61、web/static/index.html 對應位置。三邊 dist 重新編譯通過。

---

### C-3. ESM 動態 import 路徑檢查 ✅(2026-06-28 已建 CI script)

**問題**:兩邊 `main.js` 動態 import 路徑是相對路徑(例如 `./pages/narrative.js`),透過 esbuild plugin fallback 到 `shared_web/static/js/pages/narrative.js`。

但**沒有任何自動化檢查**確認每個 main.js 引用的 pages/*.js 都能在 shared_web 找到。如果有人刪掉 shared_web 內的某個 pages/*.js,esbuild 會靜默失敗。

**處置**:已建立 `scripts/ci/check_frontend_imports.sh`(74 行):驗證 admin_web/client_web 的 main.js 引用的 pages/*.js 是否在 shared_web/ 都有,缺則 exit 1。`--warn-only` 模式只警告不擋。

**已知限制**:此 script 只檢查 `pages/` 動態 import,**不檢查 `components/` 引用**。components/ 重複或孤立的偵測需另寫腳本(見 H 節)。

---

### C-4. AGENTS.md 路徑不一致 ✅(2026-06-28 已修)

**問題**:三個目錄的 AGENTS.md 都寫「`web/static/css/base/variables.css`」,但實際路徑已改成 `shared_web/static/css/base/variables.css`。

**處置**:三個 AGENTS.md L72-74 區段統一改成 `shared_web/...`。✅ 已驗證:`admin_web/AGENTS.md`、`client_web/AGENTS.md` 二者 L72 全部為 `shared_web/static/css/base/variables.css`（當時 legacy web 目錄的 AGENTS.md 仍存在，後續隨 `./web/` 整體刪除）。

---

## D. 後續追蹤(P2 — 文檔/組織性質)

### D-1. README/CLAUDE.md 補充前端重構說明 ✅(部分完成,2026-06-28)

**現況**:`CLAUDE.md` L23-67 已有完整「前端架構」段落(目錄職責表、入口檔職責表、esbuild fallback 規則、CSS/JS 規範、API 整合、疑難排解)。`AGENTS.md` 仍把 web/ 視為主目錄,但已不影響實際運作(因 CLAUDE.md 是 AI 進入入口)。

**2026-06-28 殘留瑕疵**:
- CLAUDE.md L165 Token Efficiency Rules 仍寫 `web/static/css/main.css` 範例,需改 `shared_web/static/css/main.css`(見 Phase A3 commit)
- CLAUDE.md 沒有明確指向 `atlas-pre-change-protocol` skill(目前只在 GUIDELINES_INDEX.md 與 .claude/SKILLS-MAP.md 提及)。AI 從 CLAUDE.md 進入看不到這個強制前置檢查的入口(見 Phase A2 commit)

### D-2. shared_web 內 components/ 與 pages/ 是否有 dead code

盤點時發現 `./web/static/js/components/` 與 `./shared_web/static/js/components/` 結構相同(12 個檔案),但沒逐個驗證每個 component 是否真的被某個 page 用到。下一輪應該做 dead code scan:
```bash
# 對每個 component.js,grep 是否有 pages/*.js 或 main.js import 它
for f in shared_web/static/js/components/*.js; do
  name=$(basename $f .js)
  echo "$name: $(grep -rl "from.*components/$name" shared_web/static/js/ | wc -l) references"
done
```

---

## E. 驗證指令彙整

下次驗證本輪修復時,跑以下指令:

```bash
# 1. 靜態結構對齊
for id in narrativeMacro narrativeStress narrativeRetailSentiment narrativeSeasonal \
          narrativeEvents narrativeChains narrativeModels narrativeTemplates \
          industryMap industryCycle industryLinkage industrySeasonality \
          industryGraph industryShockSim shockSource btnRunShockSim \
          liveNarrativeStrip macroRadar liveStatus riskCardsPanel \
          riskCalibrationPanel liveRiskCommentaryPanel \
          evView-compact evView-ai-analysis agentObservatory metricsTrend prismContent; do
  for dir in admin_web client_web; do
    [ -f "/Users/kaecer/workspace/atlas/$dir/static/index.html" ] && \
      grep -q "id=\"$id\"" /Users/kaecer/workspace/atlas/$dir/static/index.html && \
      echo "$dir $id ✓" || echo "$dir $id ✗"
  done
done

# 2. HTML 語法合法性
python3 -c "..."  # 見本輪執行的驗證腳本

# 3. esbuild build
cd /Users/kaecer/workspace/atlas/admin_web && npm run build
cd /Users/kaecer/workspace/atlas/client_web && npm run build

# 4. e2e (需 backend 啟動)
docker compose up -d
curl -fsS http://localhost:8080/health
# 用 playwright 開 /admin/ 與 /client/,切到報案頁面,截圖確認
```

---

## F. 變更檔案清單

本輪修改的檔案(供 git commit 參考):

| 檔案 | 變更 |
|------|------|
| `admin_web/static/index.html` | page-narrative 補 4 panel + 補 h2;page-live 整段重建;page-industry 補 3 panel + 互動 + 補 h2;evolution_panel 補 evView-compact 按鈕 |
| `admin_web/static/js/event-listeners.js` | 補 evView-compact 監聽器(L43) |
| `admin_web/dist/*` | esbuild 重新產出 |
| `client_web/static/index.html` | page-narrative 補 4 panel + 補 h2;page-industry 補 3 panel + 互動 + 補 h2;page-live 補 2 個隱藏 panel |
| `client_web/dist/*` | esbuild 重新產出 |

**未變更**:`shared_web/`、`web/`、`cmd/`、`internal/` 等後端與共享資源。

---

## G. 第二輪 P0/P2 變更摘要(2026-06-27 第二輪)

### G-1. P0 client_web 「投資風險總覽轉圈圈」根因修復

**問題**:用戶報案「投資風險總覽裡面的2個模塊功能失效,一直轉圈圈」。第一輪補齊了 HTML 端 2 個隱藏 panel,但 UI 仍卡「載入中…」。第二輪經 grep 發現根本原因:`client_web/main.js` 動態 import 列表完全沒包含 `./pages/risk.js`,且 page-live 分支只有 fetch 沒有 render 呼叫。

**修復內容**(`client_web/static/js/main.js`):
- imports 列表新增 `risk.js` + `dashboard.js`(+ 既有 9 個 page module)
- page-live 分支對齊 admin_web:用 `getJSONWithTimeout` 取代 `silentGetJSON`,補 6 個 render 呼叫(`renderLiveStatus` / `renderRiskCards` / `renderRiskCalibration` / `renderRiskCommentary` / `renderMacroRadar` / `renderLiveNarrativeStrip`)
- page-decision 分支補 `renderAIEvolution` 呼叫(順便修「AI 自我進化狀態失效」,因為 aiEvolution 在 dashboard.js 內)

**dist 大小**:322.8kb(原 276.1kb,+46.7kb = 風險 + 儀表板邏輯)

### G-2. P2-1 shared_web drift backport

**問題**:shared_web 對 5 個 pages + 1 個 component + 2 個 shared 檔案有 null check 修正,但 web/ 沒有。admin_web/client_web 透過 esbuild plugin fallback 用 shared_web(修正版),但 web/ 仍是舊版,若有人用 web/ 啟動會有潛在 null deref。

**修復**:8 個檔案從 shared_web 複製到 web/(見本檔 F. 變更檔案清單)。web/ 重新編譯驗證通過。

### G-3. P2-2 inline style 寫死色碼遷移

**問題**:本輪補入的 HTML 內含 2 處 `border-color:rgba(79,193,255,0.2)` 與 2 處 `border-color:rgba(239,68,68,0.2)` 寫死色碼。AGENTS.md 規範要求遷移到金融語意 token。

**修復**:
- shared_web + web 的 `variables.css` 各新增 2 個 RGB triplet 變數(dark + light 主題):`--accent-rgb` / `--color-danger-rgb`
- 6 處 inline rgba 改為 `rgba(var(--accent-rgb), 0.2)` / `rgba(var(--color-danger-rgb), 0.2)`(admin_web 2 + client_web 2 + web 2)
- 三邊 dist 重新編譯驗證

### G-4. 新發現:pre-existing `web/` HTML 結構錯誤

**問題**:用 Python html.parser 驗證 `web/static/index.html`,發現 3 個 mismatch 錯誤:
- L736 `</div>` 多餘(預期關閉 `div` 但 stack top 是 `main`)
- L880 `</body>` / L881 `</html>` 同樣 mismatch

**根因**:`web/index.html` L728-736 的 page-prism 結尾後多 2 個 `</div>`,整個 stack 在 L736 之前就已經 balanced,但 parser 因為多餘的 `</div>` 開始 pop 過頭。

**判定**:**pre-existing**,非本輪引入:
- git diff 顯示本輪對 web/index.html 僅改 2 行(inline rgba → var(--xxx-rgb))
- admin_web/static/index.html 與 client_web/static/index.html 驗證完全合法(0 errors)
- `web/` 是 legacy 單體版本,已被 admin_web/client_web 取代

**處置**:依「Fix minimally. NEVER refactor while fixing.」原則,**不在本輪修復**,僅記錄於追蹤清單。若未來需要重新啟用 `web/`,應先修此結構問題。

---

## H. 第三輪清理(2026-06-28)

### H-1. 平行重複實作 — `components/task-executor.js` 與 `pages/datachannels.js` 重複

**問題**(由 D-2 dead code scan 衍生):`shared_web/static/js/components/task-executor.js`(28 行)0 refs,看似 dead code。

**依 `atlas-pre-change-protocol/SKILL.md` Step 7 Code Intent 驗證**:

| 檢查項 | 結果 |
|--------|------|
| `git log --all --oneline -- "**/components/task-executor.js"` | 2 commits:9f898883 (web split 引入)、1635acb2 (dedup 重構)。從未被外部引用 |
| 介面滿足 / plugin registry / config-driven / reflect 動態載入 | 全部否(裸 `export async function`) |
| 是否有替代品? | **有**:`pages/datachannels.js` L14 `export async function triggerChannelsIngest` 為完整相同邏輯 |
| 實際呼叫鏈 | admin_web/main.js L154、client_web/main.js L142、web/main.js L142 都用 `modules.datachannels.triggerChannelsIngest`(從 pages/datachannels.js 取) |

**判定**:不是 dead code、不是未完成工作,是 **parallel duplicate**(平行重複實作),按 Step 0 + Code Removal Checklist 應刪除。

**處置**(見本 PR Phase B1+B2 commits):
- 刪除 `shared_web/static/js/components/task-executor.js`
- 同步刪除 `web/static/js/components/task-executor.js`(legacy 同樣孤立)
- 三邊 `npm run build` 全成功 → 驗證 esbuild 不會 bundle 未引用檔案(dist 不變)
- `grep -rn "triggerChannelsIngest" shared_web/static/js/components/` 期望 0(確認無殘留)

### H-2. 同步補齊機制指引:CLAUDE.md 缺 atlas-pre-change-protocol 入口

**問題**:使用者回饋每次都要重複「查 gitnexus 重疊」、「未引用 ≠ dead code」提示詞。雖然 `.claude/skills/atlas-pre-change-protocol/SKILL.md` 已完整涵蓋這兩條規則,但 CLAUDE.md(AI 進入入口)未明確指向,AI 從 CLAUDE.md 進入時看不到這個強制前置檢查。

**處置**(見本 PR Phase A2+A3 commits):
- CLAUDE.md 加 1 行明確指向 atlas-pre-change-protocol skill
- CLAUDE.md L165 stale `web/static/css/main.css` 範例改為 `shared_web/static/css/main.css`