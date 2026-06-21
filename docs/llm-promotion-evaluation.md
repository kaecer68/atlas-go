# LLM 功能晉升評估清單（E-Tier → 下一階段）

**文件目的**: 評估實驗性（E-Tier）LLM 功能是否滿足晉升至更高成熟度階段的條件。
每個功能模組在晉升前必須通過此清單的所有檢查項。

**最後更新**: 2026-06-21

---

## 1. 熔斷器狀態（Circuit Breaker Status）

### 1.1 各 Provider 過去 30 天熔斷記錄

| Provider | 熔斷次數 | 累計 Open 時長 | 最後一次熔斷日期 | 狀態 |
|----------|----------|----------------|-------------------|------|
| DeepSeek | | | | ☐ |
| MiniMax | | | | ☐ |
| Kimi | | | | ☐ |
| OpenCodeGo | | | | ☐ |

### 1.2 驗證步驟

```bash
# 檢查 router 層級的 fallback 計數器
grep -r "FallbackTriggeredTotal\|BackupChainExhaustedTotal" internal/llm/router.go

# 若已接入 metrics，查詢 Prometheus
# fallback_triggered_total{provider="deepseek"}
# backup_chain_exhausted_total
```

**閾值**: 任一 provider 過去 30 天熔斷次數 > 3 次 → 晉升前需調查。

☐ 所有 provider 熔斷次數 ≤ 3  
☐ 無持續 Open 超過 1 小時的熔斷事件

---

## 2. JSONL 審計完整性（JSONL Audit Fidelity）

### 2.1 annotations.jsonl 完整性檢查

| 檢查項 | 目標 | 實際 | 通過 |
|--------|------|------|------|
| 總記錄數 | > 0 | | ☐ |
| 有效 JSON 行數 | = 總記錄數 | | ☐ |
| 含 `annotations` 欄位 | = 總記錄數 | | ☐ |
| 含 `file` + `line` + `severity` + `message` | ≥ 90% | | ☐ |
| 空輸出記錄比例 | < 30% | | ☐ |

### 2.2 驗證步驟

```bash
# 統計 annotations.jsonl 記錄數
wc -l data/annotations.jsonl

# 檢查無效 JSON 行數
while IFS= read -r line; do
  echo "$line" | python3 -m json.tool > /dev/null 2>&1 || echo "INVALID"
done < data/annotations.jsonl | grep -c "INVALID"

# 檢查空 annotations 比例
grep -c '"annotations":\[\]' data/annotations.jsonl
```

☐ JSONL 檔案存在且非空  
☐ 無效 JSON 行數 = 0  
☐ 空 annotations 比例 < 30%

---

## 3. 監控覆蓋率（Monitor Coverage）

### 3.1 健康檢查端點

| 端點 | 狀態碼 | 延遲 p50 | 延遲 p95 | 延遲 p99 | 錯誤率 |
|------|--------|----------|----------|----------|--------|
| `/api/llm/health` | | | | | |
| `/api/llm/health/deepseek` | | | | | |
| `/api/llm/health/minimax` | | | | | |

### 3.2 驗證步驟

```bash
# 健康端點檢查
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/llm/health

# 延遲統計（若有 metrics endpoint）
curl -s http://localhost:8080/metrics | grep llm_request_duration
```

**閾值**:
- p99 延遲 < 30s（code_review_annotation 可容忍較高延遲）
- 錯誤率 < 5%

☐ 所有健康端點返回 200  
☐ p99 延遲 < 30s  
☐ 錯誤率 < 5%

---

## 4. 能力覆蓋清單（Capability Checklist）

### 4.1 12 項 LLM 能力支援矩陣

