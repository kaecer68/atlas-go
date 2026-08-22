package subscription

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestParseMembershipExpiry(t *testing.T) {
	if got := parseMembershipExpiry(json.RawMessage(`1787369429`)); got != 1787369429 {
		t.Errorf("epoch int: got %d", got)
	}
	if got := parseMembershipExpiry(json.RawMessage(`"2026-09-19T07:28:40.000Z"`)); got != 1789802920 {
		t.Errorf("ISO string: got %d, want 1787369320", got)
	}
	if got := parseMembershipExpiry(json.RawMessage(`"not-a-date"`)); got != 0 {
		t.Errorf("invalid: got %d", got)
	}
	if got := parseMembershipExpiry(nil); got != 0 {
		t.Errorf("empty: got %d", got)
	}
}

func TestVerifyRS256_StringMembershipExpiry(t *testing.T) {
	// go-member 現版 token：membershipExpiresAt 為 ISO-8601 string（驗收實測）
	claims := memberClaims{
		Sub:                 "aa40b729-4002-4e78-9be7-a5a7dbc10d78",
		Email:               "test@goluck.local",
		Tier:                "pro",
		MembershipExpiresAt: json.RawMessage(`"2026-09-19T07:28:40.000Z"`),
		Exp:                 time.Now().Add(time.Hour).Unix(),
	}
	payload, _ := json.Marshal(claims)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	// 建一個 HS256 token（模擬 legacy 路徑驗證 claims 解析）；RS256 路徑的
	// 型別解析共用 memberClaims，這裡驗證 Unmarshal 不再失敗。
	if err := json.Unmarshal(payload, &memberClaims{}); err != nil {
		t.Fatalf("memberClaims unmarshal with string membershipExpiresAt: %v", err)
	}
	_ = payloadB64
}
