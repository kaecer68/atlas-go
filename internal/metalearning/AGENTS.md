# AGENTS.md — internal/metalearning

**成熟度**: evolving
**模組職責**: 透過遺傳演算法演化學習策略，根據 MiroFish swarm 回饋持續優化策略參數。

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `MetaLearner` | `metalearner.go` | 元學習引擎：管理策略族群、精英選擇、交叉與突變 |
| `LearningStrategy` | `metalearner.go` | 單一學習策略（類型 + 參數 + 績效追蹤） |
| `StrategyPerformance` | `metalearner.go` | 策略績效指標（成功率、平均改善、收斂率、穩定度） |
| `SwarmLearningData` | `metalearner.go` | MiroFish swarm 回饋資料結構 |
| `TrainingResult` | `metalearner.go` | 單次訓練結果（改善幅度、收斂與否） |
| `bridge.go` | `bridge.go` | swarm.TrainingScenario → SwarmLearningData 轉換 |

---

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **冷啟動分數為 0** | `calculateStrategyScore()` 在 `TotalApplications == 0` 時回傳 0，新策略初期排名會墊底。 |
| **通道滿載丟棄** | `SubmitSwarmData` 與 `SubmitTrainingResult` 使用有緩衝 channel（容量 100），滿載時直接丟棄資料。 |
| **pseudo-random 為 deterministic** | 使用自訂 `deterministicRand`（LCG），`init()` 以 `time.Now().UnixNano()` 播種。測試結果可重現但非加密安全。 |
| **參數突變可能為負** | `mutateStrategy()` 對 `learning_rate` 與 `batch_size` 取絕對值，其他參數可能變為負數。 |
| **Save/Load 只存 ID 引用** | `Save()` 將 population 與 elite 存為 ID 列表；`Load()` 需從 `strategies` map 還原引用。 |

---

## 測試

- `go test ./internal/metalearning/...`
- 涵蓋 swarm 資料橋接、策略演化、Save/Load 持久化週期

(End of file - total 35 lines)
