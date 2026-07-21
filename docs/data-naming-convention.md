# DATA NAMING CONVENTION — atlas-go

**版本**: 1.0  
**日期**: 2026-06-02  
**狀態**: 權威標準（authoritative）  
**適用範圍**: `data/` 目錄下所有檔案與目錄  
**強制性**: CI 強制檢查（`scripts/ci/validate_data_naming.sh`，P0.4a 實現）  
**相關文檔**: `docs/developer-guide.md`（Go 代碼命名） · `docs/data-architecture.md`（資料架構） · `.omo/audit/2026-06-02-p0-2-root-cause-analysis.md`（內部）

---

## 1. 背景與動機

### 1.1 問題發現

2026-06-02 的 P0.2 根因分析（FG-1）發現：atlas-go 在 1,144 次 commit 中，**從未存在過任何資料檔案命名規範**。證據：

- `git log --all --oneline | grep -i 'data.*convent'` → **0 matches**
- 3 個每日數據目錄使用 3 種不同日期格式（`YYYY-MM-DD`, `YYYYMMDD_margin`, `YYYYMMDD`）
- 4 個目錄用 kebab-case，4 個用 snake_case，16 個用 plain
- `developer-guide.md`（2026-05-07）僅規範 Go 源碼檔案的 snake_case，未涵蓋資料檔案
- `/data/state/` 自 commit #2（2026-03-30）起被 gitignore，使強制執行不可能

### 1.2 設計目標

| 目標 | 說明 |
|------|------|
| **一致性** | 同一類型的資料使用同一格式，無論誰建立 |
| **可排序性** | 日期命名支援 `ls` 預設字母排序即等於時間排序 |
| **可解析性** | 名稱結構可被正則表達式機械驗證 |
| **可讀性** | 人類可從檔名一眼識別內容與時間 |
| **相容性** | 與現有 Go snake_case 慣例對齊 |

### 1.3 權威層級

```
本文件 (data-naming-convention.md)  ← 資料檔案命名之最終權威
    ↑
developer-guide.md (§命名慣例)     ← Go 源碼檔案命名（不相關）
AGENTS.md                           ← 倉庫級邊界（不覆蓋本文件）
```

當本文件與其他文件的命名規則衝突時，**本文件為數據命名的最終仲裁者**。

---

## 2. 核心規則總覽

| 規則 # | 適用對象 | 規範 | 強制性 |
|--------|---------|------|--------|
| R1 | 目錄名稱 | `snake_case` | 強制 |
| R2 | 每日數據檔案 | `YYYYMMDD_descriptor.json` | 強制 |
| R3 | 非日期數據檔案 | `snake_case.json` / `snake_case.jsonl` | 強制 |
| R4 | JSONL 檔案 | 副檔名 `.jsonl`，append-only | 強制 |
| R5 | JSON 檔案 | 副檔名 `.json`，覆蓋寫入 | 強制 |
| R6 | 備份檔案 | `.backup.YYYYMMDDHHMMSS` 後綴，儲存在 `backups/` 子目錄 | 強制 |
| R7 | 暫存檔案 | `.tmp.` 前綴或 `.tmp` 副檔名，位於 `tmp/` 或工作目錄 | 建議 |
| R8 | 禁止字符 | 無 `kebab-case`、無空格、無大寫字母（`README.md` 除外） | 強制 |
| R9 | Session 目錄 | `session_YYYYMMDD_daily/` | 強制 |
| R10 | CSV 檔案 | `snake_case[_YYYYMMDD].csv` | 強制 |

---

## 3. 詳細規則

### R1：目錄名稱 — `snake_case`

**所有 `data/` 下的目錄必須使用 `snake_case`**（小寫字母、數字、底線）。

#### ✅ 正確

```
data/state/capital_flow/
data/state/tsmc_revenue/
data/state/branch_protection_snapshots/   ← 遷移自 kebab-case
data/state/constraint_mutations/           ← 遷移自 kebab-case
data/state/mutation_briefs/               ← 遷移自 kebab-case
data/state/parameter_snapshots/           ← 遷移自 kebab-case
data/state/swarm_training/
data/state/ml_models/
data/replay/
data/cache/
data/sector_data/
data/state/sessions/
```

#### ❌ 錯誤

