package subscription

// #2125：`GetByEmail` 的「查無資料」契約。
//
// 這個契約本身就是本票的修法：舊行為回 (nil, nil)，讓「查無」與「查到」無法區分，
// 呼叫端只要寫 `err == nil` 就會對 nil *User 取用（EffectiveTier() 是指標接收者
// ⇒ 立即 panic，見 internal/recommender 的兩個站點）。

import (
	"errors"
	"testing"
)

func TestGetByEmail_MissingRowReturnsErrNotFound(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	u, err := store.GetByEmail("nobody@example.test")
	if u != nil {
		t.Fatalf("user = %+v, want nil for a missing row", u)
	}
	if err == nil {
		t.Fatal("want an error wrapping ErrNotFound; (nil, nil) is the #2125 anti-pattern")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want errors.Is(err, ErrNotFound)", err)
	}
}

func TestGetByEmail_ExistingRowStillSucceeds(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := store.Register("member@example.test", "hash"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	u, err := store.GetByEmail("member@example.test")
	if err != nil {
		t.Fatalf("GetByEmail(existing): %v", err)
	}
	if u == nil {
		t.Fatal("GetByEmail(existing) returned a nil user with a nil error")
	}
	if u.Email != "member@example.test" {
		t.Errorf("email = %q, want member@example.test", u.Email)
	}
}

// Authenticate 對未知 email 必須回錯誤（而不是讓呼叫端有機會 nil-deref）。
func TestAuthenticate_UnknownEmailReturnsError(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	u, err := store.Authenticate("nobody@example.test", "hash")
	if err == nil {
		t.Fatalf("Authenticate(unknown) = (%+v, nil), want an error", u)
	}
	if u != nil {
		t.Fatalf("Authenticate(unknown) user = %+v, want nil", u)
	}
}
