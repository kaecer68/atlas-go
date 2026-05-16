# Gateway Migration Tracking

## 目標

將所有分散在各處的直接 Provider/Client 實例化遷移至統一的 `Gateway` 模式。

## 完整 TODO 清單（共 27 個）

### `internal/orchestrator/system.go`（5個）

- [ ] `system.go:459` — Migrate to Gateway for direct Fugle provider instantiation
- [ ] `system.go:463` — Migrate to Gateway for direct mock provider instantiation
- [ ] `system.go:467` — Migrate to Gateway for direct TWSE OpenAPI provider instantiation
- [ ] `system.go:470` — Migrate to Gateway for direct hybrid provider instantiation
- [ ] `system.go:473` — Migrate to Gateway for direct hybrid provider instantiation

### `internal/orchestrator/composition.go`（1個）

- [ ] `composition.go:105` — Migrate to Gateway for direct sector data provider instantiation

### `internal/monitoring/dashboard_api.go`（16個）

- [ ] `dashboard_api.go:127` — Migrate to Gateway for direct Yahoo Finance macro provider instantiation
- [ ] `dashboard_api.go:129` — Migrate to Gateway for direct Frankfurter FX provider instantiation
- [ ] `dashboard_api.go:131` — Migrate to Gateway for direct SOX index provider instantiation
- [ ] `dashboard_api.go:133` — Migrate to Gateway for direct TWSE capital flow provider instantiation
- [ ] `dashboard_api.go:135` — Migrate to Gateway for direct TWSE margin balance provider instantiation
- [ ] `dashboard_api.go:137` — Migrate to Gateway for direct export statistics provider instantiation
- [ ] `dashboard_api.go:141` — Migrate to Gateway for direct sector data provider instantiation
- [ ] `dashboard_api.go:146` — Migrate to Gateway for direct TSMC revenue provider instantiation
- [ ] `dashboard_api.go:149` — Migrate to Gateway for direct composite macro provider instantiation
- [ ] `dashboard_api.go:151` — Migrate to Gateway for direct geopolitical composite provider instantiation
- [ ] `dashboard_api.go:153` — Migrate to Gateway for direct RSS geopolitical provider instantiation
- [ ] `dashboard_api.go:155` — Migrate to Gateway for direct GDELT geopolitical provider instantiation
- [ ] `dashboard_api.go:158` — Migrate to Gateway for direct Taiwan geopolitical composite provider instantiation
- [ ] `dashboard_api.go:160` — Migrate to Gateway for direct Taiwan RSS geopolitical provider instantiation
- [ ] `dashboard_api.go:423` — Migrate to Gateway for direct FinMind client instantiation
- [ ] `dashboard_api.go:426` — Migrate to Gateway for direct FinMind dividend provider instantiation

### `internal/monitoring/service/data_channels.go`（3個）

- [ ] `data_channels.go:132` — Migrate to Gateway for direct Fugle client instantiation
- [ ] `data_channels.go:184` — Migrate to Gateway for direct Fubon client instantiation
- [ ] `data_channels.go:236` — Migrate to Gateway for direct FinMind client instantiation

### `internal/monitoring/api/system/handlers.go`（1個）

- [ ] `handlers.go:146` — Migrate to Gateway for direct day trading provider instantiation

### `cmd/experimental/validate-twse-capital-flow/main.go`（1個）

- [ ] `main.go:23` — Migrate to Gateway for direct TWSE capital flow provider instantiation

---

## 優先級建議

| 優先級 | 範圍 | 數量 |
|--------|------|------|
| **High** | `internal/orchestrator/`（核心協調層） | 6 |
| **Medium** | `internal/monitoring/dashboard_api.go`（監控 API） | 16 |
| **Low** | `internal/monitoring/service/`, `internal/monitoring/api/system/`, `experimental/` | 5 |

> 建立時間: 2026-05-16
> 來源: 系統孤兒/過時檔案掃描
