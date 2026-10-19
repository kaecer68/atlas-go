# llm-annotator runbook（失敗歸因的 LLM 呼叫：burn rate / 斷路器 / 無流量）

> **告警（5 條帶 `runbook_url`，全部指到本檔）**：
> `LLMAnnotatorHighErrorRateFastBurn`、`...MediumBurn`、`...SlowBurn`、
> `LLMAnnotatorCircuitBreakerOpen`、`LLMAnnotatorNoTraffic`
> —— 全部在 `monitoring/rules/llm_annotator_alerts.yml`。
> 另有 4 條同族告警（`LLMAnnotatorRateLimited`／`TransportErrors`／`ProtocolErrors`／
> `RetryExhausted`，`error_category` 標籤）**沒有** `runbook_url`，本檔一併涵蓋。
> 錄音規則（burn rate 的輸入）：`monitoring/rules/llm_annotator_recording.yml`。
>
> **建立背景（2026-10-16）**：這 5 個 `runbook_url` 原本指向
> `https://wiki.internal/runbooks/...`，而 `wiki.internal` 是 **DNS NXDOMAIN**
> ⇒ 事件當下點進去是死的。
>
> ⚠️ **先讀 §1**：這個告警家族對應的指標在現行接線上**沒有生產者**。

## 1. 這個家族量的是什麼（含「它現在不會動」的實證）

指標只有一個來源：`internal/llm_annotator` 的 `KimiClient.recordOutcome`
→ `llm_annotator_requests_total{provider,outcome}`。
`outcome` 的完整值域（`annotator.go`）：`success`、`cache_hit`、`rate_limited`、
`protocol_error`、`transport_error`、`retry`、`retry_exhausted`、`client_error`、
`canceled`、`circuit_open`。

**但 `KimiClient.Annotate` 自 issue #1897 起已不是線上 annotate 後端**：

- `POST /api/strategies/{id}/annotate` 走的是 `llm.Router`
  （`llmAdapters.NewRouterAnnotator`，wiring 在 `cmd/atlas/annotator_wiring.go`）。
- legacy `KimiClient` 只保留一個用途：`GET /api/llm_annotator/cost` 的**唯讀用量來源**
  （`SetAnnotatorUsageSource`），它**不是** `/annotate` 的路徑。
- `configs/llm_router.yaml` 檔頭逐字寫明：`kimi — Kimi K2.7 (kimi-for-coding);
  NOT in any chain since ADR-012 addendum 4`；`failure_attribution` 鏈是
  `primary: minimax / backup1: deepseek / last_resort: mock`。

**實證（2026-10-16，唯讀 Prometheus 查詢）**：

| 查詢 | 結果 |
|---|---|
| `count(llm_annotator_requests_total)` | **空集合**（家族沒有任何序列） |
| `count(llm_router_fallback_triggered_total)` | `2`（router 走的是另一族指標） |

**推論與其界線（誠實標註）**：

- 本檔 5 條告警的 `expr` 全部建立在 `llm_annotator_requests_total` 或其 recording 規則上。
  Prometheus 對**不存在**的序列做 `sum(rate(...))`／比率運算會得到**空集合**，
  不是 0 ⇒ **這 5 條不會亂響，但也不會保護任何東西**（休眠，不是永久 firing）。
- 「不會響」不構成缺陷；**缺陷是「這個家族看起來是綠燈」**——它量的是一個不在
  生產路徑上的 client。修法選項見 §6。
- 這條推論來自「程式碼的唯一呼叫點 ＋ router 設定 ＋ 生產空集合」三者一致；
  若日後有人把 legacy 路徑接回 `/annotate`（#1897 之前的形狀），
  上述空集合就會變成有值 ⇒ 屆時本節要改寫。

## 2. 診斷指令（任何一條先做）

```bash
# 1. 家族是否存在（空 = 沒有生產者，見 §1）
docker exec atlas-go curl -s localhost:18080/metrics | grep -c '^llm_annotator_requests_total'

# 2. 用 router 的指標看「失敗歸因」實際的健康（現行路徑）
docker exec atlas-go curl -s localhost:18080/metrics \
  | grep -E '^(llm_client_requests_total|llm_router_fallback_triggered_total|llm_router_backup_chain_exhausted_total)'

# 3. 後端有沒有被接起來（decision tag 會直說）
docker logs atlas-go 2>&1 | grep -E 'kimi_annotator_(loaded|disabled)|llm_router_config'

# 4. 端點層的真實結果（不是指標層）
curl -s -o /dev/null -w '%{http_code}\n' -X POST \
  -H "X-API-Key: $ATLAS_API_KEY" -H 'Content-Type: application/json' \
  -d '{"frame_id":"probe"}' http://localhost:18080/api/strategies/probe/annotate
# 200 = 鏈有回答；502 = 鏈被走完仍失敗；503 = 根本沒接起來

# 5. 斷路器狀態（channel id = llm_annotator）
curl -s -H "X-API-Key: $ATLAS_API_KEY" http://localhost:18080/api/health/aggregate | head -c 1500
```

## 3. Burn-rate 三條（fast 14.4× / medium 6× / slow 1×，門檻為 0.1% 錯誤預算）

**症狀**：`llm_annotator:error_rate:rate{1h,6h}` 同時超過門檻（`for` 分別 1m／5m／6h）。
預算以 **30 天、0.1%**（=1−99.9%）計 ⇒ fast burn 約 2 天燒完、medium 約 5 天、slow 等於維持在基準。

**判讀**：`error_rate = 1 − good/total`，`good = outcome ∈ {success, cache_hit}`。
兩個視窗**同時**超過才成立 ⇒ 只會是持續性錯誤，不會是單次抖動。

