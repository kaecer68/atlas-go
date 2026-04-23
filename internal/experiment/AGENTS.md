# AGENTS.md — internal/experiment

本目錄負責 **atlas-go** 的實驗生命週期管理，包含變異產生（Mutation）、執行（Execute）與評判（Judge）。

---

## 核心職責

- **Executor** (`executor.go`)：根據 `MutationBrief` 與當前 Baseline 產生候選 Prompt 或規則變異。
- **Judge** (`judge.go`)：執行 Replay 模擬，並根據成熟度（Maturity）與接受門檻評估候選者。
- **Replay Compare** (`replay_compare.go`)：計算 Baseline 與 Candidate 的性能差異。

---

## 實驗生命週期

1. **提案 (Propose)**：建立 `MutationBrief` JSON，定義目標 Agent、變異類型與接受門檻。
2. **變異與執行 (Execute)**：
   - 呼叫 `Executor.Execute(briefPath)`。
   - 系統自動由當前 Baseline 派生候選者，並進行初步 Policy 檢查（如 Required Skills）。
   - 產生 `PromptExperimentResult` 並記錄為 `domain.ExperimentPlanned` 或 `domain.ExperimentRunning`。
3. **評判 (Judge)**：
   - 呼叫 `Judge.Evaluate(resultPath)`。
   - 載入 Replay 資料，對比 Baseline 與 Candidate 的 `Observations` 與 `Score`。
   - **接受門檻**：必須滿足觀察筆數門檻（n≥3~12）且 Candidate 績效優於 Baseline。

---

## 評判準則 (Acceptance Gates)

- **Maturity-Aware**：
  - `level_1`：n≥3，主要用於快速驗證。
  - `level_2_window_validated`：n≥8，需要多視窗驗證。
  - `level_3_regime_aware`：n≥12，高信度證據。
- **效能門檻**：Candidate 必須有實質提升（改善幅度需 > 0.0005~0.001），禁止無效變異。

---

## 高危陷阱與反模式

- **Baseline 未載入**：執行/評判前必須確認當前 Baseline Policy 有效，否則對比將失去意義。
- **視窗稀疏 (Sparse Replay)**：若觀察筆數低於門檻，Judge 會強制拒絕。不要在資料不足時強行降低門檻。
- **Mutation 漂移**：產生的變異必須保留 `RequiredSkills` 關鍵字，否則 Policy Check 會失敗。
- **重用 Slice**：在多次模擬 run 之間絕對不可重用 `[]Recommendation`。
- **遺漏 Replay**：Judge 依賴 `ATLAS_REPLAY_DATA_PATH`，若該路徑下無對應日期之 JSONL，評判將無法進行。
