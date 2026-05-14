package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ParameterHealthReport struct {
	TotalParameters        int              `json:"total_parameters"`
	ParametersWithCitation int              `json:"parameters_with_citation"`
	ParametersWithTodo     int              `json:"parameters_with_todo"`
	ParametersCalibrated   int              `json:"parameters_calibrated"`
	ParametersHeuristic    int              `json:"parameters_heuristic"`
	ParametersEmpirical    int              `json:"parameters_empirical"`
	ParametersLiterature   int              `json:"parameters_literature"`
	HighEvidenceCount      int              `json:"high_evidence_count"`
	MediumEvidenceCount    int              `json:"medium_evidence_count"`
	LowEvidenceCount       int              `json:"low_evidence_count"`
	Modules                []ModuleReport   `json:"modules"`
	Issues                 []ParameterIssue `json:"issues"`
	Recommendations        []string         `json:"recommendations"`
}

type ModuleReport struct {
	Name           string `json:"name"`
	ParameterCount int    `json:"parameter_count"`
	WithCitation   int    `json:"with_citation"`
	WithTodo       int    `json:"with_todo"`
	Calibrated     int    `json:"calibrated"`
	HighEvidence   int    `json:"high_evidence"`
	MediumEvidence int    `json:"medium_evidence"`
	LowEvidence    int    `json:"low_evidence"`
	HeuristicOnly  int    `json:"heuristic_only"`
}

type ParameterIssue struct {
	Module     string `json:"module"`
	Parameter  string `json:"parameter"`
	Issue      string `json:"issue"`
	Severity   string `json:"severity"`
	Suggestion string `json:"suggestion"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "parameter-health-check: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	workDir, _ := os.Getwd()
	configPath := filepath.Join(workDir, "configs", "parameters.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	report := analyzeParameters(config)

	// Output as JSON
	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	fmt.Println(string(out))

	return nil
}

func analyzeParameters(config map[string]any) ParameterHealthReport {
	report := ParameterHealthReport{
		Modules:         make([]ModuleReport, 0),
		Issues:          make([]ParameterIssue, 0),
		Recommendations: make([]string, 0),
	}

	for moduleName, moduleValue := range config {
		if moduleName == "version" || moduleName == "updated_at" {
			continue
		}

		moduleData, ok := moduleValue.(map[string]any)
		if !ok {
			continue
		}

		moduleReport := analyzeModule(moduleName, moduleData, &report)
		report.Modules = append(report.Modules, moduleReport)
	}

	// Generate recommendations
	if report.ParametersWithTodo > 0 {
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("Address %d parameters with TODO items", report.ParametersWithTodo))
	}
	if report.LowEvidenceCount > 20 {
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("Review %d low-evidence parameters for potential calibration", report.LowEvidenceCount))
	}
	if report.ParametersHeuristic > 150 {
		report.Recommendations = append(report.Recommendations,
			"Consider empirical validation for heuristic-dominant parameters")
	}

	return report
}

func analyzeModule(moduleName string, moduleData map[string]any, report *ParameterHealthReport) ModuleReport {
	moduleReport := ModuleReport{Name: moduleName}

	for paramName, paramValue := range moduleData {
		paramData, ok := paramValue.(map[string]any)
		if !ok {
			continue
		}

		// Check if this is a parameter object
		if _, hasValue := paramData["value"]; !hasValue {
			// Might be a nested structure (like cycle_thresholds)
			if nested, isMap := paramValue.(map[string]any); isMap {
				for nestedName, nestedValue := range nested {
					if nestedParam, isParam := nestedValue.(map[string]any); isParam {
						if _, hasVal := nestedParam["value"]; hasVal {
							moduleReport.ParameterCount++
							report.TotalParameters++
							analyzeParameter(moduleName, nestedName, nestedParam, report, &moduleReport)
						}
					}
				}
			}
			continue
		}

		moduleReport.ParameterCount++
		report.TotalParameters++
		analyzeParameter(moduleName, paramName, paramData, report, &moduleReport)
	}

	return moduleReport
}

func analyzeParameter(moduleName, paramName string, paramData map[string]any, report *ParameterHealthReport, moduleReport *ModuleReport) {
	// Check for citation
	if _, hasCitation := paramData["citation"]; hasCitation {
		report.ParametersWithCitation++
		moduleReport.WithCitation++

		// Check evidence quality
		if citation, ok := paramData["citation"].(map[string]any); ok {
			if eq, ok := citation["evidence_quality"].(string); ok {
				switch eq {
				case "high":
					report.HighEvidenceCount++
					moduleReport.HighEvidence++
				case "medium":
					report.MediumEvidenceCount++
					moduleReport.MediumEvidence++
				case "low":
					report.LowEvidenceCount++
					moduleReport.LowEvidence++
				}
			}
		}
	} else {
		// Missing citation
		report.Issues = append(report.Issues, ParameterIssue{
			Module:     moduleName,
			Parameter:  paramName,
			Issue:      "Missing citation annotation",
			Severity:   "medium",
			Suggestion: "Add citation block with source_reference, evidence_quality, and validation_method",
		})
	}

	// Check source
	if source, ok := paramData["source"].(string); ok {
		switch source {
		case "heuristic":
			report.ParametersHeuristic++
			moduleReport.HeuristicOnly++
		case "empirical":
			report.ParametersEmpirical++
		case "literature":
			report.ParametersLiterature++
		}
	}

	// Check for calibration_method
	if _, hasCalibration := paramData["calibration_method"]; hasCalibration {
		report.ParametersCalibrated++
		moduleReport.Calibrated++
	}

	// Check for TODO
	if todo, hasTodo := paramData["todo"].(string); hasTodo && todo != "" {
		report.ParametersWithTodo++
		moduleReport.WithTodo++

		// Add issue for urgent TODOs
		if strings.Contains(strings.ToLower(todo), "urgent") ||
			strings.Contains(strings.ToLower(todo), "fix") {
			report.Issues = append(report.Issues, ParameterIssue{
				Module:     moduleName,
				Parameter:  paramName,
				Issue:      fmt.Sprintf("TODO: %s", todo),
				Severity:   "high",
				Suggestion: "Address immediately",
			})
		}
	}

	// Check for common issues
	if rationale, ok := paramData["rationale"].(string); ok {
		if strings.Contains(strings.ToLower(rationale), "warning") {
			report.Issues = append(report.Issues, ParameterIssue{
				Module:     moduleName,
				Parameter:  paramName,
				Issue:      "Rationale contains WARNING",
				Severity:   "high",
				Suggestion: "Review and fix the underlying issue",
			})
		}
	}
}
