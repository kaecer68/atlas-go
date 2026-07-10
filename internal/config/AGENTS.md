# AGENTS.md — internal/config

`internal/config` 管理雙重設定系統：部署用 `Config`（環境變數）與演算法參數用 `ParametersConfig`（JSON + 權威溯源）。

---

## 雙重設定系統

| 系統 | 載入方式 | 用途 | 範例 |
|------|---------|------|------|
| `Config` | `config.Load()` | 部署設定（路徑、API 金鑰、功能開關） | `ATLAS_WORK_DIR`、`ATLAS_LEDGER_DIR` |
| `ParametersConfig` | `config.LoadParametersConfig()` | 可調校演算法參數（附溯源） | `WeightMax`、`MomentumLookbackDays` |

**禁止**：將部署設定放在 `ParametersConfig` 中，反之亦然。

---

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **`.env` 引號剝除不可見** | `loadEnvFile()` 會去除 `KEY="value"` 和 `KEY='value'` 的引號；遇到 `KEY=va"lue"` 則不會。不要依賴引號做跳脫。 |
| **`ATLAS_ENV_FILE` 靜默覆蓋** | 若設定了 `ATLAS_ENV_FILE`，它會完全取代 `.env` 載入。使用此機制時請在文件中記錄。 |
| **Keychain 後備為無操作** | `envOrKeychain()` 僅委派給 `envOr()` — keychain 整合仍是 TODO。不要把環境變數中的機密視為安全的。 |
| **`ParametersConfig` 靜默回退** | `LoadParametersConfig()` 回傳帶有預設值的 struct（來自 `parameters_defaults.go`），`Validate()` 之後才呼叫。無效的 JSON 檔會靜默回退，不報錯。 |
| **Magic number 繞過參數系統** | 禁止在業務邏輯中硬編碼 `0.3`、`2.5`、`60` 等數值。需加入帶有 `Rationale` 和 `Source` 的 `ParameterMetadata[T]`。參見 `docs/REFERENCE/PARAMETER_SYSTEM.md`。範例：`FallbackPriceTargets` map（per-skill target/stop-loss 乘數，於 `monitoring/service/session.go` 透過 `config.GetParametersConfig()` 讀取）。 |
| **`os.Getenv` 散布（祖父條款）** | 根 `agents.md` 和 `CONSTITUTION.md` 禁止直接呼叫 `os.Getenv`。`config.go` 中的呼叫為祖父條款 — 不要作為新程式碼的參考模式。 |

---

## 常用函數

| 函數 | 用途 |
|------|------|
| `config.Load()` | 載入 `Config`。僅在 main 中呼叫一次。 |
| `config.LoadParametersConfig(path)` | 載入參數。回傳 `*ParametersConfig`，附 `Validate()` 方法。 |
| `config.GetParametersConfig()` | 回傳全域參數 singleton（透過 `sync.Once` 初始化）。 |
| `config.GetReplayDataPath(workDir)` | 解析實際回放資料路徑（env → VERSION 檔案 → 預設值）。 |
