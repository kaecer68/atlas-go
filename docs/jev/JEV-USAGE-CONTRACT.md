# JEV-USAGE-CONTRACT — 在 a2a 生態使用 Jev（TypeSafe System One）的規範

> 本檔是**規範（normative）**，不是筆記。所有呼叫 Jev 的程式碼（本 repo、hermes 外掛、
> LiteLLM callback、agent 產生的一次性腳本）都必須遵守；違反者由 `make ci-static` 的
> `scripts/jev-contract-check.py` 擋下。
> 由 2026-09-23 的實測與**複核更正**沉澱而成。實測數據見 `experiments/008-jev-systemone/`。

## 0. 唯一認可的呼叫方式

```python
from jevkit import noul, score, choice, ask, ask_or_none, calibrate_threshold
```
`scripts/jevkit.py` 已內建下列全部規則。**不要自己寫 urllib 呼叫**；若必須（例如跨機器無法
import），則必須逐條滿足 §1–§5。

## 1. 傳輸層（違反會直接失敗或靜默失效）

| 規則 | 原因（實證）|
|---|---|
| **一定要設 `User-Agent`** | TypeSafe 對 Python-urllib 預設 UA 回 **403**。曾讓兩個已上線機制靜默失效數小時。|
| `choice.criteria` **必須是 dict** `{選項: 描述}` | 傳陣列回 **422** `Input should be a valid dictionary`。|
| `score.criteria` 才是有序清單 | 兩者型別不同，勿互換。|
| 設 **timeout**（建議 10–20s） | 沒有 timeout 會卡住整個 turn。|
| **429 / 529 要 retry + exponential backoff，並 honor `retry-after`** | **官方**（`/api#handling-rate-limits`、`/models`）：官方重試碼只有 **429 Too Many Requests** 與 **529 Overloaded**；401／422 **不重試**。|
| **403 要 retry + backoff** | ⚠️ **本 repo 實證，非官方**：官方錯誤表**沒有 403** ⇒ 我們的 403 屬邊緣節流的工程處置（見下方 WAF 段）。**契約標記為實證補充，勿誤讀為官方契約** ✓ |
| state 上限：state + 最長問題 ≤ 32k tokens、整體 ≤ 64k | 超過會 4xx；jevkit 以字元保守估算並截斷。|
| **Choice 選項上限 255**／**Score 級數 2–10** | **官方**（`/api`）：Choice `criteria` ≤255 選項；Score 需 ≥2 級、上限 10 級。|
| **官方速率上限**：**100k tokens/s＋40 req/s**（官方註明**會動態調整**）| **官方**（`/models`）；超限回 429 ✓ |
| **官方價格**：**$42 / Btok（輸入）**、**輸出 token 免費** | **官方**（`/models`）⇒ 可用來交叉核對 §5 的本 repo 成本觀測 ✓ |

| **WAF 內容規則（已確認 2026-09-23，二分定位）** | state 含 **`curl -s`** 的請求會被 Cloudflare 以 **403** 擋下（需足夠大的 payload；短文字不觸發）。含 `-H "Authorization: Bearer $TOKEN"` 這類真實指令的 SOP／程式碼片段最易命中。**jevkit 已內建緩解**：先送原內容 → 403 時自動以等義替換（`curl -s`→`fetch`、`Authorization: Bearer`→`Auth-token`）重送，保留語意。|

> 測試方法（可重現）：`python3 -c` 送含 `curl -s` 的 SOP → 403；把 `curl -s` 換成 `fetch` → 200。
> 教訓：**先前「無法重現」的判斷是錯的**，原因是我的重現輸入不含觸發情境（短文字 / 非真實指令）。
> 對外部服務的異常，重現失敗不等於不存在 —— 要試到「真實使用情境」為止。

## 1.5 版本語意（**官方**，`/models`）

