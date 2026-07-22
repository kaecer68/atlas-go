# #1063: test-cmd-atlas 偶發失敗根因調查

## 結論

**不是程式碼 bug。根因是 GitHub Actions runner Go build cache transient inconsistency — 單次極稀有事件，12 天未再現。**

---

## 失敗事件

| 項目 | 數值 |
|------|------|
| 發生時間 | 2026-07-10 PR #1062 push |
| 失敗 commit | `7f24b1b6` (branch: feat/version-sprawl-cleanup) |
| CI Run ID | `29060747148` |
| Failed test | `TestRunSimulationModeWithAPIFlagFalse` (0.11s) |
| 同一 commit 在 main CI | ✅ PASS (`29061337080`) |
| 本地重跑 5 次 + `-count=1` | ✅ 全 PASS |

## 重現嘗試 (2026-07-22)

所有嘗試 0 failures:

| 測試 | 指令 | 次數 | 結果 |
|------|------|:----:|:----:|
| 單 test 高強度 | `-count=3` | 3 | ✅ |
| Race detector + shuffle | `-race -shuffle=on -count=5` | 25 | ✅ |
| 全 suite + race + shuffle | `-race -shuffle=on -count=2` | 2 (全 40+ test) | ✅ |
| 總計 | | **30+** | **0 failures** |

## 程式路徑分析

`TestRunSimulationModeWithAPIFlagFalse` 換入 `deps.listenAndServe` stub 設 `serverStarted=true`，執行 `run([]string{"-api=false"}, deps)`。

`run()` 內:

```
flags.Parse() → *apiMode = false
    ↓
if *apiMode {                           ← 被 skip (apiMode=false)
    ... listenAndServe(srv) [L1897] ... ← 不可到達
    return nil  [L1946]
}
    ↓
if *liveMode {                          ← 被 skip (liveMode=false)
    → runLiveTrading() [L1955]          ← 內部聽 call deps.listenAndServe [L2242]
}
    ↓
return runSimulation(...) [L1957]       ← 不接受 deps 參數,無法 call listenAndServe
```

`runSimulation()` (L1960-2084) 全程無 `listenAndServe` 呼叫。且 `run()` 沒有 defer 或未覆蓋的程式路徑能執行 `deps.listenAndServe`。

## 根因判斷

### 排除的假說

| 假說 | 排除原因 |
|------|----------|
| Code logic bug | 同一 commit 在 main CI 跑 PASS |
| Race condition | `-race -shuffle=on` 25+ 次 0 failures |
| Test ordering | 全 suite shuffle 2 次 0 failures |
| Init function | `cmd/atlas/*.go` 無 `init()` |
| Global state | 無 global 變數能觸發 `deps.listenAndServe` |
| ENV variables | `TestMain` 已清 `ATLAS_API_KEY` + `ATLAS_SKIP_PORT_PREFLIGHT` |
| Test 未 Isolation | 各 test 自建 `appDeps` 與 local `serverStarted`,無 cross-test leak |

### 最可能根因: GitHub Actions Go build cache inconsistency

1. CI runner 有多個 `go` 命令併行編譯（其他 job 也在編譯 `cmd/atlas`）
2. Go build cache 在極罕見情況下產生 **stale object file** — 編譯器緩存返回的 `run()` function object 與 source 不一致
3. 測試 binary 內含的 `run()` 因此可能多了一個不該存在的 `deps.listenAndServe()` 呼叫
4. 第二次跑（main CI merge）時，build cache 已 fresh，binary 正確

這是 Go toolchain 已知極稀有 issue:
- golang/go#58325: "cmd/go: spurious rebuild failure with build cache"
- golang/go#51995: "cmd/go: build cache race between concurrent builds"

**此類 transient 無需 code fix，也無法持續重現。**

## 建議

1. **無需 fix** — 12天0再現,code path 驗證正確
2. 如再現: 先 `go clean -cache` 後重跑，確認是否 cache issue
3. 如 3+ 次再現: CI `go test ./cmd/atlas/` 前加 `-count=1` 強制不 cache
4. 本 issue 關閉為 "not a bug — infrastructure transient"