```
data/state/branch-protection-snapshots/   ← kebab-case 禁止
data/state/constraint-mutations/           ← kebab-case 禁止
data/state/mutation-briefs/               ← kebab-case 禁止
data/state/parameter-snapshots/           ← kebab-case 禁止
data/state/CapitalFlow/                   ← 大寫字母禁止
data/state/branch protection/             ← 空格禁止
```

#### 特殊例外

- `README.md`、`VERSION` — 工具強制要求（developer-guide.md §例外）
- `state-archive/` — 現有工具產物，遷移至 `data/archive/`（見 §7.3）
- 標準縮寫可接受（如 `tsmc_revenue` 而非 `taiwan_semiconductor_manufacturing_company_revenue`），但必須全小寫 + 底線

---

### R2：每日數據檔案 — `YYYYMMDD_descriptor.json`

**所有按日產生的數據檔案必須使用 `YYYYMMDD_descriptor.json` 格式**，其中 `YYYYMMDD` 為交易日，`descriptor` 描述數據類型（`snake_case`）。

#### 格式說明

```
YYYYMMDD_descriptor.json
│       │          │
│       │          └── 副檔名（json 或 jsonl）
│       └── 描述詞（snake_case，不超過 3 個單詞）
└── 8 位日期（西元年月日，無分隔符）
```

#### ✅ 正確

```
data/state/macro/20260412_macro.json
data/state/margin/20260420_margin.json
data/state/capital_flow/20250519_capital_flow.json
data/state/export/20260602_export.json
```

#### ❌ 錯誤

```
data/state/macro/2026-04-12.json            ← 日期含連字符（ISO format，禁止）
data/state/macro/2026_04_12_macro.json      ← 日期含底線（冗餘）
data/state/capital_flow/20250519.json       ← 缺失 descriptor（無法識別內容）
data/state/margin/20260420margin.json       ← descriptor 與日期無分隔
data/state/export/11501_export.json         ← 非標準日期格式
data/state/macro/Macro_20260412.json        ← 描述詞在前、大寫字母
data/state/macro/2026-04-12_macro.json      ← 混合 ISO 日期 + 底線分隔
```

#### 腳本驗證模式

```bash
# 匹配合法格式的正則表達式
pattern='^[0-9]{8}_[a-z][a-z0-9_]*\.(json|jsonl)$'

# 每日目錄中的所有檔案都必須匹配此模式
# 例外：README.md、_metadata.json、catalog.json
```

#### 設計理由：為何選 `YYYYMMDD` 而非 `YYYY-MM-DD`

1. **字母排序 = 時間排序**：`ls` 預設輸出即為時間順序，無需 `sort`
2. **無需跳脫**：shell glob 中 `[0-9]{8}` 可直接匹配，無連字符干擾
3. **與現有慣例對齊**：`sessions/` 目錄使用 `session-YYYYMMDD-daily`，日期部分一致
4. **國際標準**：`YYYYMMDD` 是 ISO 8601 的基本格式（Basic format）

---

### R3：非日期數據檔案 — `snake_case.json` / `snake_case.jsonl`

**不附帶日期的持久檔案使用 `snake_case` 命名**，描述詞清楚表達內容。

#### ✅ 正確

```
data/state/baseline_policy.json
data/state/darwinian_weights.json
data/state/darwinian_history.jsonl
data/state/channel_health.json
data/state/simulation_state.json
data/state/maturity_tracker.json
data/state/phase3_metrics.json
data/state/human_interventions.jsonl
data/state/metalearner_state.json
data/state/swarm_latest.json
data/state/metrics.jsonl
data/state/clamping_events.jsonl
data/state/experiments.jsonl
data/state/sector_data/sector_data.json
data/state/recommendation_outcomes.jsonl
```

#### ❌ 錯誤

```
data/state/baseline-policy.json             ← kebab-case 禁止
data/state/BaselinePolicy.json              ← PascalCase 禁止
data/state/Phase3_Metrics.json              ← 混合大小寫禁止
data/state/experiments.json                 ← 實際為 JSONL 卻用 .json 副檔名
```

---

### R4：JSONL 檔案 — `.jsonl` 副檔名，append-only

**append-only 的 JSON Lines 檔案必須使用 `.jsonl` 副檔名**，以區別於覆蓋寫入的 `.json` 檔案。

