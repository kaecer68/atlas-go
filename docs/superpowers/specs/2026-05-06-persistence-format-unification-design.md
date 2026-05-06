# 持久化格式統一設計文件

## 目標

在系統正式上線前，徹底消除 Atlas-Go 持久化 JSON / JSONL 的新舊格式混雜問題，將**正式持久化契約**統一為 **snake_case**，同時保護本系統作為**受治理約束的自我改進投資研究與模擬交易系統**所需的證據鏈、實驗可追溯性與人工審批紀錄。

本設計不再把問題視為單純欄位命名整理，而是視為：

> **正式上線前的資料契約清場 + 演化證據鏈收斂工程。**

---

## 1. 系統核心目的與設計前提

### 1.1 系統核心目的

根據 `docs/evolution-loop.md`、`docs/ai-agent-architecture.md`、`scripts/openclaw/` 與 `internal/experiment/`、`internal/baseline/`、`internal/portfolio/` 的實作，Atlas-Go 的核心目的不是單純回測，而是：

1. 在 replay / paper / guarded-live 邊界內運行多層投資研究代理
2. 將代理建議轉成可模擬、可評分、可稽核的交易決策
3. 識別弱代理、生成 mutation brief、執行實驗、進行 judge
4. 透過 baseline promotion / revert 與 Darwinian 權重回饋，形成**受控制的自我改進閉環**

### 1.2 現況判斷

這套系統**已有自我進化雛形，但不是完全無人化閉環**：

- `propose → execute → judge` 已高度自動化
- `promote / revert` 仍保留人工 gate（`scripts/openclaw/decide.sh` 要求 `--reason`）
- Darwinian weights 持續更新，但與部分高階 regime / mutation feedback 尚未完全接通

因此，持久化格式統一不能破壞下列核心證據鏈：

```text
MutationBrief
→ PromptExperimentResult / ExperimentRecord
→ baseline_policy.json
→ approvals / governance events
→ session summaries / recommendation outcomes
→ monitoring / audit / Darwinian feedback
```

---

## 2. 問題背景

目前已確認存在至少三類格式漂移：

### 2.1 Session 類資料

- 舊 `summary.json` 曾出現 PascalCase（歷史資料）
- `recommendation_outcomes.jsonl` 存在 PascalCase 主欄位與 snake_case 後加欄位混用

### 2.2 Experiment / Governance 類資料

- `ExperimentRecord` 目前 **所有欄位都沒有 json tags**
- `PromptExperimentResult` 為 **PascalCase 外層欄位 + snake_case 內部欄位** 的混合結構
- `baseline.Policy`、`PromotionRecord`、`RevertRecord` 也尚未統一為 snake_case

更精確地說：

- `ExperimentRecord` 整個結構目前會以 PascalCase 落地
- `PromptExperimentResult` 的外層欄位（如 `Experiment`、`Brief`、`CandidatePrompt`）目前是 PascalCase
- 但其內部如 `MutationBrief`、`ReplayDataMetadata`、`OOSResult` 等結構又部分已是 snake_case

這代表後續 conversion 不能做 naïve key-level 文字替換，必須先 canonicalize struct contract。

### 2.3 根本原因

根因不是單一 bug，而是：

1. 早期 domain / baseline 結構未完整加上 `json` tags，Go 預設輸出 PascalCase
2. 後期逐步補 tag，但只補了一部分型別或部分欄位
3. 某些讀取端曾用 inline struct 或字串條件判斷補洞，沒有完全依賴唯一 domain 契約
4. 沒有做 schema versioning / cleanup / migration discipline

這代表磁碟資料不是只有「新舊格式不同」，而是已出現**資料契約邊界不清楚**的架構問題。

---

## 3. 正式決策

### 3.1 Canonical Format

所有正式持久化 JSON / JSONL 一律統一為：

- **snake_case keys**

禁止：

- `PascalCase`
- mixed-case（同一型別不同欄位混搭）
- 同一型別在不同 writer 使用不同欄位 naming

### 3.2 單一真理來源

正式 persistence contract 的唯一來源為：

- `internal/domain/*`
- `internal/baseline/*` 中實際落地保存的 struct `json` tags

任何 writer / reader：

- 不得自行發明另一套持久化欄位名稱
- 不得長期依賴 inline struct 定義另一份 schema
- 不得以原始 JSON 字串關鍵字比對作為正式資料判斷邏輯

### 3.3 策略選擇

採用**修正版方案 C**：

