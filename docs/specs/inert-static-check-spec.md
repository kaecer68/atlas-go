# inert 閉環靜態檢查 — 規格

| 項目 | 內容 |
|---|---|
| 文件角色 | 「有寫入、無消費」缺陷（inert 閉環）的**自動閘門**規格：判定規則、allowlist 機制、誤報出路、以及本檢查**做不到**什麼 |
| 狀態 | v1（2026-09-25，issue [#1944](https://github.com/kaecer68/atlas-go/issues/1944) 建議 2） |
| 實作 | [`../../scripts/ci/check_inert_closure.py`](../../scripts/ci/check_inert_closure.py)（純 stdlib Python3）＋ wrapper [`../../scripts/ci/check_inert_closure.sh`](../../scripts/ci/check_inert_closure.sh) |
| 機器 allowlist | [`../../scripts/ci/inert-baseline.json`](../../scripts/ci/inert-baseline.json) |
| 敘事登記表（人看的） | [`../reference/inert-registry.md`](../reference/inert-registry.md)（由 #1944 稽核流程維護；本檔不重複其內容） |
| 回歸測試 | [`../../tests/scripts/test-inert-closure.sh`](../../tests/scripts/test-inert-closure.sh)（hermetic，自帶 fixture） |

## 1. 為什麼要做

`#1944` 的核心教訓：atlas-go 反覆出現**同一家族**缺陷 —— 程式碼「有寫入、無消費」的靜默失效：

- config 有宣告但全 repo 沒有 reader（例：`industry.max_daily_weight_change` 是假風控）；
- writer 有 setter/registrar 但沒有 production consumer（例：`sectorallocation.RegisterPolicyConsumer` 只有測試呼叫）；
- 狀態欄位硬寫 `applied=true` 卻沒有消費證據。

這類缺陷不會讓 build 或測試變紅，只能靠人工盤查發現，然後又被遺忘。本檢查把「盤查」變成
**每次 PR 自動執行**的閘門：新增的 inert 閉環必須接線、明示豁免（附理由）、或登記進 allowlist。

## 2. 四類檢查與判定規則

掃描範圍（production）：`internal/**`、`cmd/**` 的 `.go` 檔。其他目錄（`tests/`、fixtures、前端 embed
檔）**不視為 production**，因此不會被當成 consumer 證據，也不會被當成違規來源。

`_test.go` 的角色：可以當「有測試」的證據（決定 writer 是 `test-only` 還是 `dead`），
但**不能**當作 production consumer。

### 2.1 `config-inert` — config 旗標有 reader

輸入：`configs/parameters.json` 展平後的**葉參數**（`{"value": ...}` 的節點；`version` / `updated_at`
這類信封欄位排除）。每個葉參數以「最長路徑前綴」對到 `internal/config` 的 struct JSON tag。

**路徑 → Go 欄位的解析**（先做，因為它決定後面比對誰）：以 `ParametersConfig` 為根，沿 struct 巢狀結構
逐段解析 json tag。這樣才不會把 `sector_constraints_carry_trade_unwind` 這種**多個 struct 共用同一 tag**
的參數配到錯的欄位（例：配到 `Risk` 的欄位而誤判 `drawdown.*` 沒有 reader）。解析不到（或只解析到容器）
時退回扁平 tag 比對，並在輸出標明「路徑深於 Go 巢狀結構」。

一個葉參數視為**有 reader**，只要滿足任一條：

| # | 證據 | 說明 |
|---|---|---|
| R1 | 非測試 production 程式碼引用對應 Go 欄位（`\bField\b`，**不含 `internal/config` 內部**） | 直接的欄位讀取（`cfg.X.Y.Value`） |
| R2 | 讀該欄位的 accessor（`internal/config` 內 `Get*`/`Is*` 且 body 讀到該欄位）有非測試 production 呼叫端 | 「getter 存在但沒人呼叫」仍算 inert |
| R3 | 參數的**展平鍵**（`a_b_c`）出現在 `internal/config/param_table.go` 的 key 集合 | parameters API 的 by-name get/set 介面 |
| R4 | **完整路徑字串**（`a.b.c`）以字串字面量出現在非 `internal/config` 的 production 程式碼 | 以 dotted path 取值的介面 |

四條都不成立 → 違規。找不到對應 Go 欄位的葉參數另有專屬訊息（config 有宣告、沒有型別可讀）。

**為什麼 R1 不含 `internal/config` 內部**：宣告（`Field Type \`json:...\``）、預設值（`defaults_*.go` 的
`Field: ParameterMetadata{...}`）、合併（`parameters_merge*.go`）、驗證（`parameters_validate.go`）、
by-name 表（`param_table.go`）都在 package 內、都不代表「參數真的影響行為」。因此**在 package 內被讀
（含 `ToConfig` 轉換器）一律不算 reader** —— 這是刻意的保守選擇，代價是少數誤報（見 §5）。

**容器語意**：若解析只走到容器欄位（例如 JSON 以 map 為容器：`sector_allocation.base_weights.consumer`），
容器型別是 `map[...]` → 讀容器等於讀所有項目，容器證據有效；容器型別是 struct/純量 → 容器「被讀」
不代表這一葉被讀，不可當 reader 證據。

> **刻意不採用的弱證據**：單段 key（`"surge_boost"`）的字串出現。實測 `internal/orchestrator/plugin_style.go`
> 把同一字串當**顯示標籤**用（`b.add("surge_boost", ...)`），但值其實來自硬編碼常數 —— 這種比對會把
> 真 inert 誤判成「有 reader」。`internal/config/parameters_validate.go` 的錯誤訊息字串同理。

### 2.2 `writer-no-consumer` — writer 有 consumer

候選 writer：production 非測試 `.go` 中，**名稱以 `Set`/`Register`/`Emit`/`Record`/`Publish`/`Store`/
`Save`/`Write`/`Apply`/`Enable`/`Update`/… 開頭**（完整清單見程式常數 `WRITER_PREFIXES`）的 exported
函式／方法。

機械排除：**零參數**的候選（`RegisteredXxx()` 這種是 reader，不是 writer）。

consumer 證據：任何非測試 production 檔案出現 `Name(` 呼叫點（排除宣告行本身），或名稱以字串字面量
出現（依名稱註冊／反射）。沒有 consumer → 違規，並分兩級：

| tier | 意義 |
|---|---|
| `test-only` | 只有 `_test.go` 呼叫 → producer 存在、production consumer 缺席（#1944 家族的核心樣態） |
| `dead` | 連測試都沒呼叫 → 疑似死碼（含「註解說 main.go 會接，實際上沒接」的案例） |

### 2.3 `claim-not-derived` — 狀態欄位由證據推導

claim 欄位 = JSON key 命中 claim set 的結構欄位：`applied`、`calibrated`、`wired`、`enforced`、
`consumed`、`effective`，或 `*_{上述}`（例如 `rows_calibrated`）。

違規：production 非測試程式碼對該欄位（或其 JSON key）寫入**字面值真值**：

- 複合字面值：`Receipt{Applied: true}`
- 選擇子賦值：`out.Calibrated = true`
- JSON map：`"applied": true`

也就是「對外宣稱已生效」卻沒有推導過程。**本檢查不做守衛分析**：若某個站點的 `true` 其實被前面的
early-return／門檻守住（例：`internal/forecast/foreign_forecast.go` 的 `Calibrated: true` 由樣本數
與命中率兩道門檻推導），那是**已知誤報**，用行內豁免或 baseline 明示（見 §4）。

### 2.4 `claim-not-consumed` — 狀態欄位有 consumer

同一個 claim 欄位集合。違規條件：非測試 production 程式碼**有寫入**（`x.Field = ...`、`Field: ...`
複合字面值、`x.Field++`、`"key": ...`）但**沒有任何讀取點**（`x.Field` 且後面不是賦值運算子），
且該 JSON key 沒有以字面量出現在 production 程式碼。即「有 producer、無 consumer」的狀態宣稱。

### 2.5 額外違規：`inert-ok-missing-reason`

`// inert-ok` 註解**沒有寫理由**時，豁免本身即違規（理由必填的強制手段）。

## 3. 執行與接入

```bash
make inert-check            # 檢查 + 回歸測試（必跑，見下方 CI 接入）
make inert-check-update     # 把新違規寫進 baseline（reason 留 TODO，需人工填寫）
bash scripts/ci/check_inert_closure.sh --json          # 機器可讀輸出
bash scripts/ci/check_inert_closure.sh --root tests/fixtures/inert-closure   # 掃 fixture
python3 scripts/ci/check_inert_closure.py --help
```

`make inert-check-update` 只會**新增**條目（並把 `reason` 填成 `TODO`），不會刪除 stale 項目。

接入點（兩者都在 PR 上真的執行）：

- GitHub Actions：[`../../.github/workflows/quality.yml`](../../.github/workflows/quality.yml) 的
  `inert-closure` job（無 path filter → docs/scripts-only PR 也會跑；含回歸測試）。
- 本地／pre-push：`make ci-gate`（因此 `make pre-push` 也涵蓋）；另外 `make ci` 的 glob 會一併抓到
  `scripts/ci/check_inert_closure.sh`。

效能（2026-09-25 量測，MacBook M 系列，2269 個 Go 檔）：**約 3–4 秒**（含 Python 啟動）。
設計上索引一次性建立（詞法掃描 2 趟），避免 per-name 全 repo regex（初版是這樣的，耗時 134 秒）。

## 4. allowlist 與誤報出路（三條路，理由都必填）

### 4.1 行內豁免（推薦給 Go 程式碼）

```go
// inert-ok[writer-no-consumer]: test seam，production 走預設 registry；見 X 追蹤
func SetNowFn(fn func() time.Time) { nowFn = fn }
```

- 位置：宣告行本身，或**往上 3 行內**（Go doc comment 位置）。
- 範圍：`[<check-id>]` 指定單一檢查；省略（`// inert-ok: 理由`）代表全部檢查。
- **必須有理由**；空白理由視為違規（`inert-ok-missing-reason`）。
- 針對 claim 類，行內豁免的判定是**逐站點**（以違規的 file:line 為準），因此可以只豁免
  「被守衛的那一個站點」，而不是整個欄位。

### 4.2 baseline allowlist（推薦給 config 參數，JSON 不能寫註解）

`scripts/ci/inert-baseline.json`：

```json
{
  "version": 1,
  "entries": {
    "config-inert": {
      "industry.max_daily_weight_change": {
        "reason": "N-C1 假風控：config 有宣告，全 repo 無 reader；待 #1944 batch 3 接線或移除",
        "evidence": "internal/config/parameters.go:823"
      }
    },
    "writer-no-consumer": {
      "internal/foo/bar.go:SetThing": {
        "reason": "test seam；production 走預設實作",
        "tier": "test-only",
        "evidence": "internal/foo/bar.go:42  test-files=1"
      }
    }
  }
}
```

- 欄位語意：`reason`（必填，人看得懂、可自行驗證）＋ `evidence`（宣告位置，自動填）＋
  `tier`（**機械**分級：僅 `writer-no-consumer` 有，`dead`／`test-only`，由檢查自動維護）＋
  `class`（**人工**分類：`legacy-inert`／`test-seam`／`dead`／`metadata-only`／`consumed-indirectly`／
  `guarded`，供後續稽核排序；`--update-baseline` 不會寫入，由人補）。
  `tier` 與 `class` 是兩個不同維度，可以不一致（例：機械 `test-only`、人工判為 `dead`）。
- key 的格式：`config-inert` 用參數路徑；`writer-no-consumer` 用 `檔案:函式名`；
  claim 類用 `json_key@欄位名`。**刻意不含行號**（避免檔案位移造成假 stale）。
- `reason` 必填且不可用 `TODO` 開頭，否則違規（`baseline-reason-missing`）。
- 更新：`make inert-check-update` 會把新違規追加並把 reason 填成 `TODO: ...`；**填完才 commit**。
- **stale 語意**：baseline 項目不再命中時只印警告、**不讓 CI 變紅**。理由：跨 PR 耦合 ——
  另一個人把 inert 項接線後，不應該因為「忘了刪 baseline 一行」而被擋；stale 清單由維護者在
  例行的 `make inert-check` 輸出中人工複核後刪除。

### 4.3 接線（最好的出路）

讓 writer 有 production consumer、參數有 reader、狀態由消費證據推導。baseline 不是「永遠的藉口」，
只是「已登記、尚未修」的清單。

## 5. 這個檢查**不能**取代什麼（誠實邊界）

1. **Runtime inert**：consumer 存在但資料恆空（上游 producer 沒產資料）、旗標翻了但行為不變、
   指標永遠 0 —— 靜態檢查看不到。既有例子：`industry.cycle_calibration` 全 0 導致 metrics 恆空。
2. **跨服務／跨機器 inert**：consumer 在別的服務（前端、Hermes、LLM）或另一台機器的程式裡。
3. **語意正確性**：`Applied` 由「消費證據」推導，但那個證據本身可能是假的（例：`appliedCount++`
   不看 `SetParameter` 回傳值）。本檢查只管「有沒有讀寫關係」，不管「寫進去的值對不對」。
4. **守衛分析／資料流**：不追蹤「這個 `true` 是不是被前面的門檻守住」，所以 `claim-not-derived`
   會對「條件式硬寫」誤報（§2.3）。
5. **動態註冊**：以反射、以 `map[string]func` 常數表、以字串名稱註冊的呼叫端，只能靠
   「名稱以字串字面量出現」這條弱證據（§2.2），仍可能漏。實例：`RegisterApplier("finmind", ...)`
   （`cmd/atlas/main.go:677`）這類**函式值註冊**在 baseline 標為 `consumed-indirectly`。
6. **以名稱比對，不具型別感知**：同名欄位／函式出現在別的型別時，會被當成 reader／consumer 證據
   （false negative，檢查偏寬鬆）。已知例子：`SourceType`、`EvidenceQuality` 這類常見欄位名。
7. **`internal/config` 內的讀取一律不計**（見 §2.1）：因此「只在 `ToConfig` 轉換器被讀」的參數會誤報。
   已登記實例：`engine.drawdown.sector_constraints_sector_rotation`（`parameters.go:1927` 的轉換器讀取；
   該欄位名 `...Rotate` 與其他結構的 `...Rotation` 不一致，另待正名）。
8. **介面/建構子注入**：`New(deps)` 形式的注入不算 writer，不在此檢查範圍（只有名稱前綴命中的
   函式才是候選）。

因此：本檢查是**必要不充分**（necessary, not sufficient）。inert 登記表
（[`../reference/inert-registry.md`](../reference/inert-registry.md)）與 runtime 稽核仍需人工流程。

## 6. 誤報處理 SOP

1. 先判斷是「檢查規則的系統性誤報」還是「個案」。
2. 系統性誤報 → 改規則（並在 §2 或 §5 更新說明），不要用 baseline 掩蓋；改規則後重跑
   `make inert-check` 並更新 baseline。
3. 個案 → 行內豁免（Go）或 baseline（config），reason 要寫到「讀者能自己驗證」的程度
   （含 file:line 或機制）。
4. 修正後 `bash tests/scripts/test-inert-closure.sh` 必須 4/4 通過。

## 7. 驗收證據（2026-09-25，本檢查落地 PR）

| 情境 | 命令 | 結果 |
|---|---|---|
| 故意造的 inert 案例（negative fixture） | `python3 scripts/ci/check_inert_closure.py --root tests/fixtures/inert-closure-negative --baseline /tmp/none.json` | exit 1；抓到 `neg.unread_flag`、`SetDeadWriter`、`applied@Applied`、`inert-ok-missing-reason` |
| 修好（fixture 含 baseline + 正確寫法） | `python3 scripts/ci/check_inert_closure.py --root tests/fixtures/inert-closure` | exit 0；wired writer／annotated writer／由證據推導的 claim 都不誤報 |
| 既有 main（allowlist 生效） | `bash scripts/ci/check_inert_closure.sh` | exit 0；189 筆既有項由 baseline 覆蓋（config 57／writer 130／claim 2），耗時 ~2.5s |
| 理由必填 | `--baseline <reason=TODO 的副本>` | exit 1（`baseline-reason-missing`） |
| 回歸測試 | `bash tests/scripts/test-inert-closure.sh` | 4/4 PASS（hermetic，不碰 git／production／Go toolchain） |

baseline 的 `class` 分佈（189 筆，由人工逐項判讀，見 PR）：`legacy-inert` 114、`test-seam` 38、
`dead` 23、`metadata-only` 7、`consumed-indirectly` 6、`guarded` 1。