#### JSON vs JSONL 區別

| 特性 | `.json` | `.jsonl` |
|------|---------|----------|
| 寫入模式 | 覆蓋（overwrite） | 附加（append-only） |
| 格式 | 單一 JSON 物件或陣列 | 每行一個獨立 JSON 物件 |
| 讀取 | 一次性載入全部 | 逐行讀取 |
| 範例 | `baseline_policy.json` | `recommendation_outcomes.jsonl` |

#### ✅ 正確

```
data/state/recommendation_outcomes.jsonl    ← append-only，每行一筆 outcome
data/state/experiments.jsonl               ← append-only，每行一筆實驗記錄
data/state/darwinian_history.jsonl         ← append-only，每行一筆權重快照
data/state/metrics.jsonl                   ← append-only，每行一筆指標
data/state/human_interventions.jsonl       ← append-only，每行一個人為干預
data/state/clamping_events.jsonl           ← append-only
data/replay/tw_combined.jsonl
data/replay/twse_20260402.jsonl
```

#### ❌ 錯誤

```
data/state/recommendation_outcomes.json     ← 實際為 JSONL，副檔名誤導
data/state/darwinian_history.json           ← 實際為 JSONL，副檔名誤導
```

---

### R5：JSON 檔案 — `.json` 副檔名，覆蓋寫入

**覆蓋寫入的 JSON 檔案使用 `.json` 副檔名**，每次寫入取代舊內容。

#### ✅ 正確

```
data/state/baseline_policy.json            ← 覆蓋寫入（最新策略）
data/state/darwinian_weights.json          ← 覆蓋寫入（當前權重）
data/state/channel_states.json             ← 覆蓋寫入（最新狀態）
data/state/metalearner_state.json          ← 覆蓋寫入
data/state/simulation_state.json           ← 覆蓋寫入
```

---

### R6：備份檔案 — `.backup.YYYYMMDDHHMMSS` 後綴

**備份檔案必須使用 `.backup.YYYYMMDDHHMMSS` 後綴**，儲存在其所備份檔案的同一目錄下的 `backups/` 子目錄中。

#### 格式說明

```
<original_filename>.backup.YYYYMMDDHHMMSS
                     │      │
                     │      └── 14 位時間戳（年月日時分秒）
                     └── 固定後綴（與原副檔名無衝突）
```

#### ✅ 正確

```
data/state/backups/recommendation_outcomes.jsonl.backup.20260414062052
data/state/backups/baseline_policy.json.backup.20260602143000
data/state/backups/darwinian_weights.json.backup.20260602090000
```

#### ❌ 錯誤

```
data/state/recommendation_outcomes.jsonl.backup.20260414062052  ← 在根目錄而非 backups/
data/state/backups/recommendation_outcomes.bak                  ← 無時間戳，無法追溯
data/state/backups/backup_20260414.jsonl                        ← 前綴而非後綴，破壞字母排序
data/state/backups/recommendation_outcomes_copy.jsonl           ← 非標準命名
```

#### 備份生命週期

- **建立**：只在必要時手動建立（大型遷移前、實驗性變更前）
- **儲存位置**：`<data_dir>/backups/`（如 `data/state/backups/`）
- **保留期限**：7 天。`scripts/ci/cleanup_backups.sh` 自動清理超過 7 天的備份
- **不進入 git**：`/data/state/backups/` 應加入 `.gitignore`（若 `data/state/` 已 gitignored 則不需額外設定）

---

### R7：暫存檔案 — `.tmp` 前綴或副檔名

**寫入進行中的暫存檔案使用 `.tmp` 識別**，完成後 rename 為最終檔名（原子寫入）。

#### ✅ 正確

```
data/state/sessions/session_20260602_daily/.tmp.recommendation_outcomes.jsonl
data/state/.tmp.darwinian_weights.json
/tmp/atlas_macro_fetch_20260602.tmp
```

#### ❌ 錯誤

```
data/state/recommendation_outcomes.jsonl.temp     ← 非標準後綴
data/state/recommendation_outcomes_wip.jsonl     ← 無標準識別
```

#### 清理規則

