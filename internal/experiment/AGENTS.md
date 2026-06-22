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

## 因子權重穩定性閘門 (factor_weight_stability Gate)

`factor_weight_stability` 閘門會在實驗評判時比較實驗快照中的因子權重與當前運行權重。若偏離超過 `factor_weight_drift_threshold`（預設 15%），實驗將被拒絕 — 因為績效差異可能來自權重漂移而非策略改進。

此閘門依賴實驗執行時產生的 `ParameterSnapshotID`。`computeWeightDrift()` 比較快照中的 `FactorWeight.BaseWeights` 與當前 `ParametersConfig`。

**所有實驗 brief 的 `acceptance_gates` 均已包含此閘門。**

## preserve_downside_protection Gate — Sign Convention

`preserve_downside_protection` 閘門使用 `candidateDD > baselineDD * ratio` 而非 `baselineDD / ratio`。
這是刻意設計的語義：`ratio` 代表「可承受的最大回撤比例」，而非「放寬倍數」。

- `ratio = 0.8` → candidateDD 必須 ≤ baseline 回撤的 80%（比 baseline 少 20%）
- `ratio = 1.2` → candidateDD 可以到 baseline 回撤的 120%（比 baseline 多 20%，較寬鬆）
- 若用 `baselineDD / ratio` 會反轉語義：`/ 0.8 = 1.25×`，ratio 越小反而越寬鬆（違反直覺）

實作位置: `internal/experiment/judge.go:405-411`
預設值: `DrawdownProtectionRatio = 0.8`（見 `defaultGARCHParameters()` in `internal/config/parameters_defaults.go`）

## Fallback Window Gate (UsedFallbackWindow)

當實驗使用了 fallback backtest window（`result.UsedFallbackWindow == true`）時，會有資料洩漏風險：fallback 視窗可能與 OOS 視窗重疊，使得驗證結果被訓練資料污染。

- **Burn-in 期間允許**：burn-in 階段系統尚無自有 replay 資料，必須使用 fallback。
- **Burn-in 之後拒絕**：一旦系統進入 calibrating 或 full_auto 成熟度，fallback window 實驗一律拒絕。

實作位置: `internal/experiment/judge.go:340-347`（緊接在 burn-in gate 之後）。
這是**資料完整性 gate**，不接受「稍後手動審查」的替代方案。

## OOS Gate Ordering (passesAcceptance vs OOSValidation)

`Evaluate()` 內部的執行順序為：

1. **OOS validation 先跑**（`runOOSValidation`）— 填入 `result.OOSResult`
2. **OOS 硬失敗時直接拒絕**（error 或 !Passed）
3. **才跑 `passesAcceptance()`** — 此時 `result.OOSResult` 已填，`no_drawdown_spike` gate 才能正確檢查

若反過來（先 passesAcceptance 再 OOS），`no_drawdown_spike` gate 會看到 nil 的 `OOSResult`，形同關閉。**不可重排順序**。
