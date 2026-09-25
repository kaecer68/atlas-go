# JEV-EVAL-FRAMEWORK — Jev 預測力評估框架（可重用、shadow-only）

| 項目 | 內容 |
|---|---|
| 文件角色 | **規範（normative）**：任何「Jev 對某個預測目標是否有用」的評估都必須走本框架；同時是 Stage 2/3 的擴充介面契約 |
| 狀態 | v1（2026-09-25，issue [#1966](https://github.com/kaecer68/atlas-go/issues/1966)） |
| 上位規範 | [`JEV-USAGE-CONTRACT.md`](JEV-USAGE-CONTRACT.md)（§1 傳輸、§2 判斷設計、§3 門檻、§4 評估紀律、§5 生產整合） |
| 實作 | `scripts/jev_eval/`（Python 框架）+ `cmd/experimental/jev-eval-panel/`（Go：canonical 口徑的 GT/特徵匯出） |
| Stage 1 結果 | [`JEV-EVAL-STAGE1-INDUSTRY-L1.md`](JEV-EVAL-STAGE1-INDUSTRY-L1.md) |
| 已量測的成本 | 台股產業層 E0：3240 cases / 180 requests = **$0.0456**（1,085,311 input tokens @ $0.042/Mtok） |

---

## §0 這個框架回答什麼問題

> **「Jev 的判斷對〈某個預測目標〉是否攜帶資訊，且相對該目標既有的 baseline 是否帶來增量？」**

它刻意**不**回答「Jev 準不準」（沒有單一準確率這種東西），而是產出五件事：

1. 在**已校準門檻**下的 precision / recall / coverage；
2. **threshold-free 的判別力**（AUC，pooled 與「同日跨標的排名」兩種）；
3. 相對 baseline 的 **ΔAUC / Δprecision / filter lift**，附**以日為 cluster 的 paired bootstrap CI**；
4. **樣本數與成本**；
5. **分級結論**（`已驗證` / `已更正` / `未驗證`）+ 未解的洩漏與資料限制。

---

## §1 為什麼放在 `scripts/jev_eval/`（而非 `cmd/experimental/`）

| 理由 | 說明 |
|---|---|
| Jev 只能經 Python 呼叫 | 本 repo 唯一認可的呼叫方式是 `scripts/jevkit.py`（§0 契約），且 CI（`scripts/jev-contract-check.py`）是 AST 檢查 `.py`。若把評估器寫成 Go 二進位，最終仍得 shell out 到 Python，白多一層跳轉與一個失敗面。 |
| 評估是離線分析、不是產品路徑 | 框架**不得**被產品 import（shadow-only）。`cmd/` 是本專案「可執行交付物」的位置；Go 端只保留**需要 canonical 口徑**的部分（GT / 特徵匯出）。 |
| 口徑的單一真相在 Go | 扣成本命中定義與 Wilson CI 在 `internal/stockpicker`。為避免「另立口徑」，GT 由 Go 端以既有函式產生（`stockpicker.NetHit` → `IndustryWinRate`），Python 只**消費**它。 |

> 一句話：**口徑在 Go（既有），呼叫與統計在 Python（新增）**。兩邊的交界是一個 JSONL panel，因此可離線重算、可稽核、可單獨測試。

---

## §2 契約：一個目標 = 一個 task spec

```python
class TaskSpec:
    name: str          # CLI --spec
    version: str
    defaults: Mapping[str, Any]
    def build(self, args) -> SpecBuild   # 必須決定性（同輸入 → 同 fingerprint）
```

`SpecBuild` 由兩張表組成，**這是本框架最重要的一個切分**：

| 物件 | 內容 | 誰看得到 |
|---|---|---|
| `Request` | `state`（PIT 事實）+ `questions`（1..N 題 fan-out） | 送給 Jev |
| `Case` | `gt`（真值）+ `baselines`（baseline 分數）+ `group_id`（cluster 鍵） | **只有評分層** |

- state builder 只拿到 `CaseView`（`case_id` / `group_id` / `meta`），**structurally 看不到 gt**（`scripts/jev_eval/selfcheck.py::test_case_view_hides_ground_truth` 守住）。
- 一題一判斷（§2.1 契約）：判斷條件寫在 `instructions`，`noul` 另附 `criteria{true,false}` 讓邊界明確；題目 id 不會送給模型。
- 同一天的 N 個標的**合成一個 request**（fan-out）：官方與本方實測皆顯示批次遠便宜於逐題，且同日跨標的互相比較正是產業輪動的真實用法。
- `group_id` 用**交易日**（不是標的）：同日所有標的的答案來自同一個 request，彼此相關，統計上必須以日為 cluster，否則 CI 會假性顯著。

---

## §3 Run artifacts 與可離線重算

```
<out-dir>/
  manifest.json     # spec/版本/model/spec_args/requests_fingerprint/input_fingerprint/git rev/價格
  requests.jsonl    # 每次呼叫一列：fingerprint, ok, model, tokens, latency_ms, attempts, answers
  cases.jsonl       # 每個 case 一列：gt + baselines + meta（PIT 特徵與 GT 區塊都留檔供稽核）
  metrics.json      # 指標（由 requests.jsonl + spec 重建，不重打 API）
  report.md         # 人看的報告（含分級結論）
```

- **JSONL 就是快取**：`(request_id, repeat)` 的 fingerprint 相同就重用，`--force` 才重打。
- `score` / `report` 子命令**完全不呼叫 API**：指標可反覆重算、可換門檻重看，成本為零。
- `--offline` 明確禁止呼叫（CI/離線環境可用）。
- fail-open（§5.1）：任何傳輸或 API 錯誤記 `ok=false` 並繼續；該 request 的 case 分數是 `None`（**不是 0.0**——「沒答」與「答 0」是不同證據）。

---

## §4 指標定義（產業層的 base rate < 50%，定義必須寫死）

| 指標 | 定義 | 為什麼要它 |
|---|---|---|
| `base_rate` | `P(gt=1)` | 「一律看多」策略的 precision；任何 lift 宣稱的地板 |
| `precision` / `recall` | `P(gt=1 \| call up)` / `P(call up \| gt=1)`，含 Wilson 95% CI | 操作點（operating point）品質 |
| `coverage` | 做出正向呼叫的比例 | 小 coverage 的高 precision 沒有意義（見 §5 可靠性閘門） |
| `AUC` (pooled) | 分數對 gt 的秩判別力，0.5 = 無資訊 | threshold-free；不受門檻選擇影響 |
| `AUC (within group)` | 每個交易日**日內**排名 AUC 的平均（同日單一類別的日子跳過，並報可用日數） | 產業輪動真正要問的是「同一天哪個產業較強」；pooled AUC 會把市場整體漲跌混進來 |
| `ΔAUC` / `Δprecision` | Jev − baseline（paired，同一 cluster 重抽） | 「有沒有增量」 |
| `filter lift` | 在 baseline 已看多的子集裡，只留「Jev 也看多」後的 precision 增益，並附保留比例 | 平台已經在用 baseline，真正要問的是**過濾增益**而非取代 |
| `Brier` / `ECE` / reliability table | 機率本身的校準度 | 若 Jev 機率系統性偏移（實測偏保守），門檻必須據此校準，而不是照抄 0.5 |

不確定性一律用 **cluster bootstrap（以 `group_id` 重抽，預設 1000 次、固定 seed）**；paired 比較在同一重抽樣本上同時算兩邊。

---

## §5 門檻與分級紀律

### 5.1 門檻（§3 契約）

- 門檻**只用校準集**（時間序前段 `--calib-fraction`，預設 0.6 的交易日）算，呼叫 `jevkit.calibrate_threshold(pairs, min_precision=...)`。
- 校準集上沒有任何門檻達標 → 記 `fallback` 原因並回退 0.5；報告必須顯示這件事（本次 E0 即為此案）。
- **操作點可靠性閘門**：正向呼叫數 < `MIN_UP_CALLS`(30) 時，precision/recall/Δprecision/filter lift 在報告中標 `n/a*` 並附警語——避免用 1 個呼叫的 precision 1.0 當證據。

### 5.2 分級（§4.5）

`grade()` 依序套用：

1. **覆蓋閘門**：held-out cases < `--min-eval-cases`(200)、或 answered fraction < 0.9、或無 held-out AUC → `未驗證`（並列出未過的閘門）。
2. **資訊閘門**：pooled AUC 的 CI 下界 > 0.5 → 有資訊；CI 上界 < 0.5 → `已更正`（顯著**差於**隨機，也是一種結論）；否則 `未驗證`。
3. **增量閘門**：有資訊時，檢查最佳 baseline 的 ΔAUC CI 下界是否 > 0；有 → 明說「且優於 baseline X」，沒有 → 明說「有訊號但未優於 baseline」。
4. **洩漏上限（leakage cap）**：洩漏探針（§6）顯示模型能回憶該序列時，結論降為 `未驗證` 並說明原因。

> 主要指標**預先指定**為 pooled AUC；within-group AUC 為次要並同時報告。**不得**在看到結果後改選指標——那正是本次立法要消滅的行為。

---

## §6 洩漏探針（backtest 特有風險）

Jev 為 2026-09-10 釋出的模型（`jev-1.13.0`）。任何 2021 視窗的回測都可能落在其訓練語料內，於是「預測力」可能只是「記憶」。

框架因此要求 spec 提供 **leakage mode**：同一批標的、同一段窗口，但問的是**結束於 as-of 日的過去 5 個交易日**，且 state **刻意不含任何價格特徵**（只有產業 id/名稱與日期）。此時唯一可能的答題來源是**先驗知識**。

- 探針的 AUC 若顯著 > 0.5 → 模型確實記得該序列 → §5.2 第 4 條把主結論降級。
- 探針 ≈ 0.5 → 至少對這個序列沒有可偵測的記憶；主結論不能歸因於記憶。
- 探針本身也是「輸入是否含資訊」的反向檢查（§4.4 教訓）。

---

## §7 非決定性與成本

| 項目 | 實測/規範 |
|---|---|
| 非決定性 | Jev 是機率原語。`--repeat N` 會帶不同 `uid` 重送並**各自留一列**；`cli.py repeats` 量化一致性。E0 實測（20 個交易日 × 18 標的 × 3 次）：完全相同僅 **5.8%**，分數差均值 0.022、p95 0.05、最大 0.08。 |
| 分數聚合 | `--repeat-aggregate first\|mean`（預設 `first`，主要結果用首次；`mean` 供敏感性分析）。 |
| 成本 | 只算 input token；本框架以 **$0.042/Mtok** 估（`experiments/008-jev-systemone` 的實測值），`--price-per-mtok` 可覆寫；`metrics.json.cost` 一定附上。 |
| 延遲 | TTFB 實測 ~284ms（台灣），端到端 265–663ms；本框架用 `--concurrency` 並行（預設 4）控制 wall time。 |
| 批次 | 一天的多題合成一個 request（fan-out）：本次 E0 為 180 requests / 3240 cases。 |

---

## §8 如何新增一個目標（Stage 2/3 的擴充介面）

新增一個預測目標**不需要改框架**，只需要一個模組 + 五件東西：

1. **可程式重生成的 GT**：必須能用一行指令重跑（例：`go run ./cmd/experimental/jev-eval-panel ... -out panel.jsonl`）。禁止手寫標註（§4.1 教訓：行號錯位的 GT 讓 90% 變 8%）。
2. **GT 口徑要引用既有程式路徑**（不可另立）：扣成本命中用 `stockpicker.NetHit`、聚合與 CI 用 `IndustryWinRate` / `WinRate` / `WilsonScoreInterval` / `CalibrationStatusFor`。
3. **baseline 要具名**：指出平台現有的哪一條路徑（模組 + 函式 + issue），並在沒有 PIT 歷史資料時**誠實報 unavailable**，不可用近似品冒充。
4. **cluster 鍵**：通常是交易日；跨標的/跨事件同一天必須同 cluster。
5. **窗口與成本預算**：先算 requests × 平均 tokens × 單價，再決定窗口（E0 的 3240 cases 只需 $0.046）。

spec 放在 `scripts/jev_eval/specs/<name>.py`，於 `cli.py::SPECS` 註冊；`selfcheck.py` 至少要加「PIT 分離」與「決定性」兩項。

---

## §9 CLI 參考

```bash
# 列出可用 target
python3 scripts/jev_eval/cli.py specs

# 產生 GT panel（canonical 口徑；一行指令即可重生成）
go run ./cmd/experimental/jev-eval-panel \
  -dir data/state/sector_index -start 2021-01-04 -end 2021-12-30 -min-history 60 \
  -out /tmp/panel.jsonl -summary-out /tmp/panel_summary.json

# 收集（會呼叫 Jev，shadow-only）、評分、出報告
python3 scripts/jev_eval/cli.py all --spec industry_l1 --out-dir /tmp/run \
  --spec-arg panel=/tmp/panel.jsonl --concurrency 6

# 只重算指標與報告（零成本）
python3 scripts/jev_eval/cli.py report --spec industry_l1 --out-dir /tmp/run \
  --spec-arg panel=/tmp/panel.jsonl --leakage-run-dir /tmp/leak

# 量化非決定性
python3 scripts/jev_eval/cli.py repeats --out-dir /tmp/run

# 離線自檢（CI 用；不連網、不需 key）
python3 scripts/jev_eval/selfcheck.py
```

---

## §10 限制（已知且刻意保留）

1. **本框架不做線上決策**：產物只有檔案；接進產品路徑是 E2，需先過註冊閘門（§5 契約）。
2. **樣本無法解決結構性缺口**：能算的只有「有 PIT 資料的窗口」。資料缺口本身是結論的一部分（Stage 1 因此把本地 2026 合成資料整段排除）。
3. **AUC 不是錢**：本框架量的是判別力與操作點品質，不含部位大小、滑價與資金效率——那屬於策略層回測。
4. **洩漏探針只證明「沒有可偵測的記憶」**，不證明「沒有記憶」；故結論文字必須保留該不確定性。
