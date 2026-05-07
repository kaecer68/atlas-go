# Atlas Operations Guide Skill

**版本**: 1.0  
**日期**: 2026-04-23  
**職責**: 日常運維、緊急應變、流程標準化  

---

## 每日運維檢查清單

### 開盤前（08:30）

- [ ] **MacroIngestor**: 確認最新宏觀數據已讀取
  ```bash
  go run ./cmd/atlas --check-macro-data
  ```
- [ ] **風險評估**: 檢查當日宏觀風險等級
  ```bash
  go run ./cmd/atlas --risk-assessment
  ```
- [ ] **基線檢查**: 確認 `data/state/baseline_policy.json` 存在且有效
  ```bash
  ls -la data/state/baseline_policy.json
  ```
- [ ] **Agent Prompt**: 確認所有 enabled agent 有對應 prompt 檔案
  ```bash
  bash ./scripts/openclaw/verify-governance-gates.sh --require-scenario-diversity
  ```

### 盤中（09:00-13:30）

- [ ] **監控外資流向**: 每小時檢查外資買賣超
- [ ] **監控異常波動**: 單檔漲跌幅 > 7% 或組合回撤 > 5%
- [ ] **NarrativeEvent**: 重大新聞/事件時，手動觸發推導

### 收盤後（14:00）

- [ ] **Ledger 備份**: 確認當日交易記錄已寫入
  ```bash
  tail -5 data/state/recommendation_outcomes.jsonl
  ```
- [ ] **日報生成**: 執行日報腳本
  ```bash
  go run ./cmd/atlas --generate-daily-report
  ```
- [ ] **Darwinian 權重**: 檢查是否有連續截斷警告
  ```bash
  go run ./cmd/atlas --check-darwinian-clipping
  ```

---

## 每週運維（週一）

- [ ] **實驗審查**: 檢查上週實驗結果
  ```bash
  go run ./cmd/judge-experiment --list-pending
  ```
- [ ] **模型績效**: 評估上週投資模型預測準確率
- [ ] **覆蓋率檢查**: 確認測試覆蓋率 >= 40%
  ```bash
  go test -coverprofile=coverage.out ./...
  go tool cover -func=coverage.out | grep total
  ```

---

## 每月運維（月初）

- [ ] **模型權重調整**: 基於績效回饋調整投資模型權重
- [ ] **回撤機制檢討**: 檢查當月回撤觸發記錄，評估是否需要調整閾值
- [ ] **技能文件審查**: 確認所有技能文件與程式碼一致
- [ ] **GitNexus 索引更新**: 重新索引程式碼庫
  ```bash
  npx gitnexus analyze --embeddings
  ```

---

## 緊急應變流程

### 情境1：系統故障（無法讀取 Market Data）

1. **立即行動**:
   ```bash
   # 檢查 Redis 連線
   redis-cli ping
   
   # 檢查 PostgreSQL 連線
   psql -h localhost -U atlas -c "SELECT 1"
   ```
2. **備用方案**: 切換至 Replay 模式（使用歷史資料）
3. **通知**: 發送告警至 Slack/Email

### 情境2：市場熔斷（單日跌幅 > 7%）

1. **立即行動**:
   - 檢查 `DrawdownGuard` 是否觸發紅色熔斷
   - 確認所有持倉已強制平倉
2. **評估**:
   - 檢查宏觀敘事：是否為系統性風險？
   - 檢查結構性趨勢：是否有豁免條件？
3. **決策**:
   - 若系統性風險：維持清倉，等待風險消退
   - 若結構性趨勢強：次日逐步重建倉位

### 情境3：外資異常大賣超（單日 > 500億）

1. **立即行動**:
   - 檢查 `MacroRiskAssessment`：是否升級至橙色/紅色？
   - 檢查日圓匯率：是否有 Carry Trade unwind 風險？
2. **評估**:
   - 檢查內資承接：投信是否買超？
   - 檢查 AI 營收：台積電是否有利空？
3. **決策**:
   - 若無結構性因素：啟動回撤機制
   - 若有結構性因素：維持倉位，密切監控

---

## 常用指令速查

```bash
# 主程式（HTTP server，預設 port 8080）
go run ./cmd/atlas

# 實驗生命週期
go run ./cmd/run-experiment -brief <file>
go run ./cmd/judge-experiment              # auto-discovers latest
go run ./cmd/promote-baseline              # auto-discovers latest accepted
go run ./cmd/revert-baseline --list

# 回測
go run ./cmd/backtest-window -start 2026-03-26 -end 2026-03-27

# 資料匯入（CSV → JSONL）
go run ./cmd/import-replay -source <csv> -target <jsonl>

# 品質檢查
test -z "$(gofmt -l .)"
go vet ./...
staticcheck ./...
go test ./...

# 覆蓋率
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total
```

---

## 監控與告警設定

### 關鍵指標

| 指標 | 閾值 | 告警等級 |
|------|------|---------|
| 組合單日回撤 | > 5% | Warning |
| 組合單日回撤 | > 10% | Critical |
| 外資單日賣超 | > 300億 | Warning |
| 外資單日賣超 | > 800億 | Critical |
| VIX | > 30 | Warning |
| VIX | > 40 | Critical |
| USD/JPY | < 145 | Warning |
| USD/JPY | < 140 | Critical |

### 告警渠道

- **Warning**: Slack #atlas-alerts
- **Critical**: Slack #atlas-alerts + Email + SMS

---

*技能版本: 1.0*  
*最後更新: 2026-04-23*
