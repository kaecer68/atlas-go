package server

import (
	"context"
	"testing"
)

func TestHandleGetRecommendations_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"tier":"free","market":{"regime":"NEUTRAL"}}`)
	_, out, err := s.handleGetRecommendations(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/recommendations" {
		t.Fatalf("path=%s", rec.path)
	}
	if out == nil {
		t.Fatal("expected out non-nil")
	}
}
