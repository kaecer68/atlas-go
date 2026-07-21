# DATA DIRECTORY STANDARD — atlas-go

**版本**: 1.0  
**日期**: 2026-06-02  
**狀態**: 權威標準（authoritative）  
**適用範圍**: `data/` 目錄下所有子目錄結構  
**強制性**: CI 強制檢查（`scripts/ci/check_data_naming.sh`，P0.4a 實現）  
**相關文檔**: `docs/data-naming-convention.md`（命名規範）· `docs/data-architecture.md`（資料架構）· `docs/data-catalog.md`（資料目錄）

---

## 1. 背景與動機

### 1.1 問題發現

2026-06-02 的 P0.1 審計發現，`data/state/` 目錄同時存在 **17 個平面檔案** 與 **23 個子目錄**，混亂共存。例如：
- `data/state/baseline_policy.json`（平面檔案）vs `data/state/alerts/`（子目錄）
- `data/state/channel_health.json`（平面檔案）vs `data/state/macro/`（子目錄）
- `data/state/darwinian_weights.json`（平面檔案）vs `data/state/experiments/`（子目錄）

這種混亂導致：
1. **AI 探索成本高**：無法一眼判斷某目錄下有多少資料資產
2. **新增資料無指引**：開發者隨意放置，無規範可依循
3. **命名空間污染**：平面檔案與子目錄競爭同一層級

### 1.2 核心原則

> **data/state/ 下只有子目錄，沒有平面檔案。**  
> 每個子目錄對應一個資料資產或一組相關資產。

---

## 2. 目標目錄結構

```
data/
├── replay/                  # 回放數據（CSV/JSONL，唯讀歷史資料）
│   ├── tw_extended_90days.csv
│   └── *.jsonl
│
├── cache/                   # 臨時快取（可重新生成，不版本控制）
│   ├── dividends/           # 股息快取
│   └── ...
│
├── reference/               # 參考數據（靜態，手動維護，版本控制）
│   ├── sector_data/         # 產業分類數據
│   │   └── sector_data.json
│   ├── fundamentals.json    # 基本面參考數據
│   └── test_returns.json    # 測試用報酬數據
│
├── state/                   # 運行時狀態（持久化，子目錄強制）
│   ├── approvals/           # 人工核准記錄
│   ├── alerts/              # 系統告警
│   ├── baseline/            # Baseline policy 狀態
│   │   └── baseline_policy.json
│   ├── branch_protection/   # 分支保護快照
│   ├── capital_flow/        # 資金流向（每日）
│   ├── channel_health/      # 通道健康狀態
│   ├── clamping/            # 權重夾制記錄
│   ├── darwinian/           # Darwinian 權重管理
│   │   ├── darwinian_weights.json
│   │   └── darwinian_history.jsonl
│   	├── strategy_techniques/  # 投資心法狀態
│   ├── experiments/         # 實驗記錄
│   ├── export/              # 匯出資料
│   ├── finmind/             # FinMind 資料快取
│   ├── fubon/               # Fubon 資料快取
│   ├── fugle/               # Fugle 資料快取
│   ├── geopolitical/        # 地緣政治事件數據
│   ├── human_interventions/ # 人工干預記錄
│   ├── live/                # 即時交易狀態
│   ├── macro/               # 總經指標（每日）
│   ├── margin/              # 融資融券（每日）
│   ├── maturity/            # 成熟度追蹤
│   ├── metalearner/         # 元學習狀態
│   ├── metrics/             # 系統指標
│   ├── ml_models/           # ML 模型檔案
│   ├── mutation_briefs/     # 突變提案
│   ├── parameter_snapshots/ # 參數快照
│   ├── portfolio_allocation/# 投組配置版本
│   ├── session_summaries/   # Session 摘要（從 sessions/ 目錄遷出）
│   ├── sessions/            # Session 記錄（每個 session 一個子目錄）
│   │   └── session-YYYYMMDD-daily/
│   ├── simulation/          # 模擬狀態（從平面檔案遷入）
│   │   └── simulation_state.json
│   ├── swarm/               # Swarm 訓練記錄
│   │   ├── swarm_latest.json
│   │   └── swarm_training/
│   ├── traces/              # 執行追蹤
│   ├── tsmc_revenue/        # 台積電營收數據
│   └── windows/             # 回測視窗記錄
│
├── archive/                 # 歷史歸檔（可選，不參與 CI 檢查）
│   ├── state-archive/       # 舊的狀態快照
│   └── *.backup.*           # 手動備份檔案
│
├── schemas/                 # JSON Schema 定義（由數據治理管理）
│   └── *.schema.json
│
└── README.md                # 資料目錄入口（參見 docs/data-catalog.md）
```

---

## 3. 分類規則

### 3.1 四層分類

| 分類 | 目錄 | 說明 | 版本控制 | CI 檢查 |
|------|------|------|----------|---------|
| **replay** | `data/replay/` | 歷史回放數據，唯讀 | ✅ 是 | ❌ 否（測試數據） |
| **cache** | `data/cache/` | 可重新生成的快取 | ❌ 否 | ❌ 否 |
| **reference** | `data/reference/` | 靜態參考數據 | ✅ 是 | ✅ 是 |
| **state** | `data/state/` | 運行時持久狀態 | ❌ 否（gitignored） | ⚠️ 結構檢查 |

### 3.2 state/ 子目錄規則

**強制規則**：

