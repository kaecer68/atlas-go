package server

import (
	"context"
	"testing"
)

func TestHandleDataGetChannels_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleDataGetChannels(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/data-channels" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleDataGetChannelDetail_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleDataGetChannelDetail(context.Background(), nil, channelNameInput{ChannelName: "fugle"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/data-channels/fugle" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleDataGetQuality_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleDataGetQuality(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/data-quality" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleDataGetFieldContract_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleDataGetFieldContract(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/field-contract" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleUniverseGetSessions_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleUniverseGetSessions(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/sessions" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleUniverseGetUniverseOverlap_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleUniverseGetUniverseOverlap(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/universe-overlap" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}
