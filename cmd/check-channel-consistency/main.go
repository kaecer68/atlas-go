// Command check-channel-consistency keeps the three channel-health
// configuration surfaces aligned:
//
//  1. contracts (internal/apigateway.ChannelContracts)
//  2. scheduled health/fetch tasks (cmd/atlas/data_sync_health_tasks.go)
//  3. Prometheus alert hysteresis (monitoring/rules)
//
// The k3 monitoring audit (#1877 / R4) showed that changing any surface in
// isolation creates silent false positives: a task can write health for a
// channel with no contract, a task interval can exceed the freshness window,
// and a Prometheus `for` duration can be shorter than the task interval that
// samples the health gauge. This command turns those drift modes into CI
// failures.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"gopkg.in/yaml.v3"
)

const (
	defaultTaskSource   = "cmd/atlas/data_sync_health_tasks.go"
	defaultAlertSource  = "monitoring/rules/channel_health_latent_staleness.yml"
	statusErrorAlert    = "ChannelHealthStatusError"
	maxDurationViewSize = 64
)

// TaskSpec is a scheduled task that writes Gateway channel health.
type TaskSpec struct {
	Name      string        `json:"name"`
	ChannelID string        `json:"channel_id"`
	Interval  time.Duration `json:"interval"`
}

// Violation is a single contract/schedule/alert mismatch.
type Violation struct {
	Check     string `json:"check"`
	ChannelID string `json:"channel_id,omitempty"`
	Source    string `json:"source,omitempty"`
	Detail    string `json:"detail"`
}

// alertRule is the subset of a Prometheus rule needed for consistency checks.
type alertRule struct {
	Alert string `yaml:"alert"`
	Expr  string `yaml:"expr"`
	For   string `yaml:"for"`
}

type prometheusRules struct {
	Groups []struct {
		Name  string      `yaml:"name"`
		Rules []alertRule `yaml:"rules"`
	} `yaml:"groups"`
}

