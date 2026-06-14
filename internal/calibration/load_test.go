package calibration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeJSONL(t *testing.T, path string, lines []map[string]float64) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create jsonl: %v", err)
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	for _, line := range lines {
		if err := enc.Encode(line); err != nil {
			t.Fatalf("encode line: %v", err)
		}
	}
}

func TestLoadReturnsFromJSONL(t *testing.T) {
	tmp := t.TempDir()

	t.Run("happy path", func(t *testing.T) {
		path := filepath.Join(tmp, "returns.jsonl")
		writeJSONL(t, path, []map[string]float64{
			{"return": 0.01},
			{"return": -0.02},
			{"return": 0.005},
		})

		got, err := LoadReturnsFromJSONL(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []float64{0.01, -0.02, 0.005}
		if len(got) != len(want) {
			t.Fatalf("got %d returns, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("got[%d] = %v, want %v", i, got[i], want[i])
			}
		}
	})

	t.Run("empty file", func(t *testing.T) {
		path := filepath.Join(tmp, "empty.jsonl")
		writeJSONL(t, path, nil)

		got, err := LoadReturnsFromJSONL(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %d returns, want 0", len(got))
		}
	})

	t.Run("skips invalid lines", func(t *testing.T) {
		path := filepath.Join(tmp, "mixed.jsonl")
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		_, _ = f.WriteString("{\"return\":0.1}\n")
		_, _ = f.WriteString("not json\n")
		_, _ = f.WriteString("{\"return\":0.2}\n")
		_, _ = f.WriteString("\n")
		_ = f.Close()

		got, err := LoadReturnsFromJSONL(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []float64{0.1, 0.2}
		if len(got) != len(want) {
			t.Fatalf("got %d returns, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("got[%d] = %v, want %v", i, got[i], want[i])
			}
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := LoadReturnsFromJSONL(filepath.Join(tmp, "missing.jsonl"))
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("skips non-finite values", func(t *testing.T) {
		path := filepath.Join(tmp, "nonfinite.jsonl")
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		_, _ = f.WriteString("{\"return\":0.1}\n")
		_, _ = f.WriteString("{\"return\":\"NaN\"}\n")
		_, _ = f.WriteString("{\"return\":\"+Inf\"}\n")
		_, _ = f.WriteString("{\"return\":0.2}\n")
		_ = f.Close()

		got, err := LoadReturnsFromJSONL(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []float64{0.1, 0.2}
		if len(got) != len(want) {
			t.Fatalf("got %d returns, want %d", len(got), len(want))
		}
	})
}

func TestLoadReturnsFromCSV(t *testing.T) {
	tmp := t.TempDir()

	writeCSV := func(t *testing.T, path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write csv: %v", err)
		}
	}

	t.Run("happy path", func(t *testing.T) {
		path := filepath.Join(tmp, "prices.csv")
		writeCSV(t, path, `Date,Code,Name,TradeVolume,Open,High,Low,Close
2024-01-01,2330,TSMC,1000,500,510,495,500
2024-01-02,2330,TSMC,1100,505,515,500,510
2024-01-03,2330,TSMC,1200,510,520,505,520
`)

		returns, count, err := LoadReturnsFromCSV(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 3 {
			t.Errorf("count = %d, want 3", count)
		}
		if len(returns) != 2 {
			t.Fatalf("got %d returns, want 2", len(returns))
		}
		want0 := (510.0 - 500.0) / 500.0
		want1 := (520.0 - 510.0) / 510.0
		if returns[0] != want0 {
			t.Errorf("return[0] = %v, want %v", returns[0], want0)
		}
		if returns[1] != want1 {
			t.Errorf("return[1] = %v, want %v", returns[1], want1)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, _, err := LoadReturnsFromCSV(filepath.Join(tmp, "missing.csv"))
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("missing columns", func(t *testing.T) {
		path := filepath.Join(tmp, "bad.csv")
		writeCSV(t, path, "Date,Code\n2024-01-01,2330\n")

		_, _, err := LoadReturnsFromCSV(path)
		if err == nil {
			t.Fatal("expected error for missing columns")
		}
	})

	t.Run("no valid bars", func(t *testing.T) {
		path := filepath.Join(tmp, "empty.csv")
		writeCSV(t, path, `Date,Code,Name,TradeVolume,Open,High,Low,Close
`)

		_, _, err := LoadReturnsFromCSV(path)
		if err == nil {
			t.Fatal("expected error for no valid bars")
		}
	})
}

func TestLoadReturns(t *testing.T) {
	tmp := t.TempDir()

	t.Run("jsonl with enough data", func(t *testing.T) {
		path := filepath.Join(tmp, "enough.jsonl")
		lines := make([]map[string]float64, 35)
		for i := range lines {
			lines[i] = map[string]float64{"return": 0.01 * float64(i)}
		}
		writeJSONL(t, path, lines)

		returns, n, err := LoadReturns(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 35 {
			t.Errorf("n = %d, want 35", n)
		}
		if len(returns) != 35 {
			t.Fatalf("got %d returns, want 35", len(returns))
		}
	})

	t.Run("insufficient data", func(t *testing.T) {
		path := filepath.Join(tmp, "few.jsonl")
		lines := make([]map[string]float64, 10)
		for i := range lines {
			lines[i] = map[string]float64{"return": 0.01}
		}
		writeJSONL(t, path, lines)

		_, n, err := LoadReturns(path)
		if err == nil {
			t.Fatal("expected error for insufficient data")
		}
		if n != 10 {
			t.Errorf("n = %d, want 10", n)
		}
	})

	t.Run("jsonl empty falls back to csv and fails", func(t *testing.T) {
		jsonlPath := filepath.Join(tmp, "data.jsonl")
		writeJSONL(t, jsonlPath, nil)

		_, _, err := LoadReturns(jsonlPath)
		if err == nil {
			t.Fatal("expected error when falling back from JSONL to CSV on same path")
		}
	})
}
