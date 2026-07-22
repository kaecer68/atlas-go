# 系統健康盤查 — 2026-07-23（修正版）

> **Scheduler 運作正常**（29/65 tasks 已執行，36 條為長間隔）。
> 真實問題：資料源缺口導致校準無法完成。


> 從 Hermes 消費者體驗審計發現的系統性問題，非逐個 bug 修復。

## 五個根因模式

### 根因 1：資料管線從未進入 production-ready 狀態

**現狀**：
- `auto_cycle_update`：ERROR，最後更新 **2026-05-14**（超過 2 個月未修）
- `taiex_index`：ERROR，大盤指數資料源失敗
- `taifex_daily`：ERROR，期貨資料失敗
- `vix`：DEGRADED，3 天未更新
- 所有 capital_flow forces：`sample_count=3`，`calibration_status=calibrating`
- `cross_market.available=false`：國際比較永遠無法計算
- `government` flow：淨值永遠 0
- `replay_data_latest_date`：落後 1 天
- `last_window_id`：3 天前的 window

**不是「缺少某個欄位」的問題**。是**整個資料管線從未連續運作超過一週**。6 個月來每天修個別資料源，但從未診斷管線本身的架構缺陷。scheduler 被反覆提醒但始終未自動化（#825, #826 blocked）。

**假設**：管線設計時假設所有外部資料源都可靠，但實際台灣金融資料源（TWSE、TEJ、TAIFEX）有規律性的中斷週期（結算日、國定假日、API 限流），管線沒有 graceful degradation。

**待驗證**：
- [ ] scheduler 是否真的有在跑？還是手動觸發？
- [ ] 每個 ERROR channel 的根因是什麼？API key 過期？格式變更？
- [ ] sample_count=3 是因為 replay 只有 3 天資料，還是 rolling window 設定錯誤？

### 根因 2：架構漂移 — 路由、canary、合約三層不一致

**現狀**（已部分修復，但流程缺陷仍在）：
- 發現 46 條 canary test stale paths（已修）
- 發現 canary ↔ handler 交叉驗證缺失（已補 `verify-canary-vs-handler.py`）
- 發現 canary → contract → handler 錯誤傳播鏈（PR #1276 從一開始就寫錯）
- routes 遷移到 `/api/dashboard/*` 但 canary 沒跟上

**不是「這次修完就沒了」的問題**。是**沒有機制阻止未來再發生**。`verify-canary-vs-handler.py` 解決了 canary↔handler，但 handler↔router 還沒驗證。contract↔consumer expectation 更沒有。

**假設**：架構變更（route 遷移）時沒有強制的回歸測試，因為 canary test 是 build-tagged (`//go:build canary`)，從未在 CI 跑。

**待驗證**：
- [ ] handler code 中還有多少路徑在 route table 中不存在？
- [ ] 有沒有 consumer（如 dashboard UI）打的 API path 已經不存在？

### 根因 3：文件體系成為干擾而非助力

**現狀**：
- docs/ 有 196 個 .md 檔案，37,695 行
- 590 處內部交叉引用 — 更新一個文件可能破壞多個引用
- 10 個文件標記 deprecated，但未被清理
- 7 個文件有 TODO 未完成
- CLAUDE.md + AGENTS.md + copilot-instructions.md 三者部分重疊
- 6 條 audit 規範（A-H）僅為描述性文字，無強制機制

**不是「文件太多」的問題**。是**文件與程式碼之間沒有同步機制**。spec 寫了 route 應該長什麼樣，但實際 route 已經變了，spec 沒更新。文件成為「理想狀態的描述」，而非「當前狀態的記錄」。

**假設**：Agent（Claude/Hermes）讀文件時，會基於過時的 spec 做出錯誤判斷。文件越多 → agent 注意力被分散 → 更容易引用錯誤資訊。

**待驗證**：
- [ ] 哪些 spec 描述的路由/欄位已經不存在？
- [ ] 哪些文件從未被引用（dead docs）？
- [ ] CLAUDE.md 和 AGENTS.md 的內容是否衝突？

### 根因 4：消費者體驗未經 E2E 驗證

**現狀**（從 Hermes role-play 發現）：
- 新用戶第一個 tool（mcp_quickstart）就失敗
- 誤導性輸出：「上漲 0.00%」（已修）
- 預測輸出缺乏解釋（5 天相同信心 0.88，consumer 不知道為什麼）
- `force` vs `name` 欄位命名不一致
- 校準狀態未透傳給 consumer
- `crossmarket_get_us_indices` 回應不一致（有時 meta 有時數據）

**不是「個別 API bug」的問題**。是**從未有人以消費者視角完整走過一次 onboarding 流程**。每次修復都是從 developer 視角（「handler 寫對了」「route 存在」），而非 consumer 視角（「我打這個 tool 拿到什麼」「我第一次用會怎麼理解」）。