| # | 能力常數 | DeepSeek | MiniMax | Kimi | 備註 |
|---|----------|----------|---------|------|------|
| 1 | `failure_attribution` | | | | |
| 2 | `code_review_annotation` | | | | 本次評估目標 |
| 3 | `prompt_lint` | | | | |
| 4 | `rationale_generation` | | | | |
| 5 | `strategy_summary` | | | | |
| 6 | `risk_surface_extraction` | | | | |
| 7 | `regime_explanation` | | | | |
| 8 | `scenario_simulation` | | | | |
| 9 | `sentiment_explanation` | | | | |
| 10 | `performance_forensics` | | | | |
| 11 | `contra_attribution` | | | | |
| 12 | `confidence_commentary` | | | | |

標記：✅ 支援、❌ 不支援、⚠️ 部分支援、— 未測試

### 4.2 本次評估目標功能

| 能力 | Handler 檔案 | Schema 檔案 | 狀態 |
|------|-------------|-------------|------|
| `code_review_annotation` | `internal/llm/capabilities/code_review_annotation.go` | `internal/llm/schemas/code_review_annotation.go` | |

☐ 目標能力在所有註冊 provider 上均返回有效結果  
☐ 所有 provider 至少完成 10 次成功呼叫  
☐ 輸出格式符合 schema 定義

---

## 5. 降級觸發記錄（Fallback Trigger Log）

### 5.1 指標計數

| 指標 | 當前值 | 閾值 | 通過 |
|------|--------|------|------|
| `FallbackTriggeredTotal` | | < 10/天 | ☐ |
| `BackupChainExhaustedTotal` | | = 0 | ☐ |

### 5.2 事件記錄（最近 30 天）

| 日期 | 觸發次數 | 主要 Provider | 降級至 | 根因 |
|------|----------|---------------|--------|------|
| | | | | |
| | | | | |
| | | | | |

### 5.3 驗證步驟

```bash
# 從 logs 中提取降級事件
grep "fallback\|FallbackTriggered\|BackupChainExhausted" logs/atlas.log | tail -50

# 或從 metrics 查詢
# rate(fallback_triggered_total[24h])
```

☐ `BackupChainExhaustedTotal` = 0  
☐ 所有降級事件有記錄根因  
☐ 無未知根因的降級事件

---

## 6. 晉升決策（Promotion Decision）

### 6.1 必要條件（全部通過方可晉升）

☐ 1. 所有 provider 熔斷次數 ≤ 3（過去 30 天）  
☐ 2. JSONL 審計完整性檢查全部通過  
☐ 3. 健康端點監控覆蓋率達標  
☐ 4. 目標能力 12 項中至少 2 項 provider 支援  
☐ 5. `BackupChainExhaustedTotal` = 0  
☐ 6. 無未解決的 critical/high bug 與此功能相關  
☐ 7. 程式碼已通過 `cmd/check-maturity` 驗證

### 6.2 決策

| 決策 | 簽署人 | 日期 |
|------|--------|------|
| ☐ GO — 所有必要條件通過，建議晉升 | | |
| ☐ NO-GO — 必要條件未滿足，需修復後重新評估 | | |

**阻擋項目**（若 NO-GO）:

1. 
2. 
3. 

---

## 7. 執行步驟（Execution Steps）

### 7.1 晉升執行（若 GO）

- [ ] 更新 `internal/MATURITY.md`：將目標能力成熟度標記從 `experimental` 移至目標層級
- [ ] 更新 `internal/llm/doc.go`：調整對應能力的說明文字（移除 "experimental" 標記）
- [ ] 更新 `internal/llm/schemas/doc.go`：同步 schema 成熟度標記
- [ ] 執行 `go run ./cmd/check-maturity` 確認無違規
- [ ] 提交 PR，標題格式：`maturity: promote code_review_annotation to [evolving|stable]`
- [ ] 通知團隊（Slack #atlas-llm / 郵件）

### 7.2 修復執行（若 NO-GO）

- [ ] 建立阻擋項目追蹤 issue
- [ ] 分配負責人與預計完成日期
- [ ] 完成修復後重新執行本清單
- [ ] 更新本文件日期與版本

---

*此文件為實務操作清單，非學術論文。填寫完畢後歸檔至 `docs/archive/`。*
