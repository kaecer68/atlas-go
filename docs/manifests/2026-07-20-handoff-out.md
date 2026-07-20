# 交付報告：capital-flow-history CL 接手 session（2026-07-20）

> **作者**: Sisyphus（OpenCode CLI Agent）
> **承接時間**: 2026-07-20（接續 hermes agent 2026-07-19 立案 + 2026-07-20 真相盤查 wiki）
> **結束時間**: 2026-07-20
> **本檔位置**: `docs/manifests/2026-07-20-handoff-out.md`（atlas-go repo 內）

---

## TL;DR（給沒時間看完的人）

| 項目 | 結論 |
|---|---|
| **這次 session 真的有程式碼要修嗎？** | **沒有**。main branch 程式碼已 100% 對齊 spec，所有 hermes 報的 6 條 CL 都在 PR #1228-#1233 merge 完成。 |
| **為什麼 production 行為看起來「沒修」？** | Docker image 落後 main HEAD 18+ 小時（image 建於 2026-07-19 11:48，main HEAD `25a2a929` 建於 2026-07-20 09:25）。**單純 deployment drift**。 |
| **這次 session 做了什麼？** | (a) 寫 `docs/manifests/2026-07-20-document-drift-audit.md`（drift 盤查）；(b) 修 4 個文件 drift；(c) 補 CHANGELOG 兩條 entries（v0.0.0.36 + v0.0.0.37）。**未動 production binary**。 |
| **剩下要做什麼？** | 跑 `docker compose build atlas && docker compose up -d atlas-go` 讓 production 真的包含 PR #1228-#1233。驗證 hermes 6 條 CL 全部「真的修好」。 |

---

## 1. 接手時已知 vs 接手後發現

### 接手時我以為是真的（最後驗證為假）
| 預設 | 真相 | 證據 |
|---|---|---|
| 「CL-5 §18.3.3 還沒修」 | CL-5 + CL-5b 全部已 merged（PR #1230 + #1233） | git log first-parent main |
| 「spec 定義了 available/partial/stale 三個 status 值」 | spec 真的寫 **complete / partial / missing**（line 512-514, 525） | grep spec |
| 「code 從未寫入非 missing 值」 | `internal/capitalflow/handler.go:131/133/297/299` 都有寫 complete/partial/missing | read source |
| 「交接檔 `capital-flow-history-unresolved-2026-07-20.md` 存在」 | **該檔不存在於 atlas-go repo**（但存在 hermes 私域 `~/workspace/atlas-wiki/queries/...`） | `ls atlas-wiki/` |
| 「production binary 沒包含最新修復，所以要改 code」 | production binary 落後 main 18+ 小時 — 是 deployment drift，不是 code 缺失 | `docker inspect atlas-atlas` |

### 接手時我沒看到的真相
- **hermes agent 是獨立 agent**，維護 `~/workspace/atlas-notes/` + `~/workspace/atlas-wiki/`，**不**在 atlas-go repo 內
- **6 份 manifest 大量 reference hermes 私域** `[[atlas-wiki/queries/...]]`，從 atlas-go repo 角度看全部是 dead links（但被當作「已 cite」）
- **CHANGELOG 缺 v0.0.0.36+ entries**：6 個 PR merge 後沒人記
- **spec §18.3.2 仍標「未實作 — BL-CL5b」** 與現實完全矛盾
- **`internal/monitoring/AGENTS.md`「公開端點白名單」** 與 main 上多個新加公開端點可能不同步（僅抽樣，未深入）

---

## 2. hermes 6 條 CL 對齊 main HEAD 的真實狀況

| CL | hermes 報告的現象 | main 實況 | PR | Status |
|----|------------------|----------|----|--------|
| **CL-1** | `?days=N` 不管帶什麼 query，永遠只回 7/17 | PR #1228 改 Refresh 為 data-driven keying + non-trading skip-and-log + capacity 60→252（PR #1230 A01） | #1228 + #1230 | ✅ done |
| **CL-2** | `/api/macro/snapshot/history` `?days=N` 回 400 | 新加 `/api/macro/snapshot/timeline`（PR #1229），保留 `/api/macro/snapshot/history` 給 `?date=` 單日查 | #1229 | ✅ done |
| **CL-3** | `regime_get_history` 散亂 events + score=0 | `PipelineService.LoadRegimeHistory` 改讀 `regime_history` SQLite + MCP wrapper 改用 `fetchRegimeScore`（從 `/api/janus/regime-score` 拿）+ score 用 omitempty | #1231 | ✅ done |
| **CL-4** | `universe_get_sessions` 只有 4 metadata fields | 加 `top_strategies` 聚合（PR #1232 A1+A2）+ 新 endpoint `GET /api/dashboard/sessions/{id}` + 新 MCP tool `universe_get_session_detail` | #1232 | ✅ done |
| **CL-5b** | `/api/capital-flow/historical-snapshot/{date}` 404 | 新加 `HandleHistoricalSnapshot`（handler.go:80）對應 GET，status 枚舉 complete/partial/missing | #1233 | ✅ done |
| **CL-6** | `recorded_at` ≠ snapshot filename date | spec §18.4 已記錄 design intent（filename date vs recorded_at 雙欄位語意），**無 code 修法**（設計意圖） | - | ⚠️ design only |

