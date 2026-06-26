# Fubon Proxy IPv4/IPv6 Dual-Stack 問題調查（2026-06）

> **文件角色**：根因調查（RCA）。  
> **來源**：原記載於 `internal/marketdata/AGENTS.md` 與 `internal/fubonproxy/AGENTS.md`，因屬歷史調查紀錄搬遷至此。  
> **狀態**：已解決（PR #495 / PR #572）。

## 背景

`FubonClient` 與 `HybridProvider` 預設使用 IPv4 `127.0.0.1:8081` 而非 `localhost:8081`。原因是 macOS / Linux 雙棧環境下，Go `net.Dial` 對 `localhost` 預設優先走 IPv6 `[::1]`，會撞上 Python proxy 若僅綁 IPv4 的情境。IPv4 `127.0.0.1` 解析無歧義，跨平台行為一致。

## 問題：PR #495 的雙棧修正引發 IPv4 連線被拒

先前 PR #495 將 Python proxy 的 `host="0.0.0.0"` 改為 `host="::"`，想做「雙棧防禦」。但因 uvicorn 自動使用 uvloop，而 uvloop 在 IPv6 socket 上強制 `IPV6_V6ONLY=1`，導致實際只接受 IPv6、IPv4（含本 Go client）連線被拒絕。

## 解決方案

- 將 `services/fubon-proxy/main.py` 改回 `host="0.0.0.0"`
- 從 `requirements.txt` 移除 `uvicorn[standard]` 的 uvloop 隱含依賴
- `FUBON_PROXY_URL` 環境變數已於 PR #572 移除，proxy 位址固定為 `127.0.0.1:8081`，不再有 IPv6 雙棧歧義

## 現行規則

詳見 `internal/marketdata/AGENTS.md` 與 `internal/fubonproxy/AGENTS.md` 的「fubonproxy 連線位址」段。
