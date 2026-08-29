package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestCheckIntegrity_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")

	cfg := DefaultParametersConfig()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write valid params.json: %v", err)
	}

	errs := CheckParamsIntegrity(path)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for valid file, got %d: %v", len(errs), errs)
	}
}

func TestCheckIntegrity_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")

	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatalf("write invalid params.json: %v", err)
	}

	errs := CheckParamsIntegrity(path)
	if len(errs) == 0 {
		t.Fatal("expected errors for invalid JSON, got none")
	}
}

func TestCheckIntegrity_MissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")

	if err := os.WriteFile(path, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatalf("write incomplete params.json: %v", err)
	}

	errs := CheckParamsIntegrity(path)
	if len(errs) == 0 {
		t.Fatal("expected errors for missing required fields, got none")
	}
}

func TestCheckIntegrity_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")

	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("write empty params.json: %v", err)
	}

	errs := CheckParamsIntegrity(path)
	if len(errs) == 0 {
		t.Fatal("expected errors for empty file, got none")
	}
}

func TestRequiredParamsKeysMatchStructFields(t *testing.T) {
	cfg := DefaultParametersConfig()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("unmarshal default config JSON: %v", err)
	}

	actualKeys := make(map[string]struct{}, len(obj))
	for k := range obj {
		actualKeys[k] = struct{}{}
	}

	omitemptyFields := make(map[string]struct{})
	alwaysSerializedFields := make([]string, 0)
	rt := reflect.TypeFor[ParametersConfig]()
	for field := range rt.Fields() {
		tag := field.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}

		parts := strings.Split(tag, ",")
		name := strings.TrimSpace(parts[0])
		if name == "" {
			name = field.Name
		}

		isOmitempty := false
		for _, p := range parts[1:] {
			if strings.TrimSpace(p) == "omitempty" {
				isOmitempty = true
				break
			}
		}
		if isOmitempty {
			omitemptyFields[name] = struct{}{}
			continue
		}

		alwaysSerializedFields = append(alwaysSerializedFields, name)
	}
	for k := range omitemptyFields {
		delete(actualKeys, k)
	}
	sort.Strings(alwaysSerializedFields)

	requiredSet := make(map[string]struct{}, len(requiredParamsKeys))
	for _, k := range requiredParamsKeys {
		requiredSet[k] = struct{}{}
	}

	var missing, extra []string
	for k := range requiredSet {
		if _, ok := actualKeys[k]; !ok {
			missing = append(missing, k)
		}
	}
	for k := range actualKeys {
		if _, ok := requiredSet[k]; !ok {
			extra = append(extra, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) > 0 || len(extra) > 0 || len(requiredParamsKeys) != len(actualKeys) {
		t.Errorf(
			"requiredParamsKeys missing fields: [%s]; extra fields: [%s]; struct fields: [%s]",
			strings.Join(missing, ", "),
			strings.Join(extra, ", "),
			strings.Join(alwaysSerializedFields, ", "),
		)
	}
}

func TestCheckParamsIntegrity_NullValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")

	cfg := DefaultParametersConfig()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}

	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("unmarshal config to map: %v", err)
	}
	obj["darwinian"] = nil

	badData, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal mutated config: %v", err)
	}
	if err := os.WriteFile(path, badData, 0o644); err != nil {
		t.Fatalf("write params.json with null darwinian: %v", err)
	}

	errs := CheckParamsIntegrity(path)
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "null") {
		t.Fatalf("expected error to mention null, got %v", errs[0])
	}
}

func TestCheckParamsIntegrity_EmptyObject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")

	cfg := DefaultParametersConfig()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}

	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("unmarshal config to map: %v", err)
	}
	obj["darwinian"] = map[string]any{}

	badData, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal mutated config: %v", err)
	}
	if err := os.WriteFile(path, badData, 0o644); err != nil {
		t.Fatalf("write params.json with empty darwinian: %v", err)
	}

	errs := CheckParamsIntegrity(path)
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "empty") {
		t.Fatalf("expected error to mention empty, got %v", errs[0])
	}
}

func TestCheckParamsIntegrity_WrongType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")

	cfg := DefaultParametersConfig()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}

	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("unmarshal config to map: %v", err)
	}
	obj["darwinian"] = "not-an-object"

	badData, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal mutated config: %v", err)
	}
	if err := os.WriteFile(path, badData, 0o644); err != nil {
		t.Fatalf("write params.json with string darwinian: %v", err)
	}

	errs := CheckParamsIntegrity(path)
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "string") && !strings.Contains(errs[0].Error(), "type") {
		t.Fatalf("expected error to mention string or type, got %v", errs[0])
	}
}

func TestCheckParamsIntegrity_AllValidPasses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")

	cfg := DefaultParametersConfig()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write valid params.json: %v", err)
	}

	errs := CheckParamsIntegrity(path)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for valid params, got %d: %v", len(errs), errs)
	}
}

func TestValidateCalibration_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")

	cfg := DefaultParametersConfig()
	cfg.UpdatedAt = time.Now()
	cfg.Industry.ClassificationTree.Value.Segments = []IndustrySegmentConfig{
		{ID: "tech", Level: 1, RepresentativeStocks: []string{"2330.TW", "2454.TW"}},
		{ID: "tsmc", Level: 2, ParentID: "tech", RepresentativeStocks: []string{"2330.TW"}},
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write params.json: %v", err)
	}

	res, err := ValidateCalibration(path, 48*time.Hour)
	if err != nil {
		t.Fatalf("ValidateCalibration: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got issues: %v", res.Issues)
	}
	if res.SegmentsCount != 2 || res.L1Count != 1 || res.L2Count != 1 {
		t.Errorf("unexpected counts: segments=%d L1=%d L2=%d", res.SegmentsCount, res.L1Count, res.L2Count)
	}
}

func TestValidateCalibration_StaleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")

	cfg := DefaultParametersConfig()
	cfg.UpdatedAt = time.Now().Add(-72 * time.Hour)
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write params.json: %v", err)
	}

	res, _ := ValidateCalibration(path, 48*time.Hour)
	if res.OK {
		t.Fatal("expected OK=false for stale file")
	}
	if len(res.Issues) == 0 {
		t.Fatal("expected non-empty Issues for stale file")
	}
}

func TestValidateCalibration_EmptySegments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")

	cfg := DefaultParametersConfig()
	cfg.UpdatedAt = time.Now()
	cfg.Industry.ClassificationTree.Value.Segments = nil
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write params.json: %v", err)
	}

	res, _ := ValidateCalibration(path, 48*time.Hour)
	if res.OK {
		t.Fatal("expected OK=false for empty segments")
	}
	found := false
	for _, iss := range res.Issues {
		if strings.Contains(iss, "Segments is empty") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'Segments is empty' issue, got: %v", res.Issues)
	}
}

func TestValidateCalibration_BadL2Parent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")

	cfg := DefaultParametersConfig()
	cfg.UpdatedAt = time.Now()
	cfg.Industry.ClassificationTree.Value.Segments = []IndustrySegmentConfig{
		{ID: "tech", Level: 1, RepresentativeStocks: []string{"2330.TW"}},
		{ID: "tsmc", Level: 2, ParentID: "ghost", RepresentativeStocks: []string{"2330.TW"}},
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write params.json: %v", err)
	}

	res, _ := ValidateCalibration(path, 48*time.Hour)
	if res.OK {
		t.Fatal("expected OK=false for dangling L2 parent")
	}
	found := false
	for _, iss := range res.Issues {
		if strings.Contains(iss, "unknown ParentID") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'unknown ParentID' issue, got: %v", res.Issues)
	}
}