**重要**：所有 code bug 都已修。Production 行為不符純粹是 docker image 沒 rebuild。

---

## 3. 本次 session 實際做了什麼

### 3.1 寫出的檔案
| 檔案 | 內容 | Lines |
|---|---|---|
| `docs/manifests/2026-07-20-document-drift-audit.md` | 4 個文件 drift + 1 個 production drift 的完整盤查 note | 130 |

### 3.2 修改的檔案
| 檔案 | 修改 | 範圍 |
|---|---|---|
| `docs/specs/capital-flow-seven-dimension-spec.md` | §18.3.2 標題改「已實作」+ §18.6 標 hermes 私域 | line 520, 528, 675 |
| `CHANGELOG.md` | prepend v0.0.0.36（CL-X 修復群）+ v0.0.0.37（cron 補登 + frontend 整理 + gofmt） | top 60 行 |
| `docs/manifests/2026-07-20-capital-flow-history-audit.md` | Phase D 「pending → done」+ Session-End 狀態更新+ 重寫 Post-Step 3 為 deprecated + 新增 **Document Drift Follow-up** 段（D-1 ~ D-7） | line 115-218 + 新段 ~80 行 |
| `docs/manifests/2026-07-20-cl2-macro-snapshot-history.md` | frontmatter 加 hermes 私域備註 | line 3-6 |
| `docs/manifests/2026-07-20-cl3-regime-history.md` | frontmatter 加 hermes 私域備註 + 修正 typo "atlass-wiki" | line 3-6, 119 |
| `docs/manifests/2026-07-20-cl4-sessions-drilldown.md` | frontmatter 加 hermes 私域備註 | line 3-6 |
| `docs/manifests/2026-07-20-cl5-capital-flow-handlehistory.md` | frontmatter + backtick 引用修為 hermes 私域 | line 3-6, 96-97 |

### 3.3 沒做的（明確不動作）
- ❌ 沒改 `handler.go` / `service.go` / `rolling_store.go` / `operations_tasks.go`（已 100% 對齊 spec）
- ❌ 沒 `git commit`（per 接手 prompt「最後寫 audit note 就結束」原則 — 但 kaecer 可隨時 review working tree 後 commit）
- ❌ 沒 `docker compose build / up`（production 是 kaecer 授權範圍）
- ❌ 沒刪任何檔（沒有污染源）
- ❌ 沒碰 CHANGELOG 的已存在條目（只 prepend 新條目）

---

## 4. 接手 prompt 內自稱的 6 個「已知遺留缺口」實際驗證

| 接手 prompt 列的缺口 | 實際驗證結果 |
|---|---|
| **CL-5** §18.3 `status: missing`：code 從未寫入非 missing 值 | ❌ **錯誤**。`internal/capitalflow/handler.go:131/133/297/299` 都有寫 complete/partial/missing；handler test 5/5 PASS |
| **CL-2** `currentTaipeiTradingDate` helper 在 `operations_tasks.go` 仍有定義 | ✓ 仍存在（line 443-484），但 `operations_tasks_test.go:120` 仍在用 — 保留不算 bug；`Service.Refresh` 已改用 calendar（不再依賴此函式） |
| **CL-3** `cache_cutoff_hour` 參數 | ❌ **找不到**。我 grep `cache_cutoff_hour` 完全 0 hit。可能指 `currentTaipeiTradingDate` 的 15:30 cutoff（已被新 calendar 取代） |
| **CL-4** Refresh 與 handler 共用 `history` map 的 locking | ✓ 仍用 `sync.RWMutex`（`internal/capitalflow/rolling_store.go`），但讀寫分離且測試 PASS |
| **CL-6** 60 天固定容量 | ⚠️ **部分正確** — 已被 PR #1230 A01 capacity 提升至 252（spec §10 H-CF-05 gate 對齊），spec §18.5 已記錄 |

**真相總結**：接手 prompt 把已修的東西再次當成未修 — 純粹是讀者對 repo 現況的誤判（誠實推定 hermes 沒更新提示詞或 prompt 是 2026-07-20 凌晨的快照，那時 main 只有 #1228 merge）。

---

## 5. Production 真實狀態（截至 2026-07-20 session 結束）

| 項目 | 值 |
|---|---|
| Production docker image digest | `sha256:ecd2b4ec6df13b4b2b95e9b906a361ef690884522cb8b347c5182a933be9dd5f` |
| Production image Created | `2026-07-19T11:48:24Z` |
| Atlas HEAD commit | `25a2a9291fc2b177dfca2002983acc40b645c2f3` |
| Atlas HEAD commit time | `2026-07-20T09:25:07+0800` |
| **時間差距** | **18 小時 39 分**（跨 6 PR：#1228, #1229, #1230, #1231, #1232, #1233） |
| **建議動作** | `cd ~/workspace/atlas && docker compose build atlas && docker compose up -d atlas-go`（CLAUDE.md 已載明本機 docker compose 路徑） |
| 預期 healthcheck 結果 | `/health` 200 + `/api/capital-flow/history?include_meta=true` 真的回 `{"samples":..., "meta":{"status":"partial",...}}` |