| 規則 | 依據 |
|---|---|
| **一律 pin 版本 ID（現行 `jev-1.13.0`），不得使用 alias `jev-latest`** | 官方：alias 會隨官方更新漂移 ——「If you have tuned confidence thresholds against a specific version, **pin that version's ID instead of the alias** and move to the new one on your own schedule」。CI 由 **C7** 強制（`scripts/jev-contract-check.py`）|
| **記錄每次回應的 `model` 欄位**（版本化 ID）| 官方：「The response's `model` field reports the versioned ID that answered, so you can log which model produced each result.」⇒ 觀測 JSONL 應含此欄位 ✓ |
| ★ **模型版本變更 ⇒ 重讀官方文件（尤其該版本的 jaggedness 頁）＋ 重跑門檻校準** | 官方每版本皆有 jaggedness 頁（例：`/model-jaggedness/jev-1.13`）；已校準的門檻**綁定版本**，換版即須重校 ✓ |

## 2. 判斷設計（決定品質）

1. **一題一個窄判斷**：把判斷條件寫在 `instructions`，候選寫在 `criteria`。題目 id 不會送給模型。
2. **永遠提供拒答選項**：`choice` 加 `none`；審核類加 `uncertain`。
3. **`score` 的每一級要能獨立成立**（描述具體情境，不要只寫「高/中/低」）。
4. **困難/模糊案例**才需要結構化 criteria（描述 + 對比 + 排除條件 + 範例）。
   實測：對**明確**問題，粗糙與精緻設計皆 100%（7/7）→ 別把「分數低」一律歸因於措辭。
5. **fan-out**：同一個 state 的多個問題要**一次送出**。
   官方**兩處數字不一致**（`/primitives` 本文 11.5× 便宜／9.6× 快；`llms.txt` 摘要 12.2×／10×）⇒ 引用時註明來源 ✓。
   但不要為了省錢把 state 重複塞進多個請求。

## 3. 門檻（最常犯的錯）

- **不得照抄 cookbook / 官方範例的門檻**。一律用自己的資料校準：

```python
from jevkit import calibrate_threshold
cal = calibrate_threshold([(0.9, True), (0.2, False)], min_precision=0.95)
```
- **Jev 系統性保守 —— ⚠️ 標記為「語料特異實測」，非模型通性**：實測 entity alignment 在 P(same)=0.2 時實際命中率 100%（ECE 0.142）。
  官方立場是機率經 **RLCD 校準**（`/introduction/machine-learning-primer`）⇒ 我們的偏移屬**特定語料**的觀測，**不得外推為「Jev 一律保守」**；
  門檻一律以**本語料**校準（證據見 `experiments/008-jev-systemone/`）。
  官方範例的 `score >= 1.5` 在我們的語料上不如 `P >= 0.2`。
- 用 **confidence / margin** 做升級閘門：`margin < 0.4` → 轉人工或升級模型（實測有效：低信心組錯誤率 50% vs 整體 31%）。
- ★ **`noul` 題不帶 confidence**（**官方** `/confidence`：「Noul answers don't carry one.」）⇒
  **不得**對 noul 答案讀 `confidence`、也不得把 confidence 當 noul 的閘門 ✗；
  noul 的表達方式只有**機率本身**（官方：0.5 ＝ yes/no 等機率，**不是**「中等」）⇒
  需要拒答／不確定時，依 §2.2 改用帶 `uncertain`／`none` 選項的 `choice` ✓；
  也不要在 noul 的 `instructions` 裡要求「以低信心作答」（那條通道不存在）✗

## 4. 評估紀律（**任何準確率數字都必須可驗證**）

1. **標註必須能自我驗證**：GT 要能用程式重新生成（例如 grep 定位 + 人工確認），不能只靠自稱。
   教訓：某次行級檢索報 90%，實際 GT 行號**指到空白行**；用它的 GT 重測只有 8%，重建 GT 後 100%。
2. **抽樣複核他人/agent 回報的數字**：本次因此更正了兩個 headline（審核 96.1%→64.5%、一致率 100%→98.7%）。
3. **不要對全文做禁字斷言**（模型會引用被禁止的字，例如解釋規則時）；程式碼比對要**正規化空白**。
4. **先確認 state 含判斷所需資訊**：曾拿 30 種模板字串推市場結果（AUC 0.03），那是輸入無資訊，不是模型弱。
5. **回報要分級**：`已驗證 / 已更正 / 未驗證`，並附樣本數與成本。

