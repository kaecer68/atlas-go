# 事件驅動資金流預測納入當前資金流基線

## Session Boundary

- Mode: Execute
- Branch: `fix/eventdriven-capital-flow-baseline`
- Worktree: `/Users/kaecer/workspace/atlas/.worktrees/fix-eventdriven-capital-flow-baseline`
- Manifest: 本文件
- In-scope IDs: 使用者追問「5 日錢潮預測為何全顯流入，但當日法人資金實際流出」
- ATLAS_ENV: development

## Problem Statement

`/api/events/prediction` 的 5 日事件驅動預測僅使用事件日曆，未將當日實際法人資金流向（capital flow QualityScore）納入基線。當 `CapitalFlowAssessment` 處於 `calibrating` 時，現行邏輯強制把 `cfScore` 設為 0，導致預測完全由事件驅動，與當前市場資金流向脫節，產生使用者可見的矛盾（例如：今日三大法人賣超，未來 5 天卻全預測流入，信心 0.88）。

## Design Intent

1. 將 `QualityScore()` 視為「當前資金流基線訊號」（baseline drift / momentum component），不再被 `EligibleForAutomation()` 閘道強制歸零。
2. 基線權重隨預測日遞減（day 1 最高，day 5 最低），反映事件訊號隨時間增加、當前資金流動能隨時間衰減。
3. 校準狀態影響基線權重與信心上限：
   - `eligible`：完整權重、不額外壓制信心。
   - `calibrating`：基線權重折扣、信心上限 0.6，並於摘要標示不確定性。
   - `degraded` / `error`：基線權重大幅折扣。
4. 摘要文字須同時說明「當前資金流向」與「事件日曆方向」，並於兩者衝突時表達「方向分歧 / 預測不確定性高」。
5. 此變更不觸及 `CapitalFlowProvider` 介面；僅改變 `Predictor` 對既有方法的詮釋方式與測試預期。

## Acceptance Criteria

- [ ] `/api/events/prediction` 在 `calibrating` 狀態下仍會將當日資金流方向納入預測。
- [ ] 當事件方向與資金流基線衝突時，近 1-2 日預測偏向中性，而非全盤事件方向。
- [ ] `calibrating` 狀態的預測信心不超過 0.6。
- [ ] 摘要文字包含當前資金流方向與（如適用）「校準中」提示。
- [ ] 既有測試更新並通過；新增測試覆蓋 `calibrating` 基線衝突場景。
- [ ] `make ci-gate` 通過。
- [ ] `make check-binaries` 通過（如 binary 變動）。

## Backlog

- 未來可進一步改用 `DailyReport.Forces` 原始 Z-score 取代 legacy `QualityScore`，作為更精細的基線；本次保留既有介面以控制衝擊範圍。