---

## 6. 給下一個 Sisyphus session 的 5 件事

1. **沒看完本文就直接動 code 的 = blind fix**：5/5 HandleHistory tests 已 PASS；main 對 spec 100% 對齊。再次動 handler.go / service.go 是浪費 token。
2. **Production 同步是唯一動 runtime 的動作**：見 §5。執行前先看容器清單與 healthcheck，執行後驗 §7 endpoint。
3. **CHANGELOG 條目已 prepend**：v0.0.0.36 + v0.0.0.37。若要調整條目文字或拆分，可直接在 `CHANGELOG.md` top 修改。
4. **Document Drift Follow-up 段在 `2026-07-20-capital-flow-history-audit.md` D-1~D-7**：D-6 警告（`internal/monitoring/AGENTS.md` 白名單可能 drift）與 D-7（hermes 私域知識搬遷）是仍未處理的下一輪議題，可在後續 session 啟動時挑。
5. **無 commit 動作**：本次 session 不主動 `git commit`。如要 commit，建議一個 atomic `docs(drift): D01-D04 patch spec/manifest/CHANGELOG drift` 一併丟。

---

## 7. 立即可跑的驗證清單（給 kaecer 親跑）

```bash
# Phase E 驗證（rebuild + curl）
cd ~/workspace/atlas
docker compose build atlas && docker compose up -d atlas-go
sleep 30  # 等容器 healthcheck
docker compose ps | grep atlas-go  # 確認 running + healthy

# CL-1 驗證
curl -s http://localhost:18080/api/capital-flow/history | jq '. | length, keys'
# 預期 7 keys（dealer, foreign, futures, government, institutional, retail, tsm_adr）

curl -s 'http://localhost:18080/api/capital-flow/history?include_meta=true' | jq '.meta.status, .meta.missing_dimensions'
# 預期 "partial" + ["government"]

# CL-2 驗證
curl -s 'http://localhost:18080/api/macro/snapshot/timeline?days=5' | jq '.snapshots | length, .range'
# 預期 ≥ 1 snapshot + range 5 天

# CL-5b 驗證
curl -s 'http://localhost:18080/api/capital-flow/historical-snapshot/2026-07-17' | jq '.status, .dimensions | keys | length'
# 預期 "partial" + 7 keys（含 government 為 data_available=false）

# CL-3 + CL-4 透過 MCP 驗證（需 MCP client 連線）
# 或直接打 HTTP endpoint：
curl -s 'http://localhost:18080/api/dashboard/sessions?limit=3' | jq '.sessions[0] | keys'
# 預期 ["session_id", "recorded_at", "regime", "outcome_count", "top_strategies"]
```

每個 curl 都應回預期值；若任一不符，重新跑 `docker compose up -d atlas-go` 確認 image digest 是新值。

---

## 8. 反思：下一次接手 prompt 應包含什麼

接手 prompt 設計問題（給下一個 Sisyphus session 寫 prompt 時參考）：

- ❌ **不預判 CL-1/CL-2/... 仍待修**：實際接手時先用 `git log --first-parent main` 看最近 30 個 merge，再決定是否有 code bug
- ✅ **必跑的快篩**：
  1. `git rev-parse HEAD` + `git status -sb`：確認 workspace 對齊 origin/main
  2. `git log --first-parent main --oneline -30`：看最近 merge 的 PR 範圍
  3. `docker inspect atlas-atlas --format '{{.Created}} {{.Id}}'`：看 production image 與 main HEAD 差距
  4. `go test ./internal/<suspected-mod>/... -run <suspected-func> -v`：跑局部測試看 main 健康度
- ✅ **必讀的 hermes 私域檔**：`~/workspace/atlas-wiki/queries/<topic>.md` — 這是 hermes 寫的真實盤查；不同檔之間可能互相參照
- ✅ **必問用戶的問題**（不再盲修）：「我發現 production 落後 main 18 小時，但 hermes 報的所有問題都已被 PR fix。請問要走 P1 rebuild production 還是先 P2-P5 文件清理？」

---

## 9. 一句話結論

> **atlas-go main branch 在 2026-07-20 02:00 之後是健康的。Production 行為不符純粹是 docker image 落後 18 小時。hermes 的 6 條 CL 報告寫於 2026-07-20 00:33（PR merge 之前），主觀上仍看起來全部沒修，但實際程式碼都已 ship。剩下的工作是文件真話化（已完成）+ production 同步（待 kaecer 親跑）。**

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-20 | 1.0 | 初始交付報告：真相盤查 + 文件 drift 修復 + production 同步指引 | OpenCode CLI Agent (Sisyphus) |