- `.tmp` 檔案不應存活超過一次寫入週期
- CI 檢查（`detect_stale_tmp_files.sh`）：任何存在超過 1 小時的 `.tmp` 檔案視為異常
- 原子寫入模式：`write .tmp → fsync → rename .tmp → final`

---

### R8：禁止字符

**所有 `data/` 下的檔案與目錄名稱不得包含以下字符**：

| 禁止 | 原因 |
|------|------|
| 連字符 `-`（kebab-case） | 與 snake_case 不一致；shell 中需跳脫 |
| 空格 | 跨平台相容性差；shell 中需跳脫 |
| 大寫字母（`README.md`、`VERSION` 除外） | 大小寫敏感的檔案系統（Linux）與不敏感的（macOS）行為不同 |
| 特殊字符（`!@#$%^&*()+=[]{}|;:'",<>?/~\``） | 跨平台相容性；shell glob 混亂 |

#### ✅ 正確

```
data/state/capital_flow/
data/state/tsmc_revenue/
data/state/ml_models/
data/state/sessions/session_20260602_daily/
```

#### ❌ 錯誤

```
data/state/branch-protection-snapshots/      ← 連字符
data/state/CapitalFlow/                      ← 大寫
data/state/branch protection/               ← 空格
data/state/API_data/                         ← 大寫 + 底線
```

---

### R9：Session 目錄 — `session_YYYYMMDD_daily/`

**每個模擬 session 的目錄使用 `session_YYYYMMDD_daily` 格式**。

#### 格式說明

```
session_YYYYMMDD_daily/
│       │        │
│       │        └── 頻率（daily / weekly / monthly）
│       └── 8 位交易日日期
└── 固定前綴
```

#### ✅ 正確

```
data/state/sessions/session_20260602_daily/
data/state/sessions/session_20260602_daily/summary.json
data/state/sessions/session_20260602_daily/recommendation_outcomes.jsonl
data/state/sessions/session_20260602_daily/screened_symbols.jsonl
data/state/sessions/session_20260602_daily/positions.json
data/state/sessions/session_20260602_daily/experiments.jsonl
```

#### 遷移說明

現有格式 `session-YYYYMMDD-daily`（kebab-case 前綴）→ 新格式 `session_YYYYMMDD_daily`（snake_case 前綴）。遷移腳本見 §7.4。

---

### R10：CSV 檔案 — `snake_case[_YYYYMMDD].csv`

**CSV 檔案（replay 或多日數據）使用 `snake_case` 命名，可選日期後綴**。

#### ✅ 正確

```
data/replay/merged.csv
data/replay/tw_extended_90days.csv
data/replay/twse_stocks_202604.csv
data/replay/finmind_2020_2024.csv
```

#### ❌ 錯誤

```
data/replay/Merged.csv                       ← 大寫字母
data/replay/tw-extended.csv                 ← kebab-case
data/replay/data.csv                        ← 過於通用，無法識別內容
```

---

## 4. JSON vs JSONL 選擇指南

| 場景 | 使用 `.json` | 使用 `.jsonl` |
|------|------------|-------------|
| 單一記錄／最新狀態 | ✅ | ❌ |
| 逐筆累積的歷史記錄 | ❌ | ✅ |
| 可完整載入記憶體 | ✅ | ❌（但也可） |
| 超大資料集（GB+） | ❌ | ✅ |
| 需原子替換的配置 | ✅ | ❌ |
| 人類需頻繁手動編輯 | ✅ | ❌ |
| 逐行處理的串流資料 | ❌ | ✅ |

---

## 5. 目錄結構規範

### 5.1 階層深度

`data/state/` 下最大目錄深度為 **3 層**（不含 `data/state/` 本身），確保可管理性：

```
data/state/<domain>/<subtype>/   ← 第 3 層（最大）
data/state/<domain>/             ← 第 2 層
data/state/                      ← 第 1 層
```

### 5.2 平坦檔案限制

`data/state/` **根目錄不應有超過 5 個平坦檔案**。超出此數量的資料應組織到子目錄中。

現有平坦檔案（15 個）應遷移至子目錄（P1.1 任務）：

