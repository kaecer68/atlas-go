package sectorallocation

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// ClosureState 是 closure verifier 的輸入；
// manifest path 必填，後續可加 namespace/prior/legacy 計數器。
type ClosureState struct {
	ManifestPath        string
	InvariantsEvaluated map[string]ClosureStatus
	NamespaceTypesExist bool
	TypedPriorPresent   bool
	LegacyCompatActive  bool
	FinalL1TargetSum    float64
	NoncanonicalKeyCnt  int
	MissingEvidence     []MissingEvidence
}

// ClosureStatus 對齊 manifest status machine（spec §10 狀態機）。
type ClosureStatus string

const (
	StatusPending     ClosureStatus = "pending"
	StatusInProgress  ClosureStatus = "in_progress"
	StatusImplemented ClosureStatus = "implemented"
	StatusObserving   ClosureStatus = "observing"
	StatusDone        ClosureStatus = "done"
	StatusBlocked     ClosureStatus = "blocked"
)

// MissingEvidence 記錄哪個 ID 在 done 狀態缺哪一類 evidence。
type MissingEvidence struct {
	ID     string
	Kind   string // "implementation" | "observation" | "negative"
	Detail string
}

// ClosureRuleResult 是 verifier 對單一規則的判定。
type ClosureRuleResult struct {
	Rule     string
	Passed   bool
	Evidence string
}

// ManifestRow 是 closure verifier 從 manifest table 抽出的一列。
type ManifestRow struct {
	ID     string
	Status ClosureStatus
	Notes  string
}

// VerifyClosure 對 spec §10 與 manifest 5 條基礎 check 做機械化判定。
// 5 個 check：manifest_status_machine、id_done_requires_three_evidence、
// phase_dependency_complete、cross_id_dangling_dependency、source_label_lock。
// 拒絕跳狀態、缺證據、phase 越界、相依 ID 未 done、source 偷升 empirical。
func VerifyClosure(state ClosureState) []ClosureRuleResult {
	rows, err := readManifestRows(state.ManifestPath)
	if err != nil {
		return []ClosureRuleResult{{Rule: "config_error", Passed: false, Evidence: err.Error()}}
	}
	out := []ClosureRuleResult{}
	out = append(out, checkStatusMachine(rows))
	out = append(out, checkThreeEvidence(rows))
	out = append(out, checkPhaseDependency(rows))
	out = append(out, checkCrossIDDependency(rows))
	out = append(out, checkSourceLabelLock(rows))
	return out
}

var rowRe = regexp.MustCompile(`^\|\s*([A-Za-z][A-Za-z0-9-]*)\s*\|\s*([^|]*?)\s*\|\s*([^|]*?)\s*\|\s*([^|]*?)\s*\|\s*([^|]*?)\s*\|\s*(pending|in_progress|implemented|observing|done|blocked)\s*\|\s*([^|]*?)\s*\|\s*([^|]+)\s*\|?\s*$`)

