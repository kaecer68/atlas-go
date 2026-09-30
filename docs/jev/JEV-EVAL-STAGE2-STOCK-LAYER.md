# JEV-EVAL-STAGE2-STOCK-LAYER — Jev 對預測是否有用：Stage 2（個股／錢潮層 E0）

| 項目 | 內容 |
|---|---|
| 問題 | 「Jev 的判斷，對**個股（錢潮代理）**的 5 交易日、扣成本後 forward 方向，是否攜帶資訊？在**同一資訊集下**是否比平台規則式訊號更有區辨力？」 |
| 階段 | Stage 2（E0：**shadow 評估，不接任何生產決策路徑**）|
| 框架 | [`JEV-EVAL-FRAMEWORK.md`](JEV-EVAL-FRAMEWORK.md) |
| Issue | [#1968](https://github.com/kaecer68/atlas-go/issues/1968) |
| 日期 | 2026-09-30（★ **協議修訂後**：noul escape 措辭於同日修訂，見 [`JEV-EVAL-FRAMEWORK.md`](JEV-EVAL-FRAMEWORK.md) §「協議修訂政策」；**與 Stage 1／Stage 3 之修訂前結果不可直接比**）|
| 模型 | `jev-1.13.0`（pin 版本 ID，非 alias） |
| **分級結論** | **未驗證**（held-out AUC CI 含 0.5；**增量閘門**亦未通過） |
| 樣本數 | held-out **624** cases（answered **100%**）；探針與 judge 共用 case_id **312** |
| 成本 | **$0.030671**（judge 40 req／653,056 tok ＝ **$0.027428** ＋ 探針 8 req／77,216 tok ＝ **$0.003243**）⇒ ★ **超支披露**：事前估 `screen` ≈$0.0061 ✗ |

> ⚠️ **尺度誠實標註**：本次 judge 實為 **`confirm` 尺度（40 dates × 39 symbols）**，**不是** `screen`（20×20 ✗）——肇因見 §5 缺陷 ①。
> ⚠️ **成本模型**：本層每個 request 要送當日**全部個股**的 `pit`／`baselines` ⇒ 實測 **16,326 tokens/request**；以 request 數估價會**低估 2.7×**。上限已改 **token-aware**（$0.02 → $0.05）。

---

## §1 設計（摘要）

- **spec**：`scripts/jev_eval/specs/stock_layer.py`
  - **有界取樣**：symbols 依 `industry_id` **分層**（每層 2 檔，以 `sha256(seed|symbol)` 排序 ⇒ 決定性且**不偏低代號**）、總數上限 `max_symbols`；dates 於面板範圍內**等距**取樣（保留末點）
  - **成本**：**1 request ＝ 1 個日期**（同日全部個股 fan-out）；`COST_CAP_USD` **token-aware**（估算 tokens × 實價）；**兩階段** `screen`（20×20）／`confirm`（60×40）
  - **洩漏探針**：`mode=leakage` ＋ `probe_step=5`，與 judge **共用 case_id** ⇒ 可直接餵 #2179 的 `paired_auc_margin`
- **面板來源**：`cmd/experimental/jev-eval-sympanel`（**檔案進、檔案出、零 DB 依賴**；`pit` 白名單僅含 t 及以前；`forward`／`backward` 由**價格重算**，與 Stage 1 同尺 `stockpicker.NetHit`）
- **標籤**：`forward.hit` ＝ 下一 `hold_days=5` 交易日、扣 `cost_rate=0.00585` 往返成本後是否上漲
- **判準**：held-out AUC 之 bootstrap CI ＋ **增量閘門**（`auc_margin = AUC(judge) − AUC(probe)`，CI 下界須 > 0；#2179）

## §2 結果

| 指標 | 數值 |
|---|---|
| held-out AUC（judge） | CI **[0.408, 0.5217]** ⇒ **含 0.5** ✗ |
| held-out 樣本 | **624**（answered **100%**） |
| 洩漏探針 AUC | **0.5423**（CI [0.3549, 0.645]）⇒ **未觸發**洩漏上限 |
| 增量 `auc_margin`（judge − probe） | **−0.0407**（CI **[−0.1264, 0.0379]**；n_shared **312**；judge 0.4818／probe 0.5225）⇒ **下界 ≤ 0** ✗ |
| **最終 verdict** | **未驗證**（絕對 AUC 不顯著 ＋ 增量閘門再降級：無法與「記得結局」分離） |

⇒ **個股／錢潮層：Jev 未展現可辨識的增量**（與產業層、事件層**同型**）

## §3 三層彙總

| 層 | 日期 | 結果 | 判定 |
|---|---|---|---|
| Stage 1 產業（canonical L1） | 2026-09-25 | pooled AUC 0.4584 [0.4147, 0.5014]（含 0.5） | **未驗證**（修訂前） |
| Stage 3 事件（排程事件） | 2026-09-25 | AUC 0.3995 [0.3322, 0.4774]（方向為負）；洩漏探針 0.5722 [0.5064, 0.6186] **觸發上限** | **未驗證**（修訂前） |
| **Stage 2 個股／錢潮** | 2026-09-30 | 見 §2 | **未驗證**（**修訂後**） |

## §4 判讀與限制（claim 精度）

- `pit`／`baselines` **內含 momentum／net_buy 兩項平台訊號** ⇒ 本結果的讀法只能是「**同一資訊集下，Jev 的判斷是否比規則式訊號更有區辨力**」，**不可**讀成「Jev 自己找到動能」
- 「**錢潮**」於本層**僅以外資淨買為代理**（未含投信／融資等其他資金勢力）
- `netbuy` 缺日：平台 `stock_flows` 於 **2026-08-28 → 2026-09-09 缺 9 個交易日** ⇒ 以 **TWSE T86（平台 producer 同一端點）** 補齊；caliber 已證：2026-08-27 與 2026-09-29 抓取值與平台 `foreign_net` **88/88 檔逐位元相同**
- **未做** `net_buy=false` 對照（成本翻倍 ⇒ 本階段不做）
- 面板僅涵蓋 **20 個 L1** ⇒ 每層 2 檔 ⇒ 上限 40 檔 ⇒ 本層 `confirm` 實得 **39 檔**。**未來要跑 60×40 必須用涵蓋 ≥30 個 L1 的面板**

## §5 缺陷與修正（已修）

- ① **`stage` 預設在 CLI 路徑不生效** ✗（CLI 先 `merged_args()` 再餵 `build()` ⇒ 「使用者是否真的給了」資訊遺失）⇒ 實跑變成 `confirm` 尺度 ✗
  - 修法：`build(args, *, user_args=None)`（介面擴充、有預設）＋ CLI 三處 build 站點傳入 `user_args`
  - ★ 教訓：**測的介面 ≠ 用的介面** —— 直呼 `build(raw_args)` 的 selfcheck 會過，必須測 **CLI 路徑**（已加 `stage_screen_via_cli_path`，並含「不傳 `user_args` ⇒ 退回 confirm」對照）
- ② **成本上限 request-based 守不住** ✗ ⇒ 改 **token-aware**（`requests × 檔數 × 408 tokens` × `$0.042/Mtok`），上限重校 **$0.02 → $0.05**
- 修正：`0ea40189`（分支 `fix/20260930-stock-layer-token-aware`）

## §6 未來若要再論證（**需新設計，非加大 n**）

- **不建議**「同設計、更大 n」：點估計 **0.46 < 0.5** ⇒ n 變大只會**收緊 CI**，翻盤機率低
- 需要**新設計**：新題型／新標籤（例：用 #2151 的**還原序列**做 forward label）／新 baseline，或 **baseline-free A/B**（回答「Jev 是否帶來平台訊號之外的**新**資訊」）；`confirm` 成本 ≈**$0.0267–0.0411**

## §7 留下的可用資產

- spec **`stock_layer`**（有界取樣／等距日期／token-aware 上限／兩階段／洩漏探針與 judge 共用 case_id）
- 匯出器 **`cmd/experimental/jev-eval-sympanel`**（零 DB 依賴；`pit` 白名單；與 Stage 1 同尺）
- 評分能力 **`paired_auc_margin`**（#2179；本次**實際生效**並降級 verdict）
- 面板與最終報告：`~/workspace/atlas-notes/stage2-panel-20260930/`、`~/workspace/atlas-notes/stage2-final-report-20260930.md`