| 現有位置 | 遷移至 |
|---------|--------|
| `baseline_policy.json` | `data/state/baseline/baseline_policy.json` |
| `darwinian_weights.json` | `data/state/darwinian/darwinian_weights.json` |
| `darwinian_history.jsonl` | `data/state/darwinian/darwinian_history.jsonl` |
| `clamping_events.jsonl` | `data/state/darwinian/clamping_events.jsonl` |
| `channel_health.json` | `data/state/gateway/channel_health.json` |
| `channel_states.json` | `data/state/gateway/channel_states.json` |
| `experiments.jsonl` | `data/state/experiments/experiments.jsonl` |
| `human_interventions.jsonl` | `data/state/approvals/human_interventions.jsonl` |
| `maturity_tracker.json` | `data/state/system/maturity_tracker.json` |
| `metalearner_state.json` | `data/state/system/metalearner_state.json` |
| `metrics.jsonl` | `data/state/metrics/metrics.jsonl` |
| `phase3_metrics.json` | `data/state/metrics/phase3_metrics.json` |
| `simulation_state.json` | `data/state/system/simulation_state.json` |
| `swarm_latest.json` | `data/state/swarm/swarm_latest.json` |
| `recommendation_outcomes.jsonl` | `data/state/outcomes/recommendation_outcomes.jsonl` |

---

## 6. 驗證規則（供 CI 腳本使用）

以下規則可直接轉換為 `scripts/ci/validate_data_naming.sh` 的正則表達式或 glob 檢查：

### 6.1 目錄名稱驗證

```bash
# 所有 data/state/ 下的目錄必須匹配 snake_case
# 例外：sessions/（session 目錄內部用 session_ 格式）
pattern='^[a-z][a-z0-9_]*$'

# 禁止的目錄名稱
forbidden='- [A-Z] '   # 連字符、大寫、空格
```

### 6.2 每日數據檔案驗證

```bash
# macro/、margin/、capital_flow/、export/ 目錄中的所有 .json 檔案
# 必須匹配 YYYYMMDD_descriptor.json 格式
pattern='^[0-9]{8}_[a-z][a-z0-9_]*\.json$'

# 例外清單（這些不是數據檔案）：
exceptions='README.md _metadata.json catalog.json'
```

### 6.3 JSON vs JSONL 副檔名驗證

```bash
# .jsonl 副檔名：檔案內容必須是每行一個獨立 JSON 物件
# .json 副檔名：檔案內容必須是單一 JSON 物件或陣列（可一次性載入）

# CI 可透過檢查檔案第一行來驗證：
# - .jsonl 檔案的第一行必須是完整 JSON 物件（以 { 開頭，以 } 結尾）
# - .jsonl 檔案不得以 [ 開頭（那表示 JSON 陣列，非 JSONL）
```

### 6.4 備份檔案位置驗證

```bash
# 備份檔案必須位於 backups/ 子目錄中，格式必須包含時間戳
pattern='^.+\.backup\.[0-9]{14}$'

# data/state/ 根目錄中的 backup 檔案視為違規
```

### 6.5 禁止字符驗證

```bash
# 檔案名稱不得包含：連字符、空格、大寫字母（README.md 除外）
forbidden_chars='[A-Z -]'
exception='README.md VERSION'
```

---

## 7. 遷移計畫

### 7.1 遷移優先級

| 優先級 | 遷移項目 | 影響範圍 | 依賴 |
|--------|---------|---------|------|
| **P1.1a** | kebab-case 目錄 → snake_case | 4 目錄 | 需更新 Go 代碼引用 |
| **P1.1b** | 每日數據檔案重命名 | ~100+ 檔案 | 需更新寫入程式碼 |
| **P1.1c** | Session 目錄格式 | ~80 目錄 | 需更新讀取程式碼 |
| **P1.1d** | 平坦檔案 → 階層結構 | 15 檔案 | Go 代碼 + 設定引用 |
| **P1.3** | 備份檔案移至 `backups/` | 1 檔案 | 新增 `.gitignore` |
| **P2.1** | 原子寫入統一 | 所有寫入路徑 | 功能變更 |

### 7.2 kebab-case 目錄遷移對照表

| 現有名稱 | 新名稱 |
|---------|--------|
| `branch-protection-snapshots/` | `branch_protection_snapshots/` |
| `constraint-mutations/` | `constraint_mutations/` |
| `mutation-briefs/` | `mutation_briefs/` |
| `parameter-snapshots/` | `parameter_snapshots/` |

