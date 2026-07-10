# atlas-mcp Tier Guide

> **⚠️ 本文件封存在 [#1068](https://github.com/kaecer68/atlas-go/issues/1068)**
> atlas-mcp 的 tier 模型（Public / Free / Premium / Admin）定義於
> `docs/operations/tier-boundary.md`（尚未公開）。商業化上線後，本指南
> 會說明每個 tier 包含哪些 tool、價格、以及如何從 hermes 接入。

## 在此之前

- 所有 91 tools 可透過共用 dev key（`~/.config/atlas-go/.env`）免費使用
- 見 `make setup-mcp-agent`（PR #1069）
- 🔴 共用 dev key 不能用於 production live trading

## 參考

- [#1068 Commercial flow: API key registration + tier gating]
  (https://github.com/kaecer68/atlas-go/issues/1068)
- `docs/operations/tier-boundary.md` — 現有 4-tier 定義（未公開）
