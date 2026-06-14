package shared

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFlexTime_UnmarshalJSON_Null(t *testing.T) {
	var ft FlexTime
	if err := ft.UnmarshalJSON([]byte("null")); err != nil {
		t.Fatalf("UnmarshalJSON(null) error = %v", err)
	}
	if !ft.IsZero() {
		t.Error("FlexTime after null unmarshal should be zero")
	}
}

func TestFlexTime_UnmarshalJSON_EmptyString(t *testing.T) {
	var ft FlexTime
	if err := ft.UnmarshalJSON([]byte(`""`)); err != nil {
		t.Fatalf("UnmarshalJSON(empty string) error = %v", err)
	}
	if !ft.IsZero() {
		t.Error("FlexTime after empty string unmarshal should be zero")
	}
}

func TestFlexTime_UnmarshalJSON_RFC3339(t *testing.T) {
	var ft FlexTime
	input := `"2026-06-14T10:30:00Z"`
	if err := ft.UnmarshalJSON([]byte(input)); err != nil {
		t.Fatalf("UnmarshalJSON(RFC3339) error = %v", err)
	}
	want := time.Date(2026, 6, 14, 10, 30, 0, 0, time.UTC)
	if !ft.Equal(want) {
		t.Errorf("FlexTime = %v, want %v", ft.Time, want)
	}
}

func TestFlexTime_UnmarshalJSON_DateOnly(t *testing.T) {
	var ft FlexTime
	input := `"2026-06-14"`
	if err := ft.UnmarshalJSON([]byte(input)); err != nil {
		t.Fatalf("UnmarshalJSON(date only) error = %v", err)
	}
	want := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	if !ft.Equal(want) {
		t.Errorf("FlexTime = %v, want %v", ft.Time, want)
	}
}

func TestFlexTime_UnmarshalJSON_Invalid(t *testing.T) {
	var ft FlexTime
	input := `"not-a-date"`
	if err := ft.UnmarshalJSON([]byte(input)); err == nil {
		t.Error("UnmarshalJSON(invalid) should return error")
	}
}

func TestFlexTime_MarshalJSON_Zero(t *testing.T) {
	ft := FlexTime{}
	data, err := ft.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON(zero) error = %v", err)
	}
	if string(data) != "null" {
		t.Errorf("MarshalJSON(zero) = %s, want null", data)
	}
}

func TestFlexTime_MarshalJSON_NonZero(t *testing.T) {
	ft := FlexTime{Time: time.Date(2026, 6, 14, 10, 30, 0, 0, time.UTC)}
	data, err := ft.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON(non-zero) error = %v", err)
	}
	var parsed time.Time
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("remarshal error = %v", err)
	}
	if !parsed.Equal(ft.Time) {
		t.Errorf("MarshalJSON roundtrip = %v, want %v", parsed, ft.Time)
	}
}
