# Monitoring 前端 PascalCase → snake_case 欄位錯誤（2026-05-07）

> **文件角色**：Incident / 審計紀錄。  
> **來源**：原記載於 `internal/monitoring/AGENTS.md`，因屬已修復事件的詳細調查與修復過程，搬遷至此。  
> **狀態**：已修復。本文件保留以供日後稽核與預防類似問題參考。

## 問題描述

前端 JavaScript 錯誤地使用 PascalCase 存取 API 回傳的 snake_case 欄位，導致畫面顯示 `undefined`。

## 受影響欄位（`GuardOutcome`）

| Go 欄位名 | JSON tag | 前端錯誤引用 | 前端正確引用 |
|-----------|----------|-------------|-------------|
| `GuardID` | `guard_id` | `g.GuardID` | `g.guard_id` |
| `GuardSkill` | `guard_skill` | `g.GuardSkill` | `g.guard_skill` |
| `Passed` | `passed` | `g.Passed` | `g.passed` |
| `Reason` | `reason` | `g.Reason` | `g.reason` |
| `InputCount` | `input_count` | `g.InputCount` | `g.input_count` |
| `OutputCount` | `output_count` | `g.OutputCount` | `g.output_count` |

## 影響範圍

- `renderMacroRadar()` — 總經雷達頁面顯示 `undefined 筆推薦 → 最終放行 undefined 筆`
- `renderDecisionChain()` — 決策鏈頁面控制層紀錄顯示異常
- `renderPipeline()` — 投資管線頁面控制層徽章顯示異常

## 修復方式

1. 將所有前端 PascalCase 屬性存取改為 snake_case。
2. 添加防禦性預設值（`g.input_count || 0`）防止未來欄位缺失。
3. 新增 `validateApiResponse()` 函數，在開發階段自動檢測欄位命名不一致。

## 預防措施

- 後端 `domain.*` 型別的 JSON tag 一律為 snake_case。
- 前端存取 API 回應時必須使用 snake_case。
- 新增 `validateApiResponse(data, requiredFields, context)` 驗證工具，自動檢測 PascalCase 誤用。

## 現行規則

`internal/monitoring/AGENTS.md` 保留的決策性規則：

> 從 JSONL（`recommendation_outcomes.jsonl`）讀取時，注意部分 legacy 欄位使用 PascalCase（如 `AgentID`、`Skill`），Unmarshal 結構必須精確對應。前端存取 API 回應時必須使用 snake_case。
