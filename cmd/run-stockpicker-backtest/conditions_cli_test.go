package main

// conditions_cli_test.go — PR 2a CLI tests: -list-conditions output and
// -conditions flag filtering.

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// TestCLI_ListConditions: -list-conditions prints the available conditions
// (both demo conditions plus the fundamentals live_observe_only placeholder)
// and exits without creating the outcome DB or running a backtest.
func TestCLI_ListConditions(t *testing.T) {
	dir := writeTestWorkdir(t)
	if err := runWithPanel([]string{"-workdir", dir, "-list-conditions"}, synthPanel(t)); err != nil {
		t.Fatalf("-list-conditions: %v", err)
	}
	// No backtest side effects: the outcome DB must not exist yet.
	if _, err := os.Stat(filepath.Join(dir, "data", "state", "atlas.db")); !os.IsNotExist(err) {
		t.Fatalf("-list-conditions must not create the outcome DB (stat err=%v)", err)
	}

	// The rendered text lists both demo conditions + the placeholder.
	params, err := config.LoadParametersConfig(filepath.Join(dir, "configs", "parameters.json"))
	if err != nil {
		t.Fatalf("load parameters: %v", err)
	}
	text := listConditionsText(stockpicker.NewDefaultConditionRegistry(&params.Stockpicker.Conditions))
	for _, want := range []string{
		"foreign-3d-net-buy",
		"momentum-20d-positive",
		"fundamental-value",
		"live_observe_only",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("list output missing %q:\n%s", want, text)
		}
	}
}

// TestCLI_ConditionsFlag: -conditions foreign-3d-net-buy restricts the run
// to that single condition (no momentum outcomes).
func TestCLI_ConditionsFlag(t *testing.T) {
	wd, err := runCLI(t, synthPanel(t), "-conditions", "foreign-3d-net-buy")
	if err != nil {
		t.Fatalf("run with -conditions: %v", err)
	}
	db := openCLITestDB(t, wd)
	sources, err := distinctSources(t, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0] != "stockpicker-foreign-3d-net-buy" {
		t.Fatalf("sources = %v, want only [stockpicker-foreign-3d-net-buy]", sources)
	}
}

// TestCLI_ConditionsFlag_Unknown rejects unknown condition IDs.
func TestCLI_ConditionsFlag_Unknown(t *testing.T) {
	wd := writeTestWorkdir(t)
	err := runWithPanel([]string{"-workdir", wd, "-conditions", "bogus-condition"}, synthPanel(t))
	if err == nil {
		t.Fatal("unknown condition ID must error")
	}
	if !strings.Contains(err.Error(), "unknown condition") {
		t.Fatalf("error = %v, want unknown-condition mention", err)
	}
}

// TestSelectConditions_Defaults verifies an empty -conditions list resolves
// to the full default registry set, and whitespace is tolerated.
func TestSelectConditions_Defaults(t *testing.T) {
	params, err := config.LoadParametersConfig(filepath.Join("..", "..", "configs", "parameters.json"))
	if err != nil {
		t.Fatalf("load parameters: %v", err)
	}
	reg := stockpicker.NewDefaultConditionRegistry(&params.Stockpicker.Conditions)

	conds, err := selectConditions("", reg)
	if err != nil {
		t.Fatalf("selectConditions empty: %v", err)
	}
	if len(conds) != len(reg.All()) {
		t.Fatalf("empty flag resolved to %d conditions, want %d", len(conds), len(reg.All()))
	}
	for i, c := range conds {
		if c.ID != reg.All()[i].ID {
			t.Errorf("condition order mismatch at %d: %q vs %q", i, c.ID, reg.All()[i].ID)
		}
	}

	one, err := selectConditions(" momentum-20d-positive ", reg)
	if err != nil {
		t.Fatalf("selectConditions one: %v", err)
	}
	if len(one) != 1 || one[0].ID != "momentum-20d-positive" {
		t.Fatalf("selectConditions one = %+v, want [momentum-20d-positive]", one)
	}
}

// distinctSources returns the distinct source column of stock_signal_outcomes.
func distinctSources(t *testing.T, db *sql.DB) ([]string, error) {
	t.Helper()
	rows, err := db.Query(`SELECT DISTINCT source FROM stock_signal_outcomes`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
