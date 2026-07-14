package eventdriven

import (
	"sort"
	"testing"
)

// TestForcesForDirection_NewKeywords 驗證 PR-A minor polish 新增的中文關鍵字
// 對應到正確的 capital force 類別。
// 修前 driving_events = ["法說會旺季", "期貨結算日", "配息資金回流"] 因為沒有對應
// keyword → predicted_forces 永遠是空 array。
func TestForcesForDirection_NewKeywords(t *testing.T) {
	cases := []struct {
		name     string
		drivers  []string
		expected []string
	}{
		{
			name:     "法說會 should map to institutional+foreign",
			drivers:  []string{"法說會旺季"},
			expected: []string{"foreign", "institutional"},
		},
		{
			name:     "期貨結算 should map to dealer",
			drivers:  []string{"期貨結算日"},
			expected: []string{"dealer"},
		},
		{
			name:     "配息資金回流 should map to retail+institutional",
			drivers:  []string{"配息資金回流"},
			expected: []string{"institutional", "retail"},
		},
		{
			name:     "除權息 should map to retail+institutional",
			drivers:  []string{"除權息旺季"},
			expected: []string{"institutional", "retail"},
		},
		{
			name:     "股東會 should map to institutional",
			drivers:  []string{"股東會旺季"},
			expected: []string{"institutional"},
		},
		{
			name:     "English keywords still work",
			drivers:  []string{"MSCI rebalance"},
			expected: []string{"foreign"},
		},
		{
			name:     "All three real-world events together cover all 3 force types",
			drivers:  []string{"法說會旺季", "期貨結算日", "配息資金回流"},
			expected: []string{"dealer", "foreign", "institutional", "retail"},
		},
		{
			name:     "Unknown event yields no forces",
			drivers:  []string{"完全不認識的事件"},
			expected: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := forcesForDirection(tc.drivers)
			sort.Strings(got)
			if len(got) != len(tc.expected) {
				t.Fatalf("forcesForDirection(%v) = %v, want %v", tc.drivers, got, tc.expected)
			}
			for i, f := range got {
				if f != tc.expected[i] {
					t.Errorf("forcesForDirection(%v)[%d] = %q, want %q", tc.drivers, i, f, tc.expected[i])
				}
			}
		})
	}
}
