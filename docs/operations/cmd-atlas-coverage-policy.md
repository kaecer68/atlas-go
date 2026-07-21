# cmd/atlas Coverage 排除政策

> **產出時點**：2026-07-01
> **作者**：Sisyphus Atlas Agent
> **產出任務**：`.omo/plans/ci-verify-verification-2026-07-01.md` T-202
> **對應 CI 設定**：`quality.yml` `coverage:` job

## 1. 現狀

CI coverage gate（`quality.yml`）的測試指令為：

```bash
go test -coverprofile=coverage.out $(go list ./... | grep -v '/cmd/atlas$')
```

**`cmd/atlas` 套件被排除在覆蓋率測量之外**。同樣的 `grep -v` 也出現在 `ci.yml` 的 `coverage:` job。

## 2. 排除原因

### 2.1 Heavy Integration Cost

`cmd/atlas` 是 atlas 的 binary entrypoint，目錄下有 **10 個 `_test.go` 檔案**（2026-07-01 觀察）：
`main_test.go`（內含 4 個 live-broker 測試）、`live_mode_test.go`、`api_routes_test.go`、`main_api_test.go`、`load_calibration_orders_test.go`、`run_live_test.go`、`run_simulation_test.go`、`run_simulation_mode_test.go`、`simulation_mode_test.go`、`storage_route_test.go`。

其中 live-broker 測試子集（`main_test.go` line 194/281/346 + `live_mode_test.go`）需要：
- 啟動完整的 Go process
- 啟動 HTTP server on port 18080
- 連線 PostgreSQL（用 docker exec repair 機制）
- 連線 Redis
- 連線 fubon-proxy（可能 live-broker）
- 執行 preflight check

每個測試需要 0.5-5 秒，4 個 live-broker 測試合計 ~5-8 秒。當 coverage 收集器（`-coverprofile`）介入時，runtime 顯著拉長（觀察到 +30%~50% overhead）。**其他 8 個 test file 較輕量**，理論上可獨立納入 coverage，但會引入測試矩陣複雜度（見 §2.3）。

### 2.2 Live-Broker Safety 設計

`cmd/atlas` 的測試包含 `TestRunAllowsLiveBroker*` 與 `TestLiveModeAcceptsDryRunBroker`，這些測試設計上是驗證「flag chain 對 live-broker 的處理」。它們刻意 spawn 真實的 atlas binary 來測 preflight 邏輯。

將這層測試納入 coverage 收集會：
- 拉長 CI 時間（已 18-30s，會到 30-50s）
- 在 coverage binary instrumentation 環境下，flag 解析與 timing 可能與 prod 不一致，誤判 preflight 行為
- coverage instrumentation 增加 attack surface 進入 live-broker test 路徑（雖然理論上隔離）

### 2.3 Port 18080 衝突的高敏感度

`cmd/atlas` 測試需要獨占 `port 18080`。當 coverage instrumentation 介入時，可能在 coverage dump 階段啟動額外的 process，導致 port 衝突與 flaky test。

## 3. 已知後果

排除 `cmd/atlas` 意味著：
- 整體 coverage 從「包含 cmd/atlas」變成「internal/* only」
- 2026-07-01 觀察值：**67.2% 總體**（含 `internal/testutil/snapshot` 拉低）
- 移除排除的 cmd/atlas 預估會再降 5-10 個百分點（該套件主要是 flag/main 邏輯）

這是**已知設計 trade-off**，不是 bug。

## 4. 何時該重新評估

下列條件**任一**成立時，應該重新評估排除政策：

1. **PostgreSQL test infra race condition 修好後**（見 `.omo/plans/ci-verify-verification-2026-07-01.md` T-103）
   - 4 個 live-broker 測試不再 flaky 時，可考慮收入 coverage
2. **port 18080 衝突機制強化**（T-104: TestMain 加 `ATLAS_PORT_OVERRIDE` skip 機制）
   - 環境可重現時，coverage 收集更穩定
3. **cmd/atlas 內部邏輯大量擴充**（>20% Go 程式碼成長）
   - 排除成本超過整合成本時
4. **新增 capability 需要 end-to-end 測量**
   - 例如 LLM provider 切換、concurrent flag chain 等

## 5. 重新加入的具體步驟

若決定重新加入：

1. 移除 `grep -v '/cmd/atlas$'`（`ci.yml` + `quality.yml` 兩處）
2. 確認 4 個 live-broker 測試在 coverage 下穩定（先 5 次本地 re-run）
3. 評估整體 coverage drop 是否 < 5%（若是，coverage gate 仍 ≥ 60%）
4. 若 drop > 5%：
   - 為 cmd/atlas 補 unit test 把覆蓋率拉回（注意：flag chain 邏輯獨立 unit test 比 end-to-end test 容易）
   - 或下調整體 coverage gate 到 55%（最後手段）

## 6. 參考資料

- `.omo/plans/ci-verify-2026-07-01.md` § 1.1（環境狀態）
- `.omo/plans/ci-verify-verification-2026-07-01.md` Stream 7（T-701, T-702）
- `.github/workflows/ci.yml` `coverage:` job（line 54–75）
- `.github/workflows/quality.yml` `coverage:` job（line 112–142）
- `cmd/atlas/main_test.go` line 194/281/346（3 個 live-broker 測試）
- `cmd/atlas/live_mode_test.go`（TestLiveModeAcceptsDryRunBroker）
- `cmd/atlas/` 其他 8 個 `_test.go`（api_routes / main_api / load_calibration_orders / run_live / run_simulation / run_simulation_mode / simulation_mode / storage_route）
- （內部審計，`.omo/audit/`）
- `docs/silicon-indicators-coverage.md`（矽循環指標，**不衝突**，主題不同）