遷移腳本（`scripts/migrate/data_naming_cleanup.sh`）將：

1. 自動找出所有 kebab-case 目錄
2. 搜尋 Go 源碼中的所有引用並更新
3. 執行檔案系統重新命名
4. 驗證一致性

### 7.3 每日數據檔案遷移範例

```bash
# 遷移前                             →  遷移後
# =================================================================
# macro/2026-04-12.json             →  macro/20260412_macro.json
# margin/20260420_margin.json       →  margin/20260420_margin.json   ← 已正確
# capital_flow/20250519.json        →  capital_flow/20250519_capital_flow.json
```

#### 遷移偽代碼

```bash
#!/bin/bash
# scripts/migrate/rename_daily_files.sh
# 將所有每日數據檔案遷移至 YYYYMMDD_descriptor.json 標準格式

# 1. macro/: 2026-04-12.json → 20260412_macro.json
for f in data/state/macro/*.json; do
    date_part=$(echo "$f" | grep -oE '[0-9]{4}-[0-9]{2}-[0-9]{2}')
    new_date=$(echo "$date_part" | tr -d '-')
    mv "$f" "data/state/macro/${new_date}_macro.json"
done

# 2. capital_flow/: 20250519.json → 20250519_capital_flow.json
for f in data/state/capital_flow/*.json; do
    base=$(basename "$f" .json)
    mv "$f" "data/state/capital_flow/${base}_capital_flow.json"
done
```

### 7.4 Session 目錄遷移

```bash
# session-20260602-daily/ → session_20260602_daily/
for dir in data/state/sessions/session-*-daily/; do
    new_dir=$(echo "$dir" | sed 's/^session-/session_/; s/-daily$/_daily/')
    mv "$dir" "$new_dir"
done
```

### 7.5 備份遷移

```bash
# recommendation_outcomes.jsonl.backup.20260414062052
# → data/state/backups/recommendation_outcomes.jsonl.backup.20260414062052
mkdir -p data/state/backups
mv data/state/recommendation_outcomes.jsonl.backup.* data/state/backups/
```

---

## 8. 正確/錯誤範例速查表

### 8.1 目錄命名

```
✅ data/state/capital_flow/
✅ data/state/tsmc_revenue/
✅ data/state/swarm_training/
✅ data/state/branch_protection_snapshots/
❌ data/state/branch-protection-snapshots/
❌ data/state/CapitalFlow/
❌ data/state/branch protection/
❌ data/state/capital-flow/
```

### 8.2 每日數據檔案

```
✅ 20260412_macro.json
✅ 20260420_margin.json
✅ 20250519_capital_flow.json
✅ 20260602_export.json
❌ 2026-04-12.json
❌ 2026-04-12_macro.json
❌ 20260420margin.json
❌ 11501_export.json
❌ export_20260412.json
```

### 8.3 非日期檔案

```
✅ baseline_policy.json
✅ darwinian_weights.json
✅ darwinian_history.jsonl
✅ recommendation_outcomes.jsonl
✅ channel_health.json
✅ simulation_state.json
❌ baseline-policy.json
❌ BaselinePolicy.json
❌ darwinian_history.json            ← 應為 .jsonl
❌ phase3_metrics.JSON               ← 大寫副檔名
```

### 8.4 備份檔案

```
✅ data/state/backups/recommendation_outcomes.jsonl.backup.20260414062052
✅ data/state/backups/baseline_policy.json.backup.20260602143000
❌ data/state/recommendation_outcomes.jsonl.backup.20260414062052
❌ data/state/backup_recommendation_outcomes.jsonl
❌ data/state/backups/backup_20260414.jsonl
```

### 8.5 Session 目錄

```
✅ session_20260602_daily/
✅ session_20260101_daily/
❌ session-20260602-daily/
❌ Session_20260602_Daily/
❌ 20260602_daily/
```

### 8.6 Replay 檔案

```
✅ tw_combined.jsonl
✅ twse_20260402.jsonl
✅ atlas_combined_2024_2026.jsonl
✅ merged.csv
✅ tw_extended_90days.csv
❌ TW_COMBINED.jsonl
❌ tw-combined.jsonl
❌ tw_combined.json             ← 實際為 JSONL 卻用 .json
```