| 規則 | 說明 |
|------|------|
| **R1: 禁止平面檔案** | `data/state/` 下不允許任何平面檔案（.json/.jsonl/.db），所有檔案必須在子目錄中 |
| **R2: 一個資產一個子目錄** | 每個子目錄對應一個邏輯資料資產（或一組緊密相關的資產） |
| **R3: 子目錄命名 snake_case** | 遵循 `docs/data-naming-convention.md` R1 規則 |
| **R4: 每日數據子目錄統一格式** | 每日數據（macro/、margin/、capital_flow/）使用相同命名規範 |
| **R5: 每個子目錄必須有 `_metadata.json`** | 遵循 `docs/data-maturity-standard.md` |

### 3.3 何時需要子目錄

| 條件 | 子目錄 | 平面檔案 | 範例 |
|------|--------|----------|------|
| 單一檔案，無版本歷史 | ❌ | ✅（暫存） | migration tool output |
| 單一檔案，有歷史版本 | ✅ | ❌ | baseline_policy.json → baseline/ |
| 多個相關檔案 | ✅ | ❌ | experiments/（183 JSON + archive/） |
| 每日累積數據 | ✅ | ❌ | macro/（38 daily files） |
| 單一 JSONL（append-only） | ✅ | ❌ | recommendation_outcomes.jsonl → outcomes/ |

---

## 4. 遷移路徑

### 4.1 遷移階段

| Phase | 內容 | 風險 | 分支 |
|-------|------|------|------|
| **Phase 1** | 命名標準化（P1.1） | 低 | `feat/data-organization-governance` |
| **Phase 2** | 平面檔案搬遷到子目錄（P3.0） | 高 | `feat/data-restructure`（獨立分支） |
| **Phase 3** | 舊路徑相容期（6 個月） | 中 | 後續分支 |

### 4.2 遷移對照表（Phase 2）

| 目前位置（平面檔案） | 目標位置（子目錄） |
|-------------------|-------------------|
| `data/state/atlas.db` | 保留原位或遷移至 `data/state/database/atlas.db`（取決於 P2.1 決策） |
| `data/state/baseline_policy.json` | `data/state/baseline/baseline_policy.json` |
| `data/state/channel_health.json` | `data/state/channel_health/channel_health.json` |
| `data/state/clamping_events.jsonl` | `data/state/clamping/clamping_events.jsonl` |
| `data/state/darwinian_history.jsonl` | `data/state/darwinian/darwinian_history.jsonl` |
| `data/state/darwinian_weights.json` | `data/state/darwinian/darwinian_weights.json` |
| `data/state/experiments.jsonl` | `data/state/experiments/experiments.jsonl` |
| `data/state/human_interventions.jsonl` | `data/state/human_interventions/human_interventions.jsonl` |
| `data/state/maturity_tracker.json` | `data/state/maturity/maturity_tracker.json` |
| `data/state/metalearner_state.json` | `data/state/metalearner/metalearner_state.json` |
| `data/state/metrics.jsonl` | `data/state/metrics/metrics.jsonl` |
| `data/state/phase3_metrics.json` | `data/state/metrics/phase3_metrics.json` |
| `data/state/recommendation_outcomes.jsonl` | `data/state/outcomes/recommendation_outcomes.jsonl` |
| `data/state/simulation_state.json` | `data/state/simulation/simulation_state.json` |
| `data/state/swarm_latest.json` | `data/state/swarm/swarm_latest.json` |

### 4.3 遷移安全規則

1. **必須在獨立分支執行**（`feat/data-restructure`）
2. **每個 Go 檔案參考必須更新**：`grep -r "data/state/" --include="*.go" internal/ cmd/` 後逐一更新
3. **遷移腳本必須保留**：`scripts/migrate_data_structure.sh` 記錄完整步驟
4. **舊路徑相容符號連結**（可選，過渡期）：在 `data/state/` 下建立指向新位置的 symlink，6 個月後移除

---

## 5. 新數據資產加入流程

當開發者需要新增資料資產時，必須遵循：

```
1. 確認資產分類：replay / cache / reference / state？
2. 若為 state/，確認是否需要新建子目錄？遵循 R2-R5 規則
3. 建立子目錄（如需要）
4. 建立 _metadata.json（遵循 data-maturity-standard.md）
5. 更新 docs/data-catalog.md
6. 更新 Go 程式碼中的路徑參考
7. 執行 CI 檢查：bash scripts/ci/check_data_naming.sh
```

---

## 6. 例外情況

| 例外 | 理由 | 處理方式 |
|------|------|----------|
| `data/state/live/state/` | 即時交易模組內部結構 | 維持現狀，不強制重整 |
| `data/state/sessions/` | 每個 session 已是子目錄 | 維持現狀（已符合規範） |
| `data/replay/` 平面 CSV/JSONL | replay 屬唯讀歷史數據 | 允許平面檔案 |

---

## 7. CI 強制執行

本標準將由 `scripts/ci/check_data_naming.sh`（P0.4a）強制執行：

- **禁止**：`data/state/` 下的任何平面檔案（非子目錄）
- **警告**：`data/state/` 子目錄缺少 `_metadata.json`
- **禁止**：新建目錄使用 kebab-case（必須 snake_case）

---

## 8. 相關文檔

| 文檔 | 關係 |
|------|------|
| `docs/data-naming-convention.md` | 命名規範（R1-R10），被本文件引用 |
| `docs/data-maturity-standard.md` | `_metadata.json` 格式定義 |
| `docs/data-catalog.md` | 完整資料資產目錄 |
| `docs/data-architecture.md` | 資料架構與讀寫路徑 |
| `.omo/audit/2026-06-02-p0-2-root-cause-analysis.md`（內部）| 根因分析（FG-1: 無規範）|`.omo/audit/2026-06-02-p0-2-root-cause-analysis.md`（內部）| 根因分析（FG-1: 無規範） |
