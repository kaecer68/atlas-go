package sectorallocation

import "github.com/kaecer68/atlas-go/internal/industry"

// isL1KeyForFilter 把 string key 透過 industry.SectorIDFromString 解析並驗證 L1。
// SA-INV-02：非 L1 key（含 cash/defensive/_cash_reserve/industrial 等）不得進入 L1 final target。
func isL1KeyForFilter(key string) bool {
	id, ok := industry.SectorIDFromString(key)
	if !ok {
		return false
	}
	return industry.IsL1(id)
}
