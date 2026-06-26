# Risk 模組孤兒組態檔清理（2026-06-20）

> **文件角色**：審計紀錄。  
> **來源**：原記載於 `internal/risk/AGENTS.md`，因屬歷史裁定與清理紀錄搬遷至此。  
> **狀態**：已解決。

## 發現

曾存在孤兒檔案 `internal/risk/configs/parameters.json`（190KB，與全域參數高度重疊，未追蹤於 git）。

## 處置

- 已於對應 commit 刪除。
- 0 個 `.go` 檔案引用它。

## 後續規範

`internal/risk` 統一使用全域 `config.GetParametersConfig()`，禁止 per-module 組態檔。正確做法與理由見 `internal/risk/AGENTS.md` 的「組態設定」段。
