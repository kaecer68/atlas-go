# Sector Allocation Closure — Rollback Drills

> **SA11.B rollback演练**

## Drill 1: Feature Flag Toggle

1. 模擬 production flag off
2. 確認 legacy reader（BaseWeights）正常回傳
3. 確認 handler 503 → 200

## Drill 2: Snapshot Data Corruption

1. 模擬 snapshot 檔案損毀
2. 確認 `snapshot_unavailable` 503 回傳
3. 確認 legacy reader 正常

## Drill 3: TAIEX Data Missing

1. 移除 benchmark data
2. 確認 warming_up status
3. 確認 ranking 返回空 ranked list

## 通過條件

- 每次 drill 實測通過
- 結果記錄在 observation log
- 無 side effect 殘留
