package marketdata

import (
	"testing"
)

// TestParseTAIFEXPCRCSV_RealUpstreamShape - the 2026-08-31 upstream flip:
// CSV + UTF-8 BOM + Chinese headers (capture of the live response body).
func TestParseTAIFEXPCRCSV_RealUpstreamShape(t *testing.T) {
	// UTF-8 BOM prefix, exactly as TAIFEX serves it.
	body := "\ufeff日期,賣權成交量,買權成交量,買賣權成交量比率%,賣權未平倉量,買權未平倉量,買賣權未平倉量比率%\n" +
		"20260828,308922,306713,100.72,42600,41994,101.44\n" +
		"20260827,15817,15963,101.03,41998,41502,98.82\n"

	rows, err := parseTAIFEXPCRCSV([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].Date != "20260828" {
		t.Errorf("Date = %q, want 20260828", rows[0].Date)
	}
	if rows[0].PutVolume != "308922" || rows[0].CallVolume != "306713" {
		t.Errorf("volumes = %q/%q", rows[0].PutVolume, rows[0].CallVolume)
	}
	if rows[0].PutCallVolumeRatioPct != "100.72" {
		t.Errorf("vol ratio = %q, want 100.72", rows[0].PutCallVolumeRatioPct)
	}
	if rows[0].PutCallOIRatioPct != "101.44" {
		t.Errorf("OI ratio = %q, want 101.44", rows[0].PutCallOIRatioPct)
	}
}

// TestParseTAIFEXPCRCSV_MissingColumn - schema drift must fail loudly, not
// silently produce zero-valued rows (ErrTAIFEXSchema convention).
func TestParseTAIFEXPCRCSV_MissingColumn(t *testing.T) {
	body := "日期,賣權成交量\n20260828,308922\n"
	if _, err := parseTAIFEXPCRCSV([]byte(body)); err == nil {
		t.Fatal("expected error for missing columns")
	}
}

// TestIsTAIFEXJSONBody - content-based format detection.
func TestIsTAIFEXJSONBody(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		{`[{"Date":"20260828"}]`, true},
		{`{"stat":"OK"}`, true},
		{"  \n\t[1,2]", true},
		{"日期,賣權成交量\n20260828,1", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isTAIFEXJSONBody([]byte(tc.body)); got != tc.want {
			t.Errorf("isTAIFEXJSONBody(%q) = %v, want %v", tc.body, got, tc.want)
		}
	}
}
