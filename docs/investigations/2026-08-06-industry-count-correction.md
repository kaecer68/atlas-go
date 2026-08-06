# 2026-08-06: Industry Count 歷史污染修正 — 14 vs 11 vs 16

> **目的**: 記錄「14 industries」這個數字在跨 doc / commit message 的歷史污染,並給出真實數字與驗證方法。issue #1465 完成定義第 3 項。
> **結論先行**: `auto_cycle_update` 的 fail industry 數是 **11**,不是 14;L1 總數是 **16** (11 有 stocks + 5 無),另有 13 個 L2。14 是 PR-#1461 dispatch 階段的**未驗證估算**。

---

## 1. 三個數字的來源

| 數字 | 來源 | 正確性 |
|------|------|--------|
| **14** | PR-#1461 dispatch 階段的用量估算: 「14 industries × 平均 2.5 stocks × 12 calls = 420 calls per run」 | ❌ **從未驗證過** (8/5 doc line 64) |
| **11** | production 14:16:25 UTC 實際 `industry_aggregate_failed` 的 unique industry 數 | ✅ 真實 fail 數 |
| **16** | `configs/parameters.json` `industry.classification_tree.value.segments` 的 L1 總數 | ✅ 真實 L1 數 |

## 2. 真實結構 (2026-08-06 以 parameters.json 驗證)

`industry.classification_tree.value.segments` = **29 segments = 16 L1 + 13 L2**:

- **16 L1**,其中 11 個有 `representative_stocks`: ai_supply_chain / consumer / electronics / energy / financials / industrial / leo_satellite / mining / robotics / semiconductor / shipping
- **5 個 L1 無 stocks (不參與 auto_cycle_update)**: etf_rotation / defensive / high_dividend / small_cap / tech
- **13 L2 (全無 stocks)**: pcb / thermal (→electronics), foundry / server_assembly / cooling (→semiconductor), precious_metals_recycling / copper_industry / rare_earth_specialty / metal_processing (→mining), satellite_rf_components / satellite_pcb / ground_equipment / laser_communication (→leo_satellite)

`auto_cycle_update` (data_aggregator.go `AggregateIndustry`) 只跑有 stocks 的 L1 → **11 個 fail 是正確且一致的** (5 個無 stocks L1 + 13 個 L2 不參與)。

## 3. 歷史污染傳播路徑

「14」這個數字從 PR-#1461 開始散播,未經 production log 驗證:

| 位置 | 內容 |
|------|------|
| `docs/investigations/2026-08-05-auto-cycle-update-quota-misconception.md` line 64 | 「14 industries × 平均 2.5 stocks … = 420 calls per run」(估算) |
| 同 doc line 112 | 「對 14 industries 的全部 representative stocks 跑 TaiwanStockInfo」 |
| PR-#1462 commit message | 引用「14」 |
| PR-#1463 commit message | 引用「14 個 industry」 |

**根因**: 從 dispatch 階段的估算數字直接複製,沒有回去從 production log 數真實 unique industry count。

## 4. 修正後的驗證方法

**禁止** 僅憑 PR-#1461 / #1462 / #1463 commit message 內的「14」推導 fail industry 數。

正確驗證:
```bash
# 真實 fail 數 (應得 11 個 unique industry)
docker logs atlas-go --since 1h | grep "industry_aggregate_failed" \
  | awk -F'industry=' '{print $2}' | awk '{print $1}' | sort -u

# 真實 L1 結構 (16 L1 / 13 L2)
python3 -c "
import json
data = json.load(open('configs/parameters.json'))
segs = data['industry']['classification_tree']['value']['segments']
print('total:', len(segs), 'L1:', sum(1 for s in segs if s.get('level')==1), 'L2:', sum(1 for s in segs if s.get('level')==2))
"
```

## 5. 關聯文件

- 主根因盤查: `docs/investigations/2026-08-06-finmind-quota-collision.md` (已 amended, §2.2 更新為 16 L1 + 13 L2)
- 前一份盤查: `docs/investigations/2026-08-05-auto-cycle-update-quota-misconception.md` (line 64/112 保留歷史,未改 commit)
- Issue: #1465

## 6. 寫作時間

2026-08-06 (reviewer finding 3 + 8 驗證後補記)