**假設**：如果有一條 consumer journey test（從 MCP tool discovery → first call → response interpretation），90% 的誤導性問題會在開發階段就被發現。

**待驗證**：
- [ ] 有多少 tool 的 response 包含 consumer 無法理解的內部術語？
- [ ] 有多少 tool 的錯誤回應是 401/404/503 而非有意義的 error message？

### 根因 5：預測能力為零 — 系統從未完成校準

**現狀**：
- 所有 predictions 信心 0.88，5 天完全相同
- 所有 forces `calibration_status=calibrating`
- `sample_count=3` 意味著統計上不可靠
- `cross_market.available=false` — 最關鍵的國際比較永遠不可用
- 系統設計目標是「預測錢潮方向」，但從未產出過有統計意義的預測

**不是「參數還沒調好」的問題**。是**系統架構假設 data pipeline 穩定後才能校準，但 data pipeline 從未穩定**。形成死循環：管線不穩 → 無法校準 → 無法預測 → 無法證明系統價值 → 不投入資源修管線。

**假設**：即使 sample_count 只有 3，也應該能產出「低信心預測 + 明確標記資料不足」，而非隱藏校準狀態讓 consumer 以為預測可靠。

**待驗證**：
- [ ] 最小 sample_count 是多少才能進入「已校準」狀態？
- [ ] 過去 6 個月有任何一天 sample_count > 3 嗎？
- [ ] 預測模型的 baseline 是什麼？跟 random guess 比有顯著差異嗎？

---

## 修復策略：四階段

### Phase 1：資料基礎（停止失血）

目標：3 個 ERROR channel 恢復運作，sample_count 開始累積。

| # | 行動 | 驗收 |
|---|------|------|
| 1.1 | 診斷 `auto_cycle_update` 為何 2 個月未修 | channel status → OK |
| 1.2 | 診斷 `taiex_index` + `taifex_daily` ERROR | channel status → OK |
| 1.3 | 確認 scheduler 是否自動觸發 | `cron-entrypoint.sh` log 驗證 |
| 1.4 | 確認 replay 資料是否每天更新 | `replay_data_latest_date = today` |
| 1.5 | 確認 sample_count 開始增加 | ≥5（從 3 開始） |

### Phase 2：架構一致性（防止再漂移）

目標：handler↔router↔canary↔contract 四層一致，CI 強制。

| # | 行動 | 驗收 |
|---|------|------|
| 2.1 | handler↔router 交叉驗證：handler 中每個 path 都在 route table | script + CI job |
| 2.2 | 清除所有 auth:optional 但實際需要 auth 的合約標記 | contract 跟現實一致 |
| 2.3 | canary test 從 build-tag 提升為 CI job | CI 自動跑 canary |
| 2.4 | contract 加入 `consumer_facing` 標記（哪些 tool 是給 external consumer 用的） | contract 欄位 |

### Phase 3：消費者體驗（以 consumer 視角重構）

目標：新用戶 5 分鐘內能拿到有意義的市場資訊。

| # | 行動 | 驗收 |
|---|------|------|
| 3.1 | Consumer journey test：MCP discovery → first call → interpret response | script |
| 3.2 | 所有 tool 回應加 `data_quality` 欄位（sample_count, calibration_status, freshness） | API schema |
| 3.3 | 預測加 `rationale` 欄位（為什麼是這個信心、這個方向） | API schema |
| 3.4 | 修復 mcp_quickstart | tool 正常回傳 |
| 3.5 | forces 加 `display_name` 中文欄位 | API schema |

### Phase 4：廢棄清理（減熵）

目標：刪除過時文件、死碼、未使用的 tool。

| # | 行動 | 驗收 |
|---|------|------|
| 4.1 | 標記並刪除未使用的 MCP tools | contract tool count 下降 |
| 4.2 | 清理 deprecated forces（futures, tsm_adr） | API response 不再出現 |
| 4.3 | 清理過時 specs/docs（與程式碼不一致的） | docs count 下降 ≥20% |
| 4.4 | CLAUDE.md / AGENTS.md 去重整合 | 無衝突內容 |

---

## 驗收標準（全局）

修復完成 = 以下全部為真：

1. 連續 7 天 `sample_count ≥ 5`，`calibration_status ≠ calibrating`
2. 0 個 ERROR channel
3. Consumer journey test PASS（5 分鐘內拿到有意義的市場資訊）
4. 預測輸出包含 `rationale`，且 5 天預測不完全相同
5. handler↔router↔canary↔contract 四層交叉驗證全部 PASS
6. CI 自動執行以上所有驗證
