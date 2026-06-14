package monitoring

import (
	"testing"
)

func TestSanitizeMetricName_Basic(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"cpu_usage", "cpu_usage"},
		{"CPU-Usage", "CPU_Usage"},
		{"disk i/o", "disk_i_o"},
		{"123test", "123test"},
		{"test.123", "test_123"},
		{"", ""},
		{"a:b:c", "a:b:c"},
		{"hello world!", "hello_world_"},
	}

	for _, tc := range tests {
		got := sanitizeMetricName(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeMetricName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSanitizeLabelName_Basic(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"host_name", "host_name"},
		{"Host-Name", "Host_Name"},
		{"123label", "123label"},
		{"label.123", "label_123"},
		{"", ""},
		{"foo bar", "foo_bar"},
	}

	for _, tc := range tests {
		got := sanitizeLabelName(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeLabelName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestEscapeLabelValue_Basic(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"normal", "normal"},
		{`has\backslash`, `has\\backslash`},
		{`has"quote`, `has\"quote`},
		{"has\nnewline", "has\\nnewline"},
		{`mixed\and"quote`, `mixed\\and\"quote`},
	}

	for _, tc := range tests {
		got := escapeLabelValue(tc.input)
		if got != tc.want {
			t.Errorf("escapeLabelValue(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFormatMetricLine_NoLabels(t *testing.T) {
	m := Metric{Value: 42.5}
	got := formatMetricLine("cpu_usage", m)
	want := "cpu_usage 42.500000\n"
	if got != want {
		t.Errorf("formatMetricLine = %q, want %q", got, want)
	}
}

func TestFormatMetricLine_WithLabels(t *testing.T) {
	m := Metric{
		Value:  99.9,
		Labels: map[string]string{"host": "server1", "zone": "us-east"},
	}
	got := formatMetricLine("http_requests", m)
	if len(got) == 0 {
		t.Fatal("formatMetricLine returned empty string")
	}
	// Should contain the metric name, value, and label keys.
	if got[:13] != "http_requests" {
		t.Errorf("line doesn't start with metric name: %q", got)
	}
}

func TestFormatMetricLine_SingleLabel(t *testing.T) {
	m := Metric{
		Value:  0.5,
		Labels: map[string]string{"method": "GET"},
	}
	got := formatMetricLine("latency", m)
	if len(got) == 0 {
		t.Fatal("formatMetricLine returned empty string")
	}
}

func TestFormatMetricLine_EmptyLabels(t *testing.T) {
	m := Metric{Value: 0.0, Labels: map[string]string{}}
	got := formatMetricLine("zero_metric", m)
	want := "zero_metric 0.000000\n"
	if got != want {
		t.Errorf("formatMetricLine = %q, want %q", got, want)
	}
}

func TestSanitizeMetricName_AllSpecialChars(t *testing.T) {
	got := sanitizeMetricName("!@#$%^&*()")
	if got != "__________" {
		t.Errorf("sanitizeMetricName = %q, want __________", got)
	}
}

func TestSanitizeLabelName_AllSpecialChars(t *testing.T) {
	got := sanitizeLabelName("!@#$%^&*()")
	if got != "__________" {
		t.Errorf("sanitizeLabelName = %q, want __________", got)
	}
}