func main() {
	jsonMode := false
	taskPath, alertPath := defaultTaskSource, defaultAlertSource
	for _, arg := range os.Args[1:] {
		switch {
		case arg == "--json":
			jsonMode = true
		case strings.HasPrefix(arg, "--tasks="):
			taskPath = strings.TrimPrefix(arg, "--tasks=")
		case strings.HasPrefix(arg, "--rules="):
			alertPath = strings.TrimPrefix(arg, "--rules=")
		case arg == "-h" || arg == "--help":
			usage()
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown argument: %s\n", arg)
			usage()
			os.Exit(2)
		}
	}

	violations, err := runChecks(taskPath, alertPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "channel consistency check failed: %v\n", err)
		os.Exit(2)
	}
	if jsonMode {
		if err := json.NewEncoder(os.Stdout).Encode(struct {
			Tasks      int         `json:"tasks"`
			Violations []Violation `json:"violations"`
		}{Tasks: len(violations), Violations: violations}); err != nil {
			fmt.Fprintf(os.Stderr, "json encode: %v\n", err)
			os.Exit(2)
		}
	} else {
		printViolations(violations)
	}
	if len(violations) > 0 {
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: check-channel-consistency [--json] [--tasks=FILE] [--rules=FILE]

Checks that scheduled channel-health tasks and Prometheus alert hysteresis
stay aligned with apigateway.ChannelContracts().
`)
}

func runChecks(taskPath, alertPath string) ([]Violation, error) {
	tasks, err := parseScheduledTasks(taskPath)
	if err != nil {
		return nil, fmt.Errorf("parse task source %s: %w", taskPath, err)
	}
	alerts, err := parseAlertRules(alertPath)
	if err != nil {
		return nil, fmt.Errorf("parse alert rules %s: %w", alertPath, err)
	}

	registry := apigateway.ChannelContracts()
	violations := make([]Violation, 0)
	for _, v := range registry.Validate() {
		violations = append(violations, Violation{
			Check:     "contract_registry." + v.Check,
			ChannelID: v.ChannelID,
			Source:    "internal/apigateway/channel_contract.go",
			Detail:    v.Detail,
		})
	}
	violations = append(violations, checkSchedules(tasks, registry)...)
	violations = append(violations, checkAlertFor(tasks, alerts)...)

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Check != violations[j].Check {
			return violations[i].Check < violations[j].Check
		}
		if violations[i].ChannelID != violations[j].ChannelID {
			return violations[i].ChannelID < violations[j].ChannelID
		}
		return violations[i].Detail < violations[j].Detail
	})
	return violations, nil
}

// parseScheduledTasks extracts every &apigateway.ScheduledTask literal that
// declares a ChannelID. Tasks without a ChannelID are internal maintenance
// tasks and are not bound to one channel contract.
func parseScheduledTasks(path string) ([]TaskSpec, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	var tasks []TaskSpec
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || lit.Type == nil {
			return true
		}
		switch typ := lit.Type.(type) {
		case *ast.SelectorExpr:
			if typ.Sel.Name != "ScheduledTask" {
				return true
			}
		case *ast.Ident:
			if typ.Name != "ScheduledTask" {
				return true
			}
		default:
			return true
		}

		var spec TaskSpec
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			switch key.Name {
			case "Name":
				if s, ok := stringValue(kv.Value); ok {
					spec.Name = s
				}
			case "ChannelID":
				if s, ok := stringValue(kv.Value); ok {
					spec.ChannelID = s
				}
			case "Interval":
				if d, ok := durationValue(kv.Value); ok {
					spec.Interval = d
				}
			}
		}
		if spec.ChannelID != "" {
			if spec.Name == "" {
				spec.Name = spec.ChannelID
			}
			tasks = append(tasks, spec)
		}
		return true
	})
	if len(tasks) == 0 {
		return nil, fmt.Errorf("no ScheduledTask specs with ChannelID found")
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Name < tasks[j].Name })
	return tasks, nil
}

// parseAlertRules reads only the rule file's alert name/expr/for fields.
func parseAlertRules(path string) ([]alertRule, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var parsed prometheusRules
	if err := yaml.Unmarshal(b, &parsed); err != nil {
		return nil, err
	}
	var rules []alertRule
	for _, group := range parsed.Groups {
		rules = append(rules, group.Rules...)
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("no Prometheus rules found")
	}
	return rules, nil
}

// checkSchedules verifies:
//   - a scheduled channel-health writer has a registered channel
//   - it has an explicit contract (not a silently inherited default)
//   - the task interval can produce data inside FreshnessWindow
//   - no derived/external channelID has acquired a schedule
func checkSchedules(tasks []TaskSpec, registry *apigateway.ChannelContractRegistry) []Violation {
	known := make(map[string]bool, len(apigateway.KnownChannelIDs()))
	for _, id := range apigateway.KnownChannelIDs() {
		known[id] = true
	}

	var violations []Violation
	for _, task := range tasks {
		if !known[task.ChannelID] {
			violations = append(violations, Violation{
				Check:     "schedule.unknown_channel",
				ChannelID: task.ChannelID,
				Source:    "task:" + task.Name,
				Detail:    "scheduled task writes a channel that is not registered; derived records must not have schedules",
			})
			continue
		}
		contract, ok := registry.Lookup(task.ChannelID)
		if !ok {
			violations = append(violations, Violation{
				Check:     "schedule.missing_contract",
				ChannelID: task.ChannelID,
				Source:    "task:" + task.Name,
				Detail:    "scheduled task has no explicit ChannelContract (default fallback is not allowed)",
			})
			continue
		}
		if task.Interval <= 0 {
			violations = append(violations, Violation{
				Check:     "schedule.invalid_interval",
				ChannelID: task.ChannelID,
				Source:    "task:" + task.Name,
				Detail:    "task Interval must be > 0",
			})
			continue
		}
		if window := contract.EffectiveFreshnessWindow(); task.Interval > window {
			violations = append(violations, Violation{
				Check:     "schedule.interval_exceeds_freshness",
				ChannelID: task.ChannelID,
				Source:    "task:" + task.Name,
				Detail: fmt.Sprintf("Interval (%s) > FreshnessWindow (%s): health data can become stale between attempts",
					task.Interval, window),
			})
		}
	}
	return violations
}

// checkAlertFor verifies that ChannelHealthStatusError hysteresis is not
// shorter than the interval that samples the health status. A rule with no
// channel selector applies to every scheduled channel.
func checkAlertFor(tasks []TaskSpec, alerts []alertRule) []Violation {
	var selected []alertRule
	for _, rule := range alerts {
		if rule.Alert == statusErrorAlert {
			selected = append(selected, rule)
		}
	}
	if len(selected) == 0 {
		return []Violation{{
			Check:  "alert.missing_rule",
			Source: "monitoring/rules",
			Detail: fmt.Sprintf("no alert named %s", statusErrorAlert),
		}}
	}

	var violations []Violation
	for _, task := range tasks {
		applicable := make([]alertRule, 0, len(selected))
		for _, rule := range selected {
			if ruleAppliesToChannel(rule.Expr, task.ChannelID) {
				applicable = append(applicable, rule)
			}
		}
		if len(applicable) == 0 {
			violations = append(violations, Violation{
				Check:     "alert.no_hysteresis_rule",
				ChannelID: task.ChannelID,
				Source:    "task:" + task.Name,
				Detail:    "no ChannelHealthStatusError rule covers this scheduled channel",
			})
			continue
		}

		shortest := time.Duration(0)
		var shortestFor string
		for _, rule := range applicable {
			d, err := time.ParseDuration(rule.For)
			if err != nil || d <= 0 {
				violations = append(violations, Violation{
					Check:     "alert.invalid_for",
					ChannelID: task.ChannelID,
					Source:    "alert:" + rule.Alert,
					Detail:    fmt.Sprintf("invalid Prometheus `for` value %q", rule.For),
				})
				continue
			}
			if shortest == 0 || d < shortest {
				shortest, shortestFor = d, rule.For
			}
		}
		if shortest > 0 && shortest < task.Interval {
			violations = append(violations, Violation{
				Check:     "alert.for_below_interval",
				ChannelID: task.ChannelID,
				Source:    "task:" + task.Name,
				Detail: fmt.Sprintf("ChannelHealthStatusError for=%s is below task Interval=%s; a single failed sample can page before the next attempt",
					shortestFor, task.Interval),
			})
		}
	}
	return violations
}

// ruleAppliesToChannel supports the three Prometheus matcher forms used by
// this repository: no selector (all channels), channel="x", and channel!="x".
func ruleAppliesToChannel(expr, channelID string) bool {
	labelRe := regexp.MustCompile(`atlas_channel_health_status\s*\{([^}]*)\}`)
	matches := labelRe.FindAllStringSubmatch(expr, -1)
	if len(matches) == 0 {
		return true
	}
	matcherRe := regexp.MustCompile(`channel\s*(=~|!~|!=|=)\s*"([^"]+)"`)
	applies := true
	for _, match := range matches {
		for _, raw := range strings.Split(match[1], ",") {
			m := matcherRe.FindStringSubmatch(raw)
			if m == nil {
				continue
			}
			value := m[2]
			var matched bool
			switch m[1] {
			case "=":
				matched = value == channelID
			case "!=":
				matched = value != channelID
			case "=~":
				matched = regexp.MustCompile(value).MatchString(channelID)
			case "!~":
				matched = !regexp.MustCompile(value).MatchString(channelID)
			}
			applies = applies && matched
		}
	}
	return applies
}

