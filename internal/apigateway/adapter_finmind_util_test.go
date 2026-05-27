package apigateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestYesterday(t *testing.T) {
	result := yesterday()
	if !strings.Contains(result, "-") {
		t.Errorf("yesterday() = %q, expected YYYY-MM-DD format", result)
	}
	// Verify the returned date is before today
	today := time.Now().Format("2006-01-02")
	if result >= today {
		t.Errorf("yesterday() = %q, expected date before today (%s)", result, today)
	}
}

func TestSaveSnapshot(t *testing.T) {
	// saveSnapshot uses "data/state/<channelID>/latest.json" relative path
	// We need to ensure the test runs from a clean state
	channelID := "test_channel_test"
	defer func() {
		_ = os.RemoveAll("data")
	}()

	saveSnapshot(channelID, []byte(`{"test": true}`))

	expectedPath := filepath.Join("data", "state", channelID, "latest.json")
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("saveSnapshot failed to write file: %v", err)
	}
	if string(data) != `{"test": true}` {
		t.Errorf("saveSnapshot wrote %q, want {\"test\": true}", string(data))
	}
}