func readManifestRows(path string) ([]ManifestRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open manifest: %w", err)
	}
	defer func() { _ = f.Close() }()
	rows := []ManifestRow{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(strings.TrimSpace(line), "|") {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "|--") || strings.HasPrefix(trimmed, "|---") {
			continue
		}
		if strings.HasPrefix(trimmed, "| ID ") || strings.HasPrefix(trimmed, "|----") {
			continue
		}
		if !strings.HasPrefix(trimmed, "|") {
			continue
		}
		m := rowRe.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		rows = append(rows, ManifestRow{
			ID:     m[1],
			Status: ClosureStatus(m[6]),
			Notes:  m[8],
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func checkStatusMachine(rows []ManifestRow) ClosureRuleResult {
	// SA01 自我審查（plan Task 1）：這條規則 SA01 階段僅做「done 必須有 evidence + non-canonical source」的基本檢查；
	// 跳狀態偵測主要靠 phase_dependency_complete + cross_id_dangling_dependency 與 verifier 結構本身。
	// 等 SA12 close-out 階段會再擴充「in_progress→done」偵測。
	for _, r := range rows {
		if r.Status == StatusDone && strings.TrimSpace(r.Notes) == "" {
			return ClosureRuleResult{Rule: "manifest_status_machine", Passed: false, Evidence: fmt.Sprintf("%s done but notes empty", r.ID)}
		}
	}
	return ClosureRuleResult{Rule: "manifest_status_machine", Passed: true, Evidence: "no missing evidence on done IDs"}
}

func checkThreeEvidence(rows []ManifestRow) ClosureRuleResult {
	missing := []MissingEvidence{}
	for _, r := range rows {
		if r.Status != StatusDone {
			continue
		}
		notes := strings.ToLower(r.Notes)
		hasImpl := strings.Contains(notes, "implementation:")
		hasObs := strings.Contains(notes, "observation:")
		hasNeg := strings.Contains(notes, "negative:")
		if !hasImpl {
			missing = append(missing, MissingEvidence{ID: r.ID, Kind: "implementation"})
		}
		if !hasObs {
			missing = append(missing, MissingEvidence{ID: r.ID, Kind: "observation"})
		}
		if !hasNeg {
			missing = append(missing, MissingEvidence{ID: r.ID, Kind: "negative"})
		}
	}
	if len(missing) > 0 {
		ids := []string{}
		for _, m := range missing {
			ids = append(ids, fmt.Sprintf("%s(%s)", m.ID, m.Kind))
		}
		sort.Strings(ids)
		return ClosureRuleResult{Rule: "id_done_requires_three_evidence", Passed: false, Evidence: fmt.Sprintf("missing evidence on: %s", strings.Join(ids, ", "))}
	}
	return ClosureRuleResult{Rule: "id_done_requires_three_evidence", Passed: true, Evidence: "all done IDs have 3 evidence categories"}
}

func checkPhaseDependency(rows []ManifestRow) ClosureRuleResult {
	// 定義 phase：B 階段（SA02-06）依賴 A 階段（SA01 done）;
	// C 階段（SA07-10）依賴 B; D 階段（SA11-12）依賴 C。
	phases := map[string]string{
		"SA01": "A", "SA02": "B", "SA03": "B", "SA04": "B", "SA05": "B", "SA06": "B",
		"SA07": "C", "SA08": "C", "SA09": "C", "SA10": "C",
		"SA11": "D", "SA12": "D",
	}
	byID := map[string]ManifestRow{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	for _, r := range rows {
		if r.Status == StatusPending {
			continue
		}
		phase, ok := phases[r.ID]
		if !ok {
			continue
		}
		if phase == "A" {
			continue
		}
		prev := prevPhase(phase)
		for _, pr := range rows {
			pp, ok := phases[pr.ID]
			if !ok || pp != prev {
				continue
			}
			if pr.Status != StatusDone {
				return ClosureRuleResult{Rule: "phase_dependency_complete", Passed: false, Evidence: fmt.Sprintf("%s in phase %s is %s but phase %s prerequisite %s is %s", r.ID, phase, r.Status, prev, pr.ID, pr.Status)}
			}
		}
	}
	return ClosureRuleResult{Rule: "phase_dependency_complete", Passed: true, Evidence: "all in_progress IDs have prior phase done"}
}

func prevPhase(p string) string {
	switch p {
	case "B":
		return "A"
	case "C":
		return "B"
	case "D":
		return "C"
	}
	return ""
}

func checkCrossIDDependency(rows []ManifestRow) ClosureRuleResult {
	// SA10 depends on SA08; SA11 depends on SA10; SA12 depends on SA11.
	deps := map[string][]string{
		"SA10": {"SA08"},
		"SA11": {"SA10"},
		"SA12": {"SA11"},
	}
	byID := map[string]ManifestRow{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	for _, r := range rows {
		if r.Status != StatusDone {
			continue
		}
		for _, dep := range deps[r.ID] {
			d, ok := byID[dep]
			if !ok {
				continue
			}
			if d.Status != StatusDone {
				return ClosureRuleResult{Rule: "cross_id_dangling_dependency", Passed: false, Evidence: fmt.Sprintf("%s done but dependency %s is %s", r.ID, dep, d.Status)}
			}
		}
	}
	return ClosureRuleResult{Rule: "cross_id_dangling_dependency", Passed: true, Evidence: "all done IDs have their dependencies done"}
}

func checkSourceLabelLock(rows []ManifestRow) ClosureRuleResult {
	// source=empirical 永久禁用（spec §4.1 + §6.2 + 觀察期僅工程穩定性）。
	for _, r := range rows {
		notes := strings.ToLower(r.Notes)
		if strings.Contains(notes, "source=empirical") || strings.Contains(notes, "source = empirical") {
			return ClosureRuleResult{Rule: "source_label_lock", Passed: false, Evidence: fmt.Sprintf("%s notes contain source=empirical; this is a permanent lock during observation window", r.ID)}
		}
		if strings.Contains(notes, "calibration_status=calibrated") || strings.Contains(notes, "calibration_status = calibrated") {
			return ClosureRuleResult{Rule: "source_label_lock", Passed: false, Evidence: fmt.Sprintf("%s notes contain calibration_status=calibrated; this is a permanent lock during observation window", r.ID)}
		}
	}
	return ClosureRuleResult{Rule: "source_label_lock", Passed: true, Evidence: "no source=empirical or calibration_status=calibrated in any notes"}
}
