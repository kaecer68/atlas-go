package subscription

import "time"

// Tier represents a subscription level.
type Tier string

const (
	TierFree       Tier = "free"
	TierRegistered Tier = "registered"
	TierPremium    Tier = "premium"
)

// User represents a registered user.
type User struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Tier      Tier      `json:"tier"`
	TrialEnd  time.Time `json:"trial_end"`
	CreatedAt time.Time `json:"created_at"`
}

// ProfileResponse is the JSON payload returned by GET /api/user/profile.
type ProfileResponse struct {
	User          *User     `json:"user"`
	Email         string    `json:"email"`
	Tier          Tier      `json:"tier"`
	EffectiveTier Tier      `json:"effective_tier"`
	TrialEnd      time.Time `json:"trial_end"`
}

// EffectiveTier returns the current tier considering trial status.
func (u *User) EffectiveTier() Tier {
	if u.Tier != TierPremium && time.Now().Before(u.TrialEnd) {
		return TierPremium
	}
	return u.Tier
}

// SubscriptionEvent records tier changes and login events.
type SubscriptionEvent struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Event     string    `json:"event"` // "registered", "login", "upgrade", "downgrade", "trial_start"
	Timestamp time.Time `json:"timestamp"`
}