> **正式資料以重建為主；不可重建但必須保留的審計資料採一次性 canonical conversion；舊資料先完整封存。**

這不是「所有資料都重建」，而是：

- 可重建的正式衍生資料 → 重建
- 不可重建但屬於審計真相的資料 → 保留並轉換
- 所有 writer 先統一為 canonical snake_case

---

## 4. 資料分類與處理策略

### 類型 A：可重建的正式衍生資料（Rebuild-First）

這類資料的價值在於它們是系統執行結果的派生表示，只要重建條件成立，就應以**重建**取代就地遷移。

至少包含：

- `data/state/sessions/*/summary.json`
- `data/state/sessions/*/recommendation_outcomes.jsonl`
- `data/state/recommendation_outcomes.jsonl`

處理策略：

- archive 舊資料
- 確保 writer 契約已 canonical 化
- 以當前正式流程重新生成

### 類型 B：實驗與治理證據資料（Preserve-and-Convert）

這類資料是系統自我改進閉環中的**審計證據**，不可單純丟棄，也不應假設能以重跑完全等價重建。

至少包含：

- `data/state/experiments.jsonl`
- `data/state/experiments/*.json`
- `data/state/baseline_policy.json`

處理策略：

- 先補齊 canonical json tags
- 以一次性 conversion tool 轉為 snake_case
- 保留原始 archive 作為原始歷史證據

### 類型 C：人工與不可重建事件資料（治理處理原則）

這不是一組獨立檔案類型，而是對「不可重建的治理事件資料」的處理原則：

- 不依賴重建
- 保留原始 archive
- 若正式系統仍需依賴，再產出 canonical copy

適用對象通常包括 approve / reject / promote / revert 等人工治理事件。

### 類型 D：已經 canonical 的資料（Verify + Archive Only）

這類資料已符合 snake_case，不需要 conversion，只需要納入 inventory、驗證與封存管理。

目前已知至少包含：

- `data/state/approvals/*.json`
- `data/state/human_interventions.jsonl`

處理策略：

- 不重建
- 不轉換
- 只做格式驗證與封存管理

---

## 5. 先決條件：Writer Canonicalization

在任何資料重建或轉換前，必須先完成所有正式 writer 的 canonical 化。

### 5.1 必須優先修正的型別

至少包括：

- `domain.ExperimentRecord`
- `domain.PromptExperimentResult`
- `baseline.Policy`
- `baseline.PromotionRecord`
- `baseline.RevertRecord`

原因：

- 如果 writer 還會寫出 PascalCase，任何重建或 conversion 都只是在複製舊問題。

### 5.2 必須同步處理的非 domain 依賴 reader

在 canonicalize experiment / baseline 類資料時，必須同步修正仍依賴 PascalCase key 的腳本與 reader。

已知阻塞項至少包含：

- `scripts/openclaw/decide.sh`

目前此腳本直接搜尋 PascalCase keys（如 `"Status"`、`"BaselineValue"`、`"CandidateValue"`、`"TargetAgentID"`、`"Version"`）做安全檢查與決策摘要。若先把 experiment / baseline artifacts 轉成 snake_case 而不更新這支腳本，治理流程會直接失效。

### 5.3 Writer 規則

所有寫入正式 state 的程式路徑必須滿足：

1. 只輸出 snake_case
2. 只依賴 canonical struct tags
3. 不保留 mixed-case 行為作為相容機制

---

## 6. 重建可行性驗證

在執行大規模重建前，必須先驗證「可重建性」不是假設，而是事實。

### 6.1 驗證目標

挑選 2–3 個代表性 session，驗證重跑後的關鍵欄位是否在語義上可接受。

至少比對：

- `OutcomeCount`
- `PortfolioValue`
- `EndingCash`
- `GuardOutcomes`
- 主要 recommendation outcome 筆數

### 6.2 可接受差異

應允許的差異：

- `RecordedAt` 改變
- 非核心排序差異

必須調查的差異：

- outcome 數量明顯不同
- portfolio value / ending cash 偏差異常
- guard outcomes 類型或 passed 結果不一致

### 6.3 若不可重建

若某類資料無法穩定重建為語義一致結果，則改採：

- preserve-and-convert

而非強行重建。

### 6.4 Recommendation outcomes writer 一致性驗證

在進入全量重建前，必須額外確認：

- session-level `recommendation_outcomes.jsonl`
- root-level `data/state/recommendation_outcomes.jsonl`

是否由不同 writer 路徑產生，以及這些路徑是否都已收斂到同一個 canonical `RecommendationOutcome` contract。

