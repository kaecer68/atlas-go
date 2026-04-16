package db

import (
	"context"
	"testing"
)

func TestInitRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	_, err := Init(context.Background(), "", "")
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is empty")
	}
}
