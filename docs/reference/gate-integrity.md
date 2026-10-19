# 守門完整性（Gate Integrity）

> 本 repo 反覆出現「**守門說 X，但 X 不成立**」⇒ 任何「守門說通過／違規」的結論，都要先問三題（§三題）。
> 四型的共同點：紅燈或綠燈都**不是關於程式碼的陳述**，而是關於**守門自己的量測方法**。
> 本文件只收**有實據**的實例（附 `file:line` 或可重跑指令）；沒有證據的推論標「未證」或省略。

## 四型（2026-10-19 一天內各出現一次）

### 1. 量錯對象 —— 指標永遠紅／永遠誤報

- **實例**：`CalibrationArtifactStale` 量 SSOT `configs/parameters.json` 的 `updated_at`，
  但生產根本不寫它（容器改寫 bind mount 上的 overlay）⇒ age ≈ 83 天 = image 建置日 ⇒ **永久誤報**
  （`internal/monitoring/calibration_effective.go:20`；`cmd/atlas/calibration_freshness_metrics_task.go:25`
  原檢查「量的是**錯的標的**」）。
- **修法**：改量**實際產物**（overlay，`artifact="parameters_overlay"`，同檔 `:34`）；
  量不到就**刪告警**——`artifact="parameters"` 語意保留但**不再有告警規則**（`:30`），
  「不新鮮」也不再每 5 分鐘 WARN（`:32`，那正是永久誤報的另一半）。
- **狀態**：已修。★ `docs/operations/FOLLOWUPS.md:6248` 同日**撤回**了「它仍為永久誤報」的登記
  ——修完還照抄舊結論，就是本型再犯一次。

### 2. 清單過期 —— 合法行為被判違規（假紅）

- **實例 A（本 repo，已修）**：`check_db_credential_interpolation.sh` 原本只掃 repo 根
  `docker-compose.yml` ⇒ 同一條部署路徑的**第二份** compose 靜默漏掃；且 DSN userinfo 用 naive
  `index(seg, ":")` 切分 ⇒ `${DB_USER:?…}:${DB_PASSWORD:?…}` 被切壞 ⇒ **合規寫法被判違規**
  （`docs/operations/FOLLOWUPS.md:63-64`、`:80-82`）。
  修法＝**glob 自動納入**（`scripts/ci/check_db_credential_interpolation.sh:52-55`）
  ＋只認 `}` 外的冒號（同檔 `:112-113`）。
- **實例 B（非本 repo）**：`experiments/003` 手寫 fallback allowlist 放了**已退役**的 `deepseek-pro`、
  漏 `deepseek-flash` ⇒ 該檔在 **a2a-dev**，已轉交（`docs/operations/FOLLOWUPS.md:6260`）；
  修法＝改讀**運行時真相**（LiteLLM config 的 `group → deployment`，同檔 `:6266`）。
- **判準**：**手寫清單必然過期** ⇒ 能自動納入就 glob、能讀真相就讀真相。

### 3. 方向寫反 —— 本地綠、CI 紅燈

- **實例**：`tests/scripts/test-check-case-conflicts.sh` 第 ⑤ 步（突變自證）原借用
  `negproof_expect 1`，但該 helper 的語意是「命令**必須被擋**」：
  rc == 期望值才印 ✅，否則發 `::error::` 並 `return 1`（`scripts/ci/negative-proof-lib.sh:81-86`）。
  第 ⑤ 步要證的恰好相反（拿掉 `exit 1` ⇒ 應 **rc=0**）⇒ 每次都對**預期結果**發 `::error::`，
  shell 仍印 ✅ 並 exit 0 —— 一條自我否定、什麼都沒斷言的自證。歷史原文留在同檔 `:184-191`。
- **修法**：不要借用方向相反的 helper，直接檢查 expected rc（`tests/scripts/test-check-case-conflicts.sh:192-195`）。
- **狀態**：已修（#2296）。★ 附帶事實：該步的 exit code 一直是 **0**，所以它**不會**讓 push 失敗；
  「曾擋下所有 PR 推送」在此**不成立**（未證 ✗）。它紅的是 GitHub 註解（annotation），不是結論。

### 4. 判決取決於 clone 大小／歷史

- **機制**：`ci-quick` 對清單內每一支檢查都用**固定** `timeout 10`（`Makefile:613`）⇒
  判決取決於「這台機器／這個 clone 當下多慢」，不是程式碼。
- **實測（2026-10-19，同一台機、同一個乾淨 worktree `.worktrees/gateint`，各跑兩次）**：

  | 檢查 | 首跑 | 二跑 | 差異來源 |
  |---|---|---|---|
  | `check_channel_consistency.sh`（`go run`，`:8`） | **19.10s** | **0.96s** | Go build cache 冷／熱 |
  | `check_constitution.sh`（`grep -rn … .`，`:149`） | **2.56s** | **1.60s** | 走訪樹大小 |

  長壽 clone 的 `data/` = **864M／8088 檔**，乾淨 worktree = **196K／19 檔**
  （`du -sh data; find data -type f | wc -l`）⇒ 同一份樹、不同 clone，判決可以相反。
- **修法**：預算改「**預熱後才計時**」；`go run` 類檢查必須先 warm-up 一輪再開始計時，
  否則 10s 在冷 cache 的乾淨 worktree 也必然誤殺（上表首跑 19.10s 已證）。

## 三題（動手前先問）

1. 它量的是**對象**嗎？——是它宣稱的那個產物，還是剛好有 `updated_at` 的另一個？（§1）
2. 它的**清單／門檻今天還成立**嗎？——**動手前先複驗** ✓；本 repo 同日已有三次「候選缺陷其實早已處理」
   的紀錄（`docs/operations/FOLLOWUPS.md:6257`）。（§2）
3. 它的**判決方向**與**執行環境**有關嗎？——方向看 helper 語意（§3），時間看 clone 大小與 cache（§4）。

## 延伸：儀器／答題者的「身分」

凡產出經代理層（如 LiteLLM）⇒ **不得由內容推斷是誰回答的** ✗。
可用探針＝讀回應標頭 **`x-litellm-model-name`**：送 group `deepseek` 曾回 200，標頭卻是
`openai/MiniMax-M3`（**外部觀測**；本 repo 未實作此探針——除本行記載外 `grep -rIl 'x-litellm-model-name'`
為 0 命中；本機複驗時 `kimac` 無 DNS、`100.68.42.72:4000` 逾時 ⇒ **此刻未能複驗**）。
原則與四型相同：**驗證儀器實際量到的是什麼**，再引用它給的結論。

## 驗證方式：負向對照 ＋ 正向對照

- **負向對照**：修前假綠的**原文** 與 修後紅燈的**原文** 必須**不同** ✗（逐字貼輸出，不接受描述）。
- **正向對照**：合規輸入仍綠 ⇒ 證明紅燈不是「任何變動都紅」。
- **突變打在行為行並指名**：突變要改**執行判定的那一行**，並寫出 `file:line`
  （例：`scripts/ci/negative-proof-lib.sh:81` 的 `[ "${rc}" -eq "${expected}" ]`）。
  錨點找不到 ⇒ 這條自證是空的，必須 fail loud（範例：`tests/scripts/test-check-case-conflicts.sh:181`）。

## 相關文件

- [traps.md](traps.md) — 跨模組陷阱；本文件是**守門自身**的陷阱
- [iteration-gate.md](iteration-gate.md) — 自我檢查閘門規範
- `docs/operations/FOLLOWUPS.md` — 每則實例的登記、轉交與撤回紀錄
