package marketdata

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
)

// isTAIFEXJSONBody reports whether the (BOM-stripped) response body starts
// with a JSON value. TAIFEX alternates between JSON and CSV for the same
// endpoint, so format detection must be content-based, not header-based.
func isTAIFEXJSONBody(body []byte) bool {
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	return len(trimmed) > 0 && (trimmed[0] == '[' || trimmed[0] == '{')
}

// parseTAIFEXPCRCSV parses the CSV flavor of the PutCallRatio response
// (header row in Chinese, UTF-8 BOM already stripped by readTAIFEXBody).
// Columns are matched by header NAME, not position, so upstream column
// reordering does not silently scramble values.
func parseTAIFEXPCRCSV(body []byte) ([]taifexPCRRaw, error) {
	// Defensive: readTAIFEXBody already strips the UTF-8 BOM, but the parser
	// must survive direct calls with raw upstream bytes (BOM = \xEF\xBB\xBF).
	body = bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))
	reader := csv.NewReader(strings.NewReader(string(body)))
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv read: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("csv: %d rows (need header + data)", len(records))
	}

	header := records[0]
	col := func(name string) int {
		for i, h := range header {
			if strings.TrimSpace(h) == name {
				return i
			}
		}
		return -1
	}
	idx := map[string]int{
		"date":    col("日期"),
		"putVol":  col("賣權成交量"),
		"callVol": col("買權成交量"),
		"volPct":  col("買賣權成交量比率%"),
		"putOI":   col("賣權未平倉量"),
		"callOI":  col("買權未平倉量"),
		"oiPct":   col("買賣權未平倉量比率%"),
	}
	for key, i := range idx {
		if i < 0 {
			return nil, fmt.Errorf("csv: missing column %q", key)
		}
	}

	rows := make([]taifexPCRRaw, 0, len(records)-1)
	for _, rec := range records[1:] {
		if len(rec) < len(header) {
			continue
		}
		rows = append(rows, taifexPCRRaw{
			Date:                  strings.TrimSpace(rec[idx["date"]]),
			PutVolume:             strings.TrimSpace(rec[idx["putVol"]]),
			CallVolume:            strings.TrimSpace(rec[idx["callVol"]]),
			PutCallVolumeRatioPct: strings.TrimSpace(rec[idx["volPct"]]),
			PutOI:                 strings.TrimSpace(rec[idx["putOI"]]),
			CallOI:                strings.TrimSpace(rec[idx["callOI"]]),
			PutCallOIRatioPct:     strings.TrimSpace(rec[idx["oiPct"]]),
		})
	}
	return rows, nil
}
