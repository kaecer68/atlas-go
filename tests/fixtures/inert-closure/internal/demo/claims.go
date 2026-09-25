package demo

// Receipt.Applied 宣稱「已生效」，是 claim set 的目標欄位。
type Receipt struct {
	Applied bool   `json:"applied"`
	Reason  string `json:"reason"`
}

// BuildEvidenceDerived 由消費證據推導 Applied（**正確寫法**：不硬寫 true）→ 不應被標記。
func BuildEvidenceDerived(consumed int) Receipt {
	return Receipt{Applied: consumed > 0, Reason: "derived from consumption"}
}

// BuildClaimedWithoutEvidence 是 fixture 的故意案例：Applied 由字面值硬寫（由 baseline 覆蓋）。
func BuildClaimedWithoutEvidence() Receipt {
	return Receipt{Applied: true, Reason: "claimed"}
}

// Describe 是 consumer：真的讀 Receipt.Applied（避免 claim-not-consumed 誤報）。
func Describe(r Receipt) string {
	if r.Applied {
		return "applied"
	}
	return r.Reason
}
