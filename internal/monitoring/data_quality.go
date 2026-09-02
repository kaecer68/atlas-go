package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/constants"
)

// DataQualityCheck represents a single data quality check result.
type DataQualityCheck struct {
	Name      string         `json:"name"`
	Status    CheckStatus    `json:"status"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	CheckedAt time.Time      `json:"checked_at"`
	Duration  time.Duration  `json:"duration_ms"`
}

// CheckStatus represents the status of a quality check.
type CheckStatus string

const (
	StatusOK       CheckStatus = "ok"
	StatusWarning  CheckStatus = "warning"
	StatusCritical CheckStatus = "critical"
	StatusSkipped  CheckStatus = "skipped"
)

// DataQualityReport is the overall data quality report.
type DataQualityReport struct {
	Checks      []DataQualityCheck `json:"checks"`
	Overall     CheckStatus        `json:"overall"`
	Score       float64            `json:"score"`
	GeneratedAt time.Time          `json:"generated_at"`
}

// DataQualityChecker performs automated data quality checks across all data sources.
type DataQualityChecker struct {
	workDir   string
	ledgerDir string
}

// NewDataQualityChecker creates a new data quality checker.
func NewDataQualityChecker(workDir, ledgerDir string) *DataQualityChecker {
	return &DataQualityChecker{
		workDir:   workDir,
		ledgerDir: ledgerDir,
	}
}

// RunAll executes all data quality checks and returns a comprehensive report.
func (dq *DataQualityChecker) RunAll(ctx context.Context) *DataQualityReport {
	checks := []func(context.Context) DataQualityCheck{
		dq.checkAlertsFile,
		dq.checkLedgerFiles,
		dq.checkSessionFiles,
		dq.checkExperimentFiles,
		dq.checkConfigFiles,
		dq.checkPromptFiles,
		dq.checkDataDirectorySize,
		dq.checkFilePermissions,
	}

	report := &DataQualityReport{
		Checks:      make([]DataQualityCheck, 0, len(checks)),
		GeneratedAt: time.Now(),
	}

	scoreWeights := map[CheckStatus]float64{
		StatusOK:       100,
		StatusWarning:  60,
		StatusCritical: 0,
		StatusSkipped:  100,
	}

	totalScore := 0.0
	validChecks := 0

	for _, check := range checks {
		select {
		case <-ctx.Done():
			return report
		default:
		}

		start := time.Now()
		result := check(ctx)
		result.Duration = time.Since(start)
		result.CheckedAt = time.Now()

		if result.Status != StatusSkipped {
			totalScore += scoreWeights[result.Status]
			validChecks++
		}
		report.Checks = append(report.Checks, result)
	}

	if validChecks > 0 {
		report.Score = totalScore / float64(validChecks)
	}
	report.Overall = dq.determineOverallStatus(report.Checks)

	return report
}

func (dq *DataQualityChecker) checkAlertsFile(ctx context.Context) DataQualityCheck {
	check := DataQualityCheck{Name: "alerts_file"}

	alertsPath := filepath.Join(dq.workDir, "data", "state", "alerts", "alerts.jsonl")
	info, err := os.Stat(alertsPath)
	if err != nil {
		if os.IsNotExist(err) {
			check.Status = StatusWarning
			check.Message = "警報檔案不存在（首次啟動時正常）"
			return check
		}
		check.Status = StatusCritical
		check.Message = fmt.Sprintf("無法讀取警報檔案: %v", err)
		return check
	}

	if info.Size() == 0 {
		check.Status = StatusWarning
		check.Message = "警報檔案為空（尚無警報觸發）"
		check.Details = map[string]any{
			"path":       alertsPath,
			"size_bytes": 0,
			"modified":   info.ModTime(),
		}
		return check
	}

	if time.Since(info.ModTime()) > 24*time.Hour {
		check.Status = StatusWarning
		check.Message = "警報檔案超過 24 小時未更新"
	} else {
		check.Status = StatusOK
		check.Message = "警報檔案正常"
	}

	check.Details = map[string]any{
		"path":       alertsPath,
		"size_bytes": info.Size(),
		"modified":   info.ModTime(),
	}

	return check
}

func (dq *DataQualityChecker) checkLedgerFiles(ctx context.Context) DataQualityCheck {
	check := DataQualityCheck{Name: "ledger_files"}

	ledgerDir := filepath.Join(dq.ledgerDir)
	entries, err := os.ReadDir(ledgerDir)
	if err != nil {
		if os.IsNotExist(err) {
			check.Status = StatusWarning
			check.Message = "Ledger 目錄不存在"
			return check
		}
		check.Status = StatusCritical
		check.Message = fmt.Sprintf("無法讀取 Ledger 目錄: %v", err)
		return check
	}

	jsonlCount := 0
	totalSize := int64(0)
	var newestMod time.Time

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jsonl") {
			jsonlCount++
			info, err := entry.Info()
			if err != nil {
				continue
			}
			totalSize += info.Size()
			if info.ModTime().After(newestMod) {
				newestMod = info.ModTime()
			}
		}
	}

	if jsonlCount == 0 {
		check.Status = StatusWarning
		check.Message = "尚無 Ledger 檔案（尚未執行回測）"
		return check
	}

	check.Status = StatusOK
	check.Message = fmt.Sprintf("找到 %d 個 Ledger 檔案", jsonlCount)
	check.Details = map[string]any{
		"file_count":    jsonlCount,
		"total_size_mb": float64(totalSize) / (1024 * 1024),
		"newest_file":   newestMod.Format("2006-01-02 15:04:05"),
	}

	if totalSize > 50*1024*1024 {
		check.Status = StatusWarning
		check.Message = "Ledger 檔案總大小超過 50MB，建議考慮歸檔"
	}

	return check
}

func (dq *DataQualityChecker) checkSessionFiles(ctx context.Context) DataQualityCheck {
	check := DataQualityCheck{Name: "session_files"}

	sessionDir := filepath.Join(dq.workDir, "data", "state", "sessions")
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		if os.IsNotExist(err) {
			check.Status = StatusWarning
			check.Message = "Session 目錄不存在"
			return check
		}
		check.Status = StatusCritical
		check.Message = fmt.Sprintf("無法讀取 Session 目錄: %v", err)
		return check
	}

	jsonCount := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			jsonCount++
		}
	}

	if jsonCount == 0 {
		check.Status = StatusWarning
		check.Message = "尚無 Session 記錄（尚未執行任何分析）"
	} else {
		check.Status = StatusOK
		check.Message = fmt.Sprintf("找到 %d 個 Session 記錄", jsonCount)
	}

	check.Details = map[string]any{
		"session_count": jsonCount,
	}

	return check
}

func (dq *DataQualityChecker) checkExperimentFiles(ctx context.Context) DataQualityCheck {
	check := DataQualityCheck{Name: "experiment_files"}

	experimentDir := filepath.Join(dq.workDir, "data", "state", "experiments")
	entries, err := os.ReadDir(experimentDir)
	if err != nil {
		if os.IsNotExist(err) {
			check.Status = StatusSkipped
			check.Message = "實驗目錄不存在（未啟用實驗功能）"
			return check
		}
		check.Status = StatusCritical
		check.Message = fmt.Sprintf("無法讀取實驗目錄: %v", err)
		return check
	}

	jsonCount := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			jsonCount++
		}
	}

	check.Status = StatusOK
	check.Message = fmt.Sprintf("找到 %d 個實驗記錄", jsonCount)
	check.Details = map[string]any{
		"experiment_count": jsonCount,
	}

	return check
}

func (dq *DataQualityChecker) checkConfigFiles(ctx context.Context) DataQualityCheck {
	check := DataQualityCheck{Name: "config_files"}

	requiredFiles := []string{
		constants.AgentsConfigPath,
		constants.ParametersFile,
	}

	missing := []string{}
	for _, f := range requiredFiles {
		path := filepath.Join(dq.workDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			missing = append(missing, f)
		}
	}

	if len(missing) > 0 {
		check.Status = StatusCritical
		check.Message = fmt.Sprintf("缺少必要設定檔: %s", strings.Join(missing, ", "))
		return check
	}

	check.Status = StatusOK
	check.Message = "所有必要設定檔存在"
	check.Details = map[string]any{
		"checked_files": requiredFiles,
	}

	return check
}

func (dq *DataQualityChecker) checkPromptFiles(ctx context.Context) DataQualityCheck {
	check := DataQualityCheck{Name: "prompt_files"}

	agentsPath := filepath.Join(dq.workDir, "configs", "agents.json")
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		check.Status = StatusSkipped
		check.Message = "無法讀取 agents.json，跳過 prompt 檢查"
		return check
	}

	enabledAgents := parseEnabledAgents(data)
	if len(enabledAgents) == 0 {
		check.Status = StatusSkipped
		check.Message = "無啟用的 agent，跳過 prompt 檢查"
		return check
	}

	missingPrompts := []string{}
	promptsDir := filepath.Join(dq.workDir, "prompts", "agents")
	for _, agent := range enabledAgents {
		// agents.json carries the authoritative promptFile path (English
		// file id) while `name` is the Chinese display label — checking by
		// name produced false "missing prompt" criticals for every agent
		// (observed 2026-09-02: 5 agents flagged though all prompts exist).
		promptPath := agent.promptFile
		if promptPath != "" && !filepath.IsAbs(promptPath) {
			promptPath = filepath.Join(dq.workDir, promptPath)
		}
		if promptPath == "" {
			promptPath = filepath.Join(promptsDir, agent.name+".md")
			if _, err := os.Stat(promptPath); os.IsNotExist(err) {
				promptPath = filepath.Join(promptsDir, agent.name+".prompt.md")
			}
		}
		if _, err := os.Stat(promptPath); os.IsNotExist(err) {
			missingPrompts = append(missingPrompts, agent.name)
		}
	}

	if len(missingPrompts) > 0 {
		check.Status = StatusCritical
		check.Message = fmt.Sprintf("啟用的 agent 缺少 prompt 檔: %s", strings.Join(missingPrompts, ", "))
		return check
	}

	check.Status = StatusOK
	check.Message = fmt.Sprintf("所有 %d 個啟用 agent 都有對應 prompt 檔", len(enabledAgents))
	return check
}

func (dq *DataQualityChecker) checkDataDirectorySize(ctx context.Context) DataQualityCheck {
	check := DataQualityCheck{Name: "data_directory_size"}

	dataDir := filepath.Join(dq.workDir, "data")
	totalSize := int64(0)
	fileCount := 0

	err := filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			totalSize += info.Size()
			fileCount++
		}
		return nil
	})
	if err != nil {
		check.Status = StatusCritical
		check.Message = fmt.Sprintf("無法計算資料目錄大小: %v", err)
		return check
	}

	sizeMB := float64(totalSize) / (1024 * 1024)

	check.Status = StatusOK
	check.Message = fmt.Sprintf("資料目錄大小 %.1f MB (%d 個檔案)", sizeMB, fileCount)
	check.Details = map[string]any{
		"size_mb":    sizeMB,
		"file_count": fileCount,
	}

	if sizeMB > 500 {
		check.Status = StatusWarning
		check.Message = fmt.Sprintf("資料目錄超過 500 MB (%.1f MB)，建議清理或歸檔", sizeMB)
	}

	return check
}

func (dq *DataQualityChecker) checkFilePermissions(ctx context.Context) DataQualityCheck {
	check := DataQualityCheck{Name: "file_permissions"}

	dirsToCheck := []string{
		filepath.Join(dq.workDir, "data", "state"),
		filepath.Join(dq.ledgerDir),
	}

	writableCount := 0
	for _, dir := range dirsToCheck {
		testFile := filepath.Join(dir, ".write_test")
		if err := os.WriteFile(testFile, []byte{}, 0o644); err != nil {
			check.Status = StatusCritical
			check.Message = fmt.Sprintf("目錄不可寫: %s (%v)", dir, err)
			return check
		}
		_ = os.Remove(testFile)
		writableCount++
	}

	check.Status = StatusOK
	check.Message = fmt.Sprintf("所有 %d 個關鍵目錄可寫", writableCount)
	return check
}

func (dq *DataQualityChecker) determineOverallStatus(checks []DataQualityCheck) CheckStatus {
	hasCritical := false
	hasWarning := false

	for _, c := range checks {
		switch c.Status {
		case StatusCritical:
			hasCritical = true
		case StatusWarning:
			hasWarning = true
		}
	}

	if hasCritical {
		return StatusCritical
	}
	if hasWarning {
		return StatusWarning
	}
	return StatusOK
}

type agentPromptRef struct {
	name       string
	promptFile string
}

func parseEnabledAgents(data []byte) []agentPromptRef {
	var reg struct {
		Agents []struct {
			Name       string `json:"name"`
			PromptFile string `json:"promptFile"`
			Enabled    bool   `json:"enabled"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil
	}
	var agents []agentPromptRef
	for _, a := range reg.Agents {
		if a.Enabled {
			agents = append(agents, agentPromptRef{name: a.Name, promptFile: a.PromptFile})
		}
	}
	return agents
}