---

## 9. 例外清單

以下檔案名稱為已知例外，有明確的正當理由：

| 檔案名稱 | 例外原因 | 替代方案 | 狀態 |
|---------|---------|---------|------|
| `README.md` | 工具強制（GitHub、IDE 自動辨識） | 無 | 永久例外 |
| `VERSION` | 工具慣例（無副檔名版本檔案） | 無 | 永久例外 |
| `Dockerfile` | 工具強制 | 無 | 永久例外 |
| `SKILL.md` | 工具強制 | 無 | 永久例外 |
| `CLAUDE.md` | 工具強制 | 無 | 永久例外 |
| `MAKEFILE` | 工具強制 | 無 | 永久例外 |

**新增例外必須**：
1. 在此表格中記錄
2. 提供正當理由
3. 經 PR review 批准
4. 附帶 CI 例外註冊（`validate_data_naming.sh` 中的白名單）

---

## 10. 與現有規範的關係

### 10.1 `developer-guide.md`

`developer-guide.md` §檔案命名指定 `snake_case` 用於「所有新檔案：`my_module.go`, `data_export.json`」。本文件擴展該規範至 `data/` 目錄內的**所有**檔案與目錄，並補充日期格式、JSONL 區分、備份管理等數據特定規範。

### 10.2 `data-architecture.md`

`data-architecture.md` 描述**資料儲存架構**（在哪裡、如何讀寫）。本文件描述**資料命名規範**（叫什麼、怎麼叫）。兩者互補，不相衝突。

### 10.3 `.gitignore`

`.gitignore` 排除 `/data/state/` 和 `/data/replay/*.jsonl` 及 `/data/replay/*.csv`。命名規範的強制執行由 CI 腳本（`validate_data_naming.sh`）完成，不依賴 git 追蹤。

### 10.4 `AGENTS.md`

`AGENTS.md` 高危陷阱表中「JSON tag 大小寫錯誤」提及 `json:"FactorScores"` → `factor_scores` 的案例。本文件的 snake_case 命名規則與 JSON tag 的 snake_case 慣例一致。

---

## 11. 強制執行

### 11.1 CI 檢查

```bash
# 將在 P0.4a 實現的 CI 腳本
bash scripts/ci/validate_data_naming.sh
```

檢查項目：

1. 所有 `data/state/` 目錄名稱符合 `snake_case`（無連字符、大寫、空格）
2. 每日數據目錄中的所有 `.json` 檔案符合 `YYYYMMDD_descriptor.json`
3. `.jsonl` 檔案不為 JSON 陣列格式
4. 備份檔案位於 `backups/` 子目錄中
5. 無平坦 `backup` 檔案在 `data/state/` 根目錄

### 11.2 Pre-commit Hook

```bash
# .git/hooks/pre-commit 附加檢查
# 僅在 data/ 下的檔案變更時觸發
if git diff --cached --name-only | grep -q '^data/'; then
    bash scripts/ci/validate_data_naming.sh --staged-only
fi
```

### 11.3 例外覆蓋

CI 腳本支援 `.data_naming_exceptions` 檔案（位於 `data/` 根目錄），列出永久豁免的檔案路徑（每行一個 glob 模式）。此檔案本身受 git 追蹤，其變更需 PR review。

---

## 12. 文檔維護

本文件是資料治理三部曲（P0.3）的一部分：

| 編號 | 文件 | 狀態 |
|------|------|------|
| P0.3a | `data-naming-convention.md`（本文件） | ✅ 完成 |
| P0.3b | `DATA_STORAGE_POLICY.md` | ⏳ P0.3b |
| P0.3c | `DATA_SCHEMA_STANDARD.md` | ⏳ P0.3c |

### 更新流程

1. 提出變更 PR，包含 `docs/data-naming-convention.md` 修改
2. 若新增例外，同時更新 `data/.data_naming_exceptions`
3. CI 檢查：`validate_data_naming.sh` 確保修改後的規範與現有檔案一致（或標記待遷移項目）

---

*文件版本: 1.0*  
*最後更新: 2026-06-02*  
*維護者: Atlas-Go 資料治理工作組*  
*參考: .omo/audit/2026-06-02-p0-2-root-cause-analysis.md（內部）FG-1, FG-2*
