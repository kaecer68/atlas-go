package narrative_test

import (
	"encoding/json"
	"testing"

	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/testutil/snapshot"
)

// TestKnowledgeBase_PublicAPI locks the exported surface of knowledge_base.go
// and knowledge_base_templates.go. Per #611 sub-issue-3 (NarrativeEngine /
// KnowledgeBase split), any change to public signatures fails this test.
func TestKnowledgeBase_PublicAPI(t *testing.T) {
	snap, err := snapshot.CaptureAPIs("knowledge_base.go", "knowledge_base_templates.go")
	if err != nil {
		t.Fatalf("CaptureAPIs: %v", err)
	}
	snapshot.AssertAPI(t, snap, "testdata/knowledge_base_api.golden.json")
}

// TestDefaultTemplates_Golden locks the built-in causal narrative templates.
// Per internal/narrative/AGENTS.md, "所有主題的命中率統一由 DefaultTemplates()
// 提供，hitRateForTheme() 自動查表" — DefaultTemplates is the canonical
// source of truth for all theme hit rates. Refactor must preserve all
// fields exactly (ID, name, trigger theme, hit rate, causal chain, sector
// impact classifications).
func TestDefaultTemplates_Golden(t *testing.T) {
	templates := narrative.DefaultTemplates()
	if len(templates) == 0 {
		t.Fatal("DefaultTemplates returned empty slice — at minimum 18 themes expected per AGENTS.md")
	}

	data, err := json.MarshalIndent(templates, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	snapshot.AssertGoldenJSON(t, json.RawMessage(data), "testdata/default_templates.golden.json")
}

// TestDefaultThemeDurations_Golden locks the built-in theme durations.
// Per internal/narrative/AGENTS.md, "所有持續時間定義於 DefaultThemeDurations()
// 為 EventLifecycleManager 與 KB detector 的唯一權威來源" — refactor must
// preserve every theme's price-discovery + impact duration exactly.
func TestDefaultThemeDurations_Golden(t *testing.T) {
	durations := narrative.DefaultThemeDurations()
	if len(durations) == 0 {
		t.Fatal("DefaultThemeDurations returned empty map — at minimum 18 themes expected per AGENTS.md")
	}

	data, err := json.MarshalIndent(durations, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	snapshot.AssertGoldenJSON(t, json.RawMessage(data), "testdata/default_theme_durations.golden.json")
}

// BenchmarkDefaultTemplates captures the cost of the canonical hit-rate
// accessor. Called on every KB chain-match (high-frequency path).
func BenchmarkDefaultTemplates(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = narrative.DefaultTemplates()
	}
}

// BenchmarkDefaultThemeDurations captures the cost of the lifecycle
// duration accessor. Called on every NarrativeEvent construction.
func BenchmarkDefaultThemeDurations(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = narrative.DefaultThemeDurations()
	}
}