func stringValue(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

// durationValue supports the literal forms used in ScheduledTask
// registrations (e.g. 5*time.Minute, 1*time.Hour, time.Hour).
func durationValue(expr ast.Expr) (time.Duration, bool) {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		return selectorDuration(e)
	case *ast.BinaryExpr:
		if e.Op != token.MUL {
			return 0, false
		}
		left, lok := durationValue(e.X)
		right, rok := durationValue(e.Y)
		if !lok || !rok {
			return 0, false
		}
		return left * right, true
	case *ast.BasicLit:
		if e.Kind != token.INT {
			return 0, false
		}
		n, err := strconv.ParseInt(e.Value, 10, 64)
		if err != nil || n <= 0 {
			return 0, false
		}
		return time.Duration(n), true
	default:
		return 0, false
	}
}

func selectorDuration(expr *ast.SelectorExpr) (time.Duration, bool) {
	x, ok := expr.X.(*ast.Ident)
	if !ok || x.Name != "time" {
		return 0, false
	}
	switch expr.Sel.Name {
	case "Nanosecond":
		return time.Nanosecond, true
	case "Microsecond":
		return time.Microsecond, true
	case "Millisecond":
		return time.Millisecond, true
	case "Second":
		return time.Second, true
	case "Minute":
		return time.Minute, true
	case "Hour":
		return time.Hour, true
	default:
		return 0, false
	}
}

func printViolations(violations []Violation) {
	if len(violations) == 0 {
		fmt.Println("✅ channel contracts, schedules, and alert hysteresis are consistent")
		return
	}
	fmt.Printf("❌ %d channel consistency violations:\n", len(violations))
	for _, v := range violations {
		channel := "-"
		if v.ChannelID != "" {
			channel = v.ChannelID
		}
		source := "-"
		if v.Source != "" {
			source = v.Source
		}
		fmt.Printf("  - [%s] channel=%s source=%s: %s\n", v.Check, channel, source, v.Detail)
	}
}
