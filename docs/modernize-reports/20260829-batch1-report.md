# atlas modernize 批次報告 (2026-08-29)

分支: `atlas-modernize-20260829` (未 push master) | 工具: `~/go/bin/modernize` (golang.org/x/tools modernize, go1.26.4)

## 1. 摘要

- modernize 診斷總數: **1412 → 350** (減少 1062)
- 完成 Top 3 類: rangeint (689)、any (135)、newexpr (249, 含 boolPtr→new(x) 125)
- 4 個 commit, 每批可獨立 revert
- 最終驗證: `go vet ./...` 0 issue、`golangci-lint run --timeout=5m ./...` 0 issue、`go test ./...` 165 ok / 0 FAIL

## 2. 分類統計 (unique diagnostics, 去除 package/test 重複)

| Analyzer | 改前 | 改後 | 已處理 | 說明 |
|---|---:|---:|---:|---|
| rangeint | 689 | 11 | 678 | 剩 11 處刻意保留(見 §5 風險) |
| newexpr | 249 | 0 | 249 | 231 call sites + 14 helper 改寫 + 4 helper 刪除 |
| any | 135 | 0 | 135 | 另 3 處在 build-tag/註解內, 非 AST 可達 |
| omitzero | 78 | 78 | 0 |  |
| minmax | 38 | 38 | 0 |  |
| waitgroupgo | 33 | 33 | 0 |  |
| mapsloop | 32 | 32 | 0 |  |
| slicescontains | 30 | 30 | 0 |  |
| stringscutprefix | 30 | 30 | 0 |  |
| atomictypes | 24 | 24 | 0 |  |
| stringsseq | 22 | 22 | 0 |  |
| slicesbackward | 15 | 15 | 0 |  |
| testingcontext | 9 | 9 | 0 |  |
| forvar | 8 | 8 | 0 |  |
| stringsbuilder | 6 | 6 | 0 |  |
| stringscut | 6 | 6 | 0 |  |
| slicessort | 4 | 4 | 0 |  |
| stditerators | 2 | 2 | 0 |  |
| errorsastype | 2 | 2 | 0 |  |

## 3. Commit 清單

| Commit | 內容 | 檔案數 | +/- |
|---|---|---:|---:|
| `1619f8ba` | batch1: range-over-int (678 處) | 182 | +678/-678 |
| `7763a318` | batch2: interface{}→any (135 處) + risk API golden 更新 | 47 | +167/-167 |
| `60333a5d` | batch3: new-like helper → new(expr) (231 call sites) | 52 | +227/-224 |
| `7d6ec66c` | batch1-followup: 2 處暴露出的 range-int 轉換 | 2 | +2/-2 |

整支 diff (vs main): **272 files, +1074/-1071**

## 4. 每批驗證結果

| 批次 | go vet ./... | golangci-lint | go test ./... |
|---|---:|---:|---:|
| baseline (改前) | 0 | 0 issues | 165 ok / 0 FAIL |
| batch1 rangeint | 0 | 0 issues * | 165 ok / 0 FAIL |
| batch2 any | 0 | 0 issues | 165 ok / 0 FAIL |
| batch3 newexpr | 0 | 0 issues | 165 ok / 0 FAIL |
| **最終 (4 commits 全含)** | **0** | **0 issues** | **165 ok / 0 FAIL** |

\* batch1 初版曾出現 9 個 gosec G602 新 issue, 已處理(見 §5)。

## 5. 風險評估

### 5.1 rangeint (range-over-int)
- modernize 的 rangeint analyzer 本身有嚴格限制: limit 必須是 loop-invariant (常數 / 只賦值一次的 local / unexported global, 或 len(slice)), 且 index 在迴圈內不得被賦值或取址 → 轉換語意等價。
- 但 gosec G602 對 `for i := range K` (K 為變數) 無法證明 slice index 邊界, 產生 9 個 false positive (`internal/risk/spillover.go` ×6、`internal/config/bayesian_optimizer.go` ×2、`internal/ml/randomforest.go` ×1)。
  - 處理: 這些迴圈**保留原 3-clause 寫法**(共 11 處), 不加 nolint; 維持 lint 0 issue。
  - 影響: rangeint 剩 11 個診斷, 為刻意保留。
- batch1 副作用: 外層迴圈轉成 range 後, 內層以「外層 range 變數」為 limit 的迴圈變成可轉換(2 處), 已補上(`7d6ec66c`), 語意等價(limit 每次進入內層迴圈時求值一次)。

### 5.2 any (interface{} → any)
- `any` 與 `interface{}` 是同一型別, 純文字替換, 風險趨近零。
- 唯一非預期: `internal/risk` 的 API snapshot golden (`risk_api.{golden,actual}.json`) 用 go/types 印出型別字串, go1.26 印 `any` 而非 `interface{}` → golden 更新(4 行, 僅型別字串表示, 無 API 變化)。
- 剩 3 處 interface{}: `postgres_historical_test.go` (`//go:build integration`, 不在預設 build)、2 處在註解內。

### 5.3 newexpr (new-like helper → new(expr))
- 工具檢查 `new` 未被 shadow、回傳型別與參數型別嚴格一致後才替換 → 語意等價。
- 4 個 production helper (boolPtr×2、intPtr、float64Ptr) 在全部 call site 替換後變成 dead code, 已機械性刪除(先 grep 確認全 tree 無殘留參考, 含 build-tag 檔案)。
- 14 個 test-file helper 改寫為 `new(expr)` + `//go:fix inline` (golangci 排除 `_test.go`, 不影響 lint)。

### 5.4 測試衝擊
- 每批後跑完整 `go test ./...`, 皆 165 ok / 0 FAIL(最終再跑一次確認)。
- 未新增/修改任何測試邏輯; 唯一 testdata 變更為上述 risk API golden。

## 6. 待辦 (剩餘 350 診斷, 未處理)

| Analyzer | 數量 | 預估風險 |
|---|---:|---|
| omitzero | 78 | 中(需確認 omitempty 語意, 與 JSON 行為相關) |
| minmax | 38 | 低(if/else → min/max) |
| waitgroupgo | 33 | 低(wg.Add/go/Done → wg.Go) |
| mapsloop | 32 | 低 |
| stringscutprefix | 30 | 低 |
| slicescontains | 30 | 低 |
| atomictypes | 24 | 中(需確認 atomic 型別轉換) |
| stringsseq | 22 | 低 |
| slicesbackward | 15 | 低 |
| rangeint | 11 | — (刻意保留, gosec G602 相容) |
| testingcontext | 9 | 低(t.Context) |
| forvar | 8 | 低 |
| stringscut / stringsbuilder / slicessort | 6/6/4 | 低 |
| errorsastype / stditerators | 2/2 | 低 |

建議下一批: minmax + stringscutprefix + mapsloop + slicescontains (低風險, 約 130 處)。omitzero 與 atomictypes 需逐處人工確認語意。