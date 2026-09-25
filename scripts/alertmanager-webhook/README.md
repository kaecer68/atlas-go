# scripts/alertmanager-webhook — Alertmanager → Telegram 告警轉送

一支小 relay：`Alertmanager (webhook :9095)` → 本腳本 → `Telegram sendMessage`。
用 launchd 常駐（label `com.goluck.atlas-webhook-to-telegram`）。

## ⛔ 機密規則（本 repo 是 **PUBLIC**）

| 檔案 | 是否可含 token | 說明 |
|---|---|---|
| `com.goluck.atlas-webhook-to-telegram.plist`（repo） | **不可** ✗ | 只放 placeholder `__INJECT_AT_INSTALL__` |
| `~/Library/LaunchAgents/com.goluck.atlas-webhook-to-telegram.plist`（安裝版） | 可 ✓ | 由安裝腳本在**安裝期**注入，`chmod 600` |

**事故背景（2026-09-25）**：這個 plist 從 **2026-08-27** 起就把**明文 bot token** 寫進版控，
而本 repo 是 public ⇒ 該 token 等於公開；等它失效後，告警鏈沉默了 **41 小時 59 分**才被發現
（Alertmanager → webhook → Telegram 三段任一斷掉，本地幾乎沒有跡象）。此後：
**機密不進版控**由 `scripts/ci/check_secrets.sh` 在 CI 強制把關（詳見該檔與 `scripts/secret-scan.sh`）。

### 為什麼是「安裝期注入 plist」而不是改成 `TELEGRAM_BOT_TOKEN_FILE`？

因為 **a2a-dev 的 `macmini-recover` #50 會從已安裝 plist 的 `EnvironmentVariables`
讀 `TELEGRAM_BOT_TOKEN`** 來做 `getMe` 有效性檢查（那是目前唯一的「死 token」自動偵測）。
換成 `*_FILE` 會讓它退回 `warn`，而 recover 的 DoD 是 **48 OK / 0 WARN / 0 FAIL** ⇒ 會直接破壞 DoD ✗。
所以維持 **key 名不變**、只把「值」的來源從 repo 移到安裝期 ✓。
（若哪天 recover 改成支援 `*_FILE`，這個設計可以再簡化；屆時兩邊要同一個 PR 一起改。）

## 安裝 / 更新

```bash
cd <atlas-go>
git pull

# 三選一提供 token（都不會進入版控）
bash scripts/alertmanager-webhook/install-webhook.sh \
    --token-file ~/.config/atlas-alert-webhook/telegram-bot-token      # 建議；chmod 600
# 或
bash scripts/alertmanager-webhook/install-webhook.sh --from-env-file ~/.config/atlas-go/.env
# 或
TELEGRAM_BOT_TOKEN=<token> bash scripts/alertmanager-webhook/install-webhook.sh
```

腳本會：① 渲染 plist（placeholder → token、`/Users/kk` → 你實際的 `$HOME`）
② 備份既有安裝版（`.bak-<ts>`）③ 以 **mode 600** 安裝 plist、複製 relay 到 `~/bin/`
④ `launchctl bootout` + `bootstrap`（失敗退回 `load -w`）⑤ 印出 `state` / `pid`。

- **`--no-load`**：只渲染與寫檔，**不碰 launchd**（沙箱測試用，見 `tests/scripts/test-install-webhook.sh`）。
- **`--verify`**：安裝前打 `getMe` 確認 token 真的能用（避免裝出一支每次都 401 的服務）。
- **`--home <dir>` / `--dest-dir` / `--bin-dir`**：改寫入位置（測試或非標準環境用）。
- 全程**不印 token** ✓（只印 `前4…後4` 與長度）。

## 輪替 token（憑證外洩後或例行）

1. BotFather 重新簽發（或從既有的 `.env` 取新值）。
2. 更新你的 token 檔（`~/.config/atlas-alert-webhook/telegram-bot-token`，`chmod 600`）。
3. `bash scripts/alertmanager-webhook/install-webhook.sh --token-file <path> --verify`。
4. 驗證送達：`tail -5 ~/Library/Logs/atlas-webhook.err.log` 應見 `listening on :9095`，
   下一次告警應見 `OK sent …`；或跑 a2a-dev 的 `macmini-recover`（#50 會同時驗 getMe 與最近一次送達）。
5. ⚠️ **舊 token 一定要撤銷**（BotFather `/revoke`）——它已經在 git 歷史裡 ✗，換掉檔案不等於失效 ✓。

## 驗證與測試

```bash
bash tests/scripts/test-install-webhook.sh     # 14 項：placeholder 規則 / 注入 / mode 600 / 不洩漏 token / 壞輸入必失敗
bash scripts/ci/check_secrets.sh               # 掃 tracked 檔的機密樣式 + 上述測試
```

## 已知耦合 / 注意

- `ProgramArguments` 指向 `~/bin/atlas-alertmanager-webhook-to-telegram.py`（**複製品**）；
  改本目錄的 `.py` 後必須重跑安裝腳本才會生效。
- 安裝版 plist 含 token ⇒ 備份檔 `.bak-*` 也含 token ⇒ 輪替後請清掉舊備份。
- Alertmanager 端的 receiver 指向 `http://host.docker.internal:9095/` 這條路徑的設定在
  `monitoring/alertmanager.yml`（變更後需 `docker restart atlas-alertmanager`）。
