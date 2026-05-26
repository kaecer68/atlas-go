# experimental/ TODO Audit — 2026-05-26

## 檔案: cmd/experimental/validate-twse-capital-flow/main.go

### TODO (line 23)
```go
// TODO: Migrate to Gateway for direct TWSE capital flow provider instantiation.
```

### 評估結果: 低優先，可保留

1. **此檔案屬於實驗驗證工具**（`cmd/experimental/AGENTS.md`），非生產路徑。
2. `TWSECapitalFlowProvider` 是 local file reader（讀取 `data/state/capital_flow`），非網路 client。
3. Gateway migration tracking（`docs/GATEWAY_MIGRATION_TRACKING.md`）將其歸類為 Wave 1 Legacy — 非生產路徑，低優先。
4. 其他實驗命令（如 `validate-stress-index`）也使用類似的直接 provider 模式，保持一致性。

### 建議
- **不立即處理**。保留 TODO 作為備忘。
- 若未來決定統一實驗命令的 provider 建立模式，可一併處理所有 `cmd/experimental/` 下的直接 provider 使用。

## 其他實驗命令檢查

| 命令 | Provider 使用 | Gateway 遷移需求 |
|------|--------------|-----------------|
| `validate-twse-capital-flow` | `NewTWSECapitalFlowProvider` | 低（local file reader） |
| `validate-stress-index` | 無直接 provider | 無 |
| `validate-broker` | 無直接 provider | 無 |
| `validate-narrative-shock` | 無直接 provider | 無 |
| `staging-drill` | 使用 production path | 無（已通過 Gateway） |
| `janus-backtest` | 使用 production path | 無（已通過 Gateway） |