## 5. 生產整合紀律（fail-open）

1. **一律 fail-open**：Jev 故障不得影響產品（`ask_or_none()`）。
2. **log-first**：先 `log`/shadow 模式累積校準資料，再切 `enforce`（hermes 外掛與 LiteLLM guardrail 皆如此）。
3. **可一鍵關閉**：環境變數或 config 開關；關掉不得需要改碼。
4. **可觀測**：每次判定寫 JSONL（分數、門檻、決定、延遲、tokens），供事後校準與稽核。
5. **成本/延遲期待**：~$0.00003–0.002/次；TTFB 約 284ms（台灣）→ 端到端 265–663ms，**不要期待官方的 150ms**；
   吞吐靠批次與並行（3.4×–12×）。

## 5.1 現況合規清單（既有呼叫者）

| 呼叫者 | 位置 | §1 傳輸 | §5 fail-open | 使用 jevkit |
|---|---|---|---|---|
| `hermes-jev-prefilter` | Mac Mini `~/.hermes/plugins/jev-prefilter/jev_gate.py` | ✅ UA+retry+timeout | ✅ 全錯誤回 None | ⚠️ 例外：自帶 urllib（已符合 §1；state 有截斷）|
| `hermes-jev-skill-suggest` | Mac Mini `~/.hermes/plugins/jev-skill-suggest/suggest.py` | ✅ UA+retry+timeout | ✅ | ⚠️ 例外：同上 |
| `litellm-jev-guardrail` | Mac Mini `~/code/litellm/custom_callback/jev_guardrail.py` | ✅ UA（2026-09-23 補）| ✅ | ⚠️ 例外 |
| `a2a-dev` 內新程式 | 本 repo | 必須用 `scripts/jevkit.py` | 必須 | ✅ 由 CI 強制 |

> 例外者屬既有生產程式碼，改動風險高於收益；新程式碼一律從 `jevkit` 起手（CI 會擋）。
> Mac Mini 已安裝 `~/.local/lib/jevkit.py` 供該機新程式使用。

## 6.1 契約檢查器的自測（C5）——**預設純靜態；live 為手動入口**

- **預設不再連外** ✓：`scripts/jev-contract-check.py` 預設只做 AST／靜態檢查 ⇒
  **預設路徑永不因網路或限流失敗而紅** ✓（`make ci-quick`／pre-push／CI 皆走預設 ✓）
- **live 檢查保留為手動入口** ✓（兩個確切命令）：
  - `python3 scripts/jevkit.py --selftest` —— 直接驗證 jevkit 對 API 的呼叫
  - `python3 scripts/jev-contract-check.py --with-selftest` —— 靜態檢查 ＋ live 自測
  - （等義環境變數：`JEV_CONTRACT_SELFTEST=1`；`JEV_CONTRACT_STRICT_SELFTEST=1` 為**相容別名** ✓）
- **opt-in 下的失敗語意** ✓：live 自測失敗 ⇒ **blocking（rc≠0）** ✓（顯式要求才執行的檢查，不該再降級為警告）
- **動機與出處** ✓：原設計把**真實連外呼叫**放在閘門內 ⇒ 外部服務／網路一 flake 就擋合法 push ✗
  （`FU-20260926-21`）；且一次 live 檢查成本 ≈ **$0.000012**（281 input tokens @ $0.042/Mtok，官方價 ✓）
  ⇒ 改為「**預設零連外、要驗才驗**」✓

> 範圍：本節只改 **C5 的 live 部分**。`# jev-docs:` 標頭要求（**C6**）與其餘靜態檢查
> （**C1–C4**、**C6**、**C7**）**不受影響** ✓ —— 它們從不連外，且仍是預設路徑的一部分 ✓

## 7. 官方文件出處（**normative 依據**；本契約每一條都指回這裡）

> 索引：**`https://docs.typesafe.ai/llms.txt`**（57 條；新增規則前先讀索引 ✓）。
> 取用方式：Mintlify 於頁面路徑後加 `.md` 即得 Markdown（例 `…/api.md`）✓。