**在現行接線下**（§1）：序列不存在 ⇒ recording 規則無輸入 ⇒ 空集合 ⇒ 不響。
若**真的響了**，第一個要問的不是「哪個 provider 壞了」，而是
**「誰把 legacy client 接回呼叫路徑了」**（#1897 的回歸）。

## 4. `LLMAnnotatorCircuitBreakerOpen`（critical；`rate(...{outcome="circuit_open"}[1m]) > 0`）

**症狀**：斷路器以 `circuit_open` 拒絕呼叫。breaker 是 `apigateway.CircuitBreaker`，
channel id `llm_annotator`（`internal/llm_annotator/circuit_breaker.go` 的 facade）。

**判讀**：

1. 先看是不是**預算觸發的手動開路**：`Config.BudgetThreshold` 達標時 wiring 會
   `ForceOpen()` ＋ `SetManualOverride(true)`（= 「維持開路，直到 operator Reset」）。
   這個形狀**不會自癒**，重啟才會清掉（override 不是持久狀態）。
2. 再看是不是**連續失敗**自動開路（closed → open → half-open → closed）。
3. 呼叫層的限流是 **1 req/s、burst 4**（`rate.NewLimiter(rate.Every(time.Second), 4)`）；
   `outcome="rate_limited"` 代表連本地 limiter 都排不到（`Wait` 被 context 取消），
   不是上游 429。
4. HTTP 狀態的分類：`client_error`（4xx，**不重試**）、`retry_exhausted`（有狀態碼但重試用完）、
   `transport_error`（無狀態碼，網路層）、`protocol_error`（200 但回應不可解析）。

**緩解**：確認是預算觸發還是連敗觸發；前者要 operator 決定要不要提高預算或 Reset，
後者要修上游。重試上限是 **3 次**（`maxAttempts`），所以「重試」不會無限延長事件。

## 5. `LLMAnnotatorNoTraffic`（warning；`sum(rate(success[5m])) == 0 and sum(increase(success[10m])) == 0`）

**症狀**：10 分鐘內沒有任何成功呼叫。

**判讀**：

1. 先分辨「服務掛了」與「沒有流量」：`outcome="success"` 的序列**存在但為 0**
   ⇒ 真的沒有成功呼叫；序列**不存在** ⇒ 空集合，不會成立（這也是 §1 的情況）。
2. 現行架構下 `/annotate` 是**隨請求觸發**的（使用者按下去才有呼叫）
   ⇒ 沒有流量本身是常態。這條規則的價值在「有流量卻全部失敗」的形狀；
   若確認 `/annotate` 有請求進來但 5m／10m 內 success 皆 0，走 §4 的分類。

**緩解**：先確認 #3 指令的 HTTP 碼；502 代表鏈走完失敗（看 router 的 fallback 指標），
503 代表沒接起來（看 §2 指令 3 的 decision tag）。

## 6. 要修這個家族的話（選項，本檔不實作）

1. **退役**這 5 條（含 4 條無 `runbook_url` 的同族）：規則沒有生產者時，
   綠燈是假的安心；退役的判準與格式見 `monitoring/rules/` 內的既有先例註解。
2. **改指向 router 的指標**：把 burn rate 建在現行的
   `llm_client_requests_total`／`llm_router_fallback_triggered_total` 上
   （issue #1926 才剛把 router 的 fallback 接上 `/metrics`）。
3. **明確接回 legacy**：只有當有人要把 `KimiClient` 放回 `/annotate` 時才成立；
   那等於撤掉 #1897，需另案決定。
4. 不論選哪個：**指標名稱與標籤是對外契約**（`llm_annotator:error_rate:*` 這些 recording
   規則名也在 Grafana 被引用），改名要有對照表。

## 7. 升級路徑

1. 值班先跑 §2 的五個指令，把 HTTP 碼、decision tag、router 指標現值記下來。
2. 「鏈答不出來」→ `internal/llm`（Router／capability chain）擁有者。
3. 「指標家族不存在」→ `internal/llm_annotator`（legacy）與 `internal/monitoring`（collector）。
4. 「規則／annotation 本身有問題」→ `monitoring/`（權威樹；
   `scripts/ci/check_monitoring_single_source.sh` 會擋第二棵樹）。

## 8. 已知限制（刻意列舉）

- 本家族**不覆蓋** failure attribution 的實際成功率；那要看 router 的
  `llm_client_requests_total` 與 `llm_router_fallback_triggered_total`。
- `monitoring/tests/*_test.yml` 沒有本家族的 fixture ⇒ 這 5 處 `runbook_url` 沒有 promtool 單元測試覆蓋。
  ⚠️ 之所以要強調：promtool `test rules` **會**逐字比對 alert 的 annotation（含 `runbook_url`）
  （2026-10-19 在 replay 家族實測；有 fixture 的家族改 URL 未同步 ⇒ RC=1）——
  本家族**沒有**這道守門，只有 R4 驗目標存在。
- §1 的接線事實有時間性：以 `configs/llm_router.yaml` 與
  `cmd/atlas/annotator_wiring.go` 的現況為準。

## 9. 相關文件

- [llm-routing-spec.md](../specs/llm-routing-spec.md) — Router 與 fallback 鏈（ADR-012）
- [llm-interface-contract-spec.md](../specs/llm-interface-contract-spec.md) — LLM 介面契約
- [local-deploy.md](local-deploy.md) — 部署／重啟／回滾
- [traps.md](../reference/traps.md) — 跨模組陷阱（含「LLM 路由繞過」）