若存在多個 writer 路徑，必須在 Phase 3 一併 canonicalize，而不能只修其中一條。

---

## 7. 執行流程

### Phase 1：Inventory

輸出完整 inventory：

- 哪些檔案是 PascalCase
- 哪些是 snake_case
- 哪些是 mixed-case
- 哪些已 canonical、只需驗證
- 檔案屬於 A / B / C / D 哪一類
- 對應 writer / reader / domain owner 是誰
- 是否存在多個 writer 寫入同一種 artifact（特別是 `recommendation_outcomes.jsonl`）

### Phase 2：封存舊資料

完整封存至：

- `data/state-archive/<timestamp>/...`

規則：

- 未封存前不得覆寫正式資料
- archive 必須可原樣回復

### Phase 3：Canonicalize Writers

修正所有正式 writer：

- 只輸出 snake_case
- 補齊缺失 json tags
- 移除造成 mixed-case 的結構定義

### Phase 4：Pilot Rebuild / Pilot Conversion

先對小範圍 representative data 執行：

- 類型 A：pilot rebuild
- 類型 B/C：pilot convert

驗證結果符合預期後，才進入全量處理。

### Phase 5：Full Rebuild / Full Conversion

- 類型 A：全量重建
- 類型 B/C：全量 conversion + canonical rewrite

### Phase 6：驗證與清場

驗證項目：

1. 正式 `data/state/**` 不再出現 PascalCase keys
2. dashboard / monitoring / report / experiment flows 正常讀取
3. 重要治理流程可正常使用：promote / revert / approvals / judge
4. 不再依賴 mixed-case inline reader 或字串 hack 才能運作
5. `scripts/openclaw/decide.sh` 不再依賴 PascalCase key 比對

---

## 8. Rollback / 回退策略

### 8.1 回退原則

回退不是把單一檔案修回，而是：

1. 清除新產生的 canonical state
2. 將 `data/state-archive/<timestamp>` 還原為正式 `data/state`

### 8.2 為什麼

因為這次處理的不是小型 migration，而是整體 persistence contract 收斂。最穩定的 rollback 是整批切回 archive。

### 8.3 要求

- archive 必須完整
- rollback 必須可腳本化
- rollback 後系統要能重新啟動並讀取舊 state

---

## 9. Legacy Compatibility Policy

### 短期

目前已存在的 compatibility reader（例如 `RecommendationOutcome.UnmarshalJSON`）可以暫時保留，作為過渡保險。

### 注意

目前 **沒有** `SessionSummary.UnmarshalJSON`，因此 spec 與實作不得假設它存在。

### 長期

當 canonical state 全面驗證完成後：

- 將 legacy compatibility 標記為 temporary
- 再安排下一階段移除

目標是讓正式系統最終只理解 canonical snake_case。

---

## 10. 與系統升級方向的關係

這份 spec 不是獨立於產品目的的清理工作。它服務的，是下列更高層升級目標：

1. 讓自我改進閉環的 artifact schema 可預測、可測試、可治理
2. 讓未來的 walk-forward validation、regime-aware feedback、Darwinian / Janus 閉環整合，有穩定持久化契約可依賴
3. 讓 baseline promotion / revert / approvals / experiments 成為真正可審計的證據鏈，而不是混雜格式的歷史遺跡

因此本次統一格式工作，不是單純清理技術債，而是：

> **為「可治理的自我進化投資研究系統」建立可信的持久化底座。**

---

## 11. 驗證標準

完成後必須滿足：

1. 正式 `data/state/**` 中，不再存在 PascalCase / mixed-case 正式資料
2. `summary.json`、`recommendation_outcomes.jsonl`、`experiments.jsonl`、`experiments/*.json`、`baseline_policy.json` 都符合 canonical contract
3. OpenClaw 操作流程與 experiment / baseline 生命周期可正常工作
4. dashboard / monitoring 關鍵頁面可正常載入
5. legacy compatibility 不再是正式系統運作的必要條件

---

## 最終結論

Atlas-Go 的目標不是單次回測，而是建立一套**可治理、可審計、可逐步自我改進的投資研究與模擬交易系統**。

因此持久化格式統一的正確方案不是「只修兩個檔案」，也不是「粗暴全部重跑」，而是：

- **正式契約唯一化：snake_case**
- **先修 writer，再處理資料**
- **可重建資料重建**
- **不可重建但重要的證據資料轉換保留**
- **原始歷史完整封存**
- **legacy 相容層最終移除**

這是符合系統核心目的的修正版方案 C。
