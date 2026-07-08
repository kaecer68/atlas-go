# AGENTS.md — internal/experiment

本目錄負責 **atlas-go** 的實驗生命週期管理，包含變異產生（Mutation）、執行（Execute）與評判（Judge）。

---

## 核心職責

- **Executor** (`executor.go`)：根據 `MutationBrief` 與當前 Baseline 產生候選 Prompt 或規則變異。
- **Judge** (`judge.go`)：執行 Replay 模擬，並根據成熟度（Maturity）與接受門檻評估候選者。
- **Replay Compare** (`replay_compare.go`)：計算 Baseline 與 Candidate 的性能差異。

---

## 實驗生命週期

1. **提案**：建立 `MutationBrief` JSON（目標 Agent、變異類型、接受門檻）。
2. **執行**：`Executor.Execute(briefPath)` — 由當前 Baseline 派生候選者 + Policy 檢查（Required Skills），記錄為 `domain.ExperimentPlanned` / `Running`。
3. **評判**：`Judge.Evaluate(resultPath)` — 載入 Replay、對比 Baseline 與 Candidate 的 `Observations` / `Score`，須滿足觀察筆數門檻（n≥3~12）且 Candidate 績效優於 Baseline。

---

## 評判準則 (Acceptance Gates)

- **Maturity-Aware n 門檻**：`level_1` n≥3 / `level_2_window_validated` n≥8 / `level_3_regime_aware` n≥12。
- **效能門檻**：Candidate 改善幅度 > 0.0005~0.001，禁止無效變異。

---

## 高危陷阱與反模式

- **Baseline 未載入**：執行/評判前必須確認當前 Baseline Policy 有效，否則對比將失去意義。
- **視窗稀疏 (Sparse Replay)**：若觀察筆數低於門檻，Judge 會強制拒絕。不要在資料不足時強行降低門檻。
- **Mutation 漂移**：產生的變異必須保留 `RequiredSkills` 關鍵字，否則 Policy Check 會失敗。
- **重用 Slice**：在多次模擬 run 之間絕對不可重用 `[]Recommendation`。
- **遺漏 Replay**：Judge 依賴 `ATLAS_REPLAY_DATA_PATH`，若該路徑下無對應日期之 JSONL，評判將無法進行。

## 重要 Gates 速查

**`factor_weight_stability`**：比較實驗快照的因子權重與當前 `ParametersConfig`。偏離 > `factor_weight_drift_threshold`（預設 15%）拒絕，避免把「權重漂移」誤判為「策略改進」。所有 brief 的 `acceptance_gates` 已包含。

**`preserve_downside_protection`**：使用 `candidateDD > baselineDD * ratio`（**非** `baselineDD / ratio`）。`ratio` 是「可承受最大回撤比例」非「放寬倍數」：`ratio=0.8` 表示 candidate 需 ≤ baseline 回撤的 80%。位置 `judge.go:468-475`，預設 `DrawdownProtectionRatio=0.8`。

**`fallback_window` / `UsedFallbackWindow`**：資料洩漏防護。burn-in 階段允許 fallback；進入 `calibrating` / `full_auto` 後一律拒絕。位置 `judge.go:370-377`（gate 主邏輯）+ `judge.go:398`（發 `PublishExperimentInsufficientData` 事件）。**資料完整性 gate**，不接受「稍後手動審查」替代。

**OOS Gate Ordering**（`Evaluate()` 內）：1) OOS validation 先跑填入 `result.OOSResult`；2) OOS 硬失敗直接拒絕；3) 才跑 `passesAcceptance()`。**不可重排順序** — 否則 `no_drawdown_spike` gate 看到 nil `OOSResult` 形同關閉。