| 官方頁 | 支撐本契約哪一節 |
|---|---|
| `/api`（HTTP API reference） | §1：`criteria` 型別（choice＝map、score＝有序陣列）、**Choice ≤255 選項**、**Score 2–10 級**、**錯誤表 401／422／429／529**、rate-limit 退避指引 |
| `/models` | §1（**64k／32k**、**100k tok/s＋40 req/s**、**$42/Btok 輸入、輸出免費**）／§1.5 **版本語意**（alias 漂移、pin 版本、記錄回應 `model`）|
| `/confidence` | §3：confidence 由機率分佈導出（例 `(3×最大機率−1)/2`）、**0.5 floor**、「Low confidence ⇒ do not act／route to a human」、**noul 不帶 confidence** |
| `/primitives`（＋`/patterns/fan-out`） | §2.2（`other`／`none of the above` 逃逸選項）、§2.5（**同 state 的問題一次送**；批次數字） |
| `/concepts/how-to-build-with-system-one` | §2.1（narrow, typed questions；code owns the workflow）、§3／§4（**用自家資料測門檻：plot confidence against accuracy**） |
| `/cookbooks/citation_check`／`/cookbooks/llm_guardrails` | §3：**「Start high, and lower the threshold as you see how the model does on your own documents」**／「route() thresholds **in your code**」⇒ 支持「不得照抄 cookbook 門檻」 |
| ★ `/model-jaggedness/jev-1.13` | §2／§4：**該版已知失效模式**（見下表）⇒ 設計題目前必讀 |
| `/introduction/quickstart` | §0／§1：最小可用請求形狀（官方範例以 `curl` 呼叫，**未設 `User-Agent`**）|
| `/sdk`（＋`/sdk/python/api/retries`、`/sdk/python/api/constants`） | §0：官方 SDK 為另一條合法路徑（自動重試／`TYPESAFE_API_KEY`、`TYPESAFE_BASE_URL`、`TYPESAFE_DEFAULT_MODEL`）；本 repo 選 `jevkit` 之理由＝**跨機單檔、零依賴**（SDK 需安裝）；差異與取捨須明示 ✓ |

### 7.1 ★ Jev 1.13 已知失效模式（官方 `/model-jaggedness/jev-1.13`）⇒ 對應我們的紀律
| 官方失效模式 | 我們的對策 |
|---|---|
| **Math and Numbers**（計數、數值表示、以 score 做運算） | **★ 一切數值運算在 code 做**，Jev 只負責窄判斷（同族於 §4「AUC 不是錢」）|
| Date and time comparison | 日期比較在 code 完成（本 repo GT 一律由程式重生，§4.1 ✓）|
| Literal reading | 題目與 criteria 寫明邊界（§2.1／§2.3 ✓），不倚賴常識補全 |
| Indirection | 避免多跳推論；需要時拆成多題（§2.1 ✓）|
| **Large state full of irrelevant detail** | state 只放判斷所需材料（§4.4 ✓）；jevkit 亦做長度上限截斷（§1 ✓）|
| Adversarial content／Contradictory instructions and criteria | state 為外部內容時，先做輸入淨化；criteria 不得互相矛盾（§2.3 ✓）|
| Generation | **不要要求 Jev 產生文字**（它是判斷模型，不是生成模型）|

## 6. 相關檔案

| 檔案 | 用途 |
|---|---|
| `scripts/jevkit.py` | 唯一認可呼叫方式（題型驗證、UA、retry、截斷、門檻校準、fail-open）|
| **官方文件索引** | **`https://docs.typesafe.ai/llms.txt`**（權威出處總表見 §7 ✓）|
| `scripts/jev-contract-check.py` | 本規範的 CI 靜態檢查（違反者 ci-static FAIL）|
| `experiments/008-jev-systemone/RESULTS-5-USECASES.md` | 五項應用實測（含驗證狀態）|
| `experiments/008-jev-systemone/MEASUREMENT-LESSONS.md` | 量測教訓全文 |
| `experiments/008-jev-systemone/verify_semantic_find.py` | 可驗證 GT 的範例實作 |
