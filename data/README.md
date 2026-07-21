# atlas-go Data Directory

**資料治理框架**: 2026-06-02 導入 · 遵循 `docs/` 下的六份權威標準文件

## 快速導航

| 你想找什麼？ | 去哪裡？ |
|-------------|---------|
| **完整資料目錄**（39 個資產） | [`docs/data-catalog.md`](../docs/data-catalog.md) |
| **資料架構與讀寫路徑** | [`docs/data-architecture.md`](../docs/data-architecture.md) |
| **檔案命名規範** | [`docs/data-naming-convention.md`](../docs/data-naming-convention.md) |
| **目錄結構規範** | [`docs/data-directory-standard.md`](../docs/data-directory-standard.md) |
| **JSON Schema 標準** | [`docs/json-schema-standard.md`](../docs/json-schema-standard.md) |
| **資料成熟度標準** | [`docs/data-maturity-standard.md`](../docs/data-maturity-standard.md) |
| **JSON Schema 檔案** | [`schemas/`](../schemas/) |

## 目錄結構總覽

```
data/
├── replay/          → 歷史回放數據（CSV/JSONL，唯讀）
├── cache/           → 可重新生成的快取（不版本控制）
├── reference/       → 靜態參考數據（sector_data, fundamentals）
│
├── state/           → 運行時持久狀態（gitignored，子目錄強制）
│   ├── sessions/    → Session 目錄（最完整的 outcome 數據）★
│   ├── outcomes/    → 全域推薦結果聚合
│   ├── experiments/ → 實驗記錄
│   ├── macro/       → 總經指標（每日）
│   ├── margin/      → 融資融券（每日）
│   ├── capital_flow/→ 資金流向（每日）
│   ├── darwinian/   → Darwinian 權重管理
│   ├── baseline/    → Baseline policy
│   ├── approvals/   → 人工核准記錄
│   ├── swarm/       → Swarm 訓練記錄
│   ├── traces/      → 執行追蹤
│   ├── windows/     → 回測視窗
│   └── ...          → 其他 20 個子目錄
│
├── state-archive/   → 歷史歸檔（⚠️ 待清理）
└── schemas/         → JSON Schema 定義
```

> ★ Session 目錄有最完整的數據（含 per-agent forward return）。AI agent 應優先從此讀取。

## 重要提示

- **`data/state/` 已 gitignored** — 本地運行時數據，不納入版本控制
- **所有 `data/state/` 子目錄必須有 `_metadata.json`** — 遵循 `docs/data-maturity-standard.md`
- **命名必須遵循規範** — `docs/data-naming-convention.md` 的 R1-R10 規則，CI 強制檢查
- **新增資料資產** → 更新 `docs/data-catalog.md`（CI 會檢查）

## 已知問題

| 問題 | 狀態 | 追蹤 |
|------|------|------|
| 全域 outcomes 14.7x 大於 per-session 總和 | 已知，待調查 | P0.1 C.3 |
| atlas.db 冗餘（PostgreSQL 重疊） | 決策：移除 | `內部審計（.omo/audit/）` |
| 7 個空歸檔目錄 | 待清理 | P2.2 |
| backup 檔案 orphaned | 待清理 | P2.2 |
| 平面檔案混雜於子目錄 | 獨立分支處理 | P3.0 (`feat/data-restructure`) |
