package recommender

import (
	"strconv"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/subscription"
)

// 本檔是 #2128 的 tier 來源與語彙收斂。背景（C-02 atlas-jwt-trust）：
//
//   - 存取層級的權威是**已驗證的 JWT claims**（go-member 的 tier claim 由
//     JWTManager 的 verifyRS256 映射成 access tier：registered→basic、
//     premium→pro、其他→free）。本地 users 列在 C-02 之後**不再是**會員來源。
//   - 兩代語彙並存：access tier `free`/`basic`/`pro` 與 legacy 自簽 HS256 的
//     `free`/`registered`/`premium`。同一個存取等級必須對應同一份內容。
//   - `internal/strategy_ranker` 的 `RankedReport.Tier` **不是**這個軸：它是
//     「報告內容深度」標籤（依排名 assign），不代表呼叫者身份。

// accessClass 把兩代 tier 語彙收斂成同一個「存取等級」整數，供比較與稽核使用。
//
//	free                         → 0
//	basic / registered           → 1（註冊會員）
//	pro   / premium              → 2（付費會員）
//	未知值                        → 0（最低；未知不得因此取得更多內容）
//
// 未知值刻意落在 0：與 switch tier 的 default 分支（不給 strategies）一致。
func accessClass(t subscription.Tier) int {
	switch t {
	case subscription.TierBasic, subscription.TierRegistered:
		return 1
	case subscription.TierPro, subscription.TierPremium:
		return 2
	default:
		return 0
	}
}

// tierDrift 回報「claims 的存取等級」與「本地列的存取等級」是否不同，
// 並回傳可讀的說明供 log 使用（#2128：本地列只做稽核，判定一律以 claims 為準）。
func tierDrift(claimsTier, localTier subscription.Tier) (bool, string) {
	cc, lc := accessClass(claimsTier), accessClass(localTier)
	if cc == lc {
		return false, ""
	}
	return true, "claims_class=" + strconv.Itoa(cc) + " local_class=" + strconv.Itoa(lc)
}

// auditLocalTierDrift 記錄「本地 users 列與 claims 不一致」的稽核警告。
//
// 只做觀測：**不影響**上方已由 claims 決定的 tier（#2128）。沒有本地列是
// go-member 會員的常態，不算異常 ⇒ 直接返回、不記警告。
func (h *Handler) auditLocalTierDrift(email string, claimsTier subscription.Tier) {
	if email == "" {
		return
	}
	user, err := h.subStore.GetByEmail(email)
	if err != nil || user == nil {
		return
	}
	if drifted, detail := tierDrift(claimsTier, user.EffectiveTier()); drifted {
		logging.Warn("recommender", "tier_claim_local_drift",
			logging.FStr("email", email),
			logging.FStr("claims_tier", string(claimsTier)),
			logging.FStr("local_tier", string(user.EffectiveTier())),
			logging.FStr("detail", detail))
	}
}
