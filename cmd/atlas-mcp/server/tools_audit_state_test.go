package server

// feat/20260807-mcp-audit-state-tool — M6 audit_state 工具測試。
//
// 驗證 audit_state 回傳憲章審計追蹤表快照的結構正確性與統計一致性。

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAuditState_ReturnsSnapshot(t *testing.T) {
	s := &server{}
	_, out, err := s.handleAuditState(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handleAuditState: %v", err)
	}

	if out.DocVersion != "v1.1c" {
		t.Errorf("doc_version = %q, want v1.1c", out.DocVersion)
	}
	if out.HeadCommit == "" {
		t.Error("head_commit empty")
	}

	// §附錄 D：22 個審計項目（P0 13 + P1 6 + P2 1 + aligned 2）
	if len(out.AuditItems) != 22 {
		t.Errorf("audit_items = %d, want 22", len(out.AuditItems))
	}

	// 統計一致性：P0 13 項全部 done
	p0Total, p0Done := 0, 0
	for _, it := range out.AuditItems {
		if it.Level == "P0" {
			p0Total++
			if it.Status == AuditStatusDone {
				p0Done++
			}
		}
	}
	if p0Total != 13 {
		t.Errorf("P0 total = %d, want 13", p0Total)
	}
	if p0Done != 13 {
		t.Errorf("P0 done = %d, want 13 (all P0 must be complete)", p0Done)
	}

	// 統計欄位與明細一致
	if out.Stats.Total != 22 || out.Stats.Done != 22 {
		t.Errorf("stats total/done = %d/%d, want 22/22", out.Stats.Total, out.Stats.Done)
	}
	if out.Stats.P0Total != 13 || out.Stats.P0Done != 13 {
		t.Errorf("stats P0 total/done = %d/%d, want 13/13", out.Stats.P0Total, out.Stats.P0Done)
	}
}

func TestAuditState_GovernanceGroups(t *testing.T) {
	s := &server{}
	_, out, err := s.handleAuditState(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handleAuditState: %v", err)
	}

	// §附錄 F：F1-F5 + M1-M6 + X1-X3 = 14 行
	if len(out.Governance) != 14 {
		t.Fatalf("governance = %d, want 14", len(out.Governance))
	}

	// 群組覆蓋
	groups := map[string]int{}
	for _, g := range out.Governance {
		groups[g.Group]++
	}
	if groups["fmx"] != 5 {
		t.Errorf("fmx group = %d, want 5 (F1-F5)", groups["fmx"])
	}
	if groups["mcp"] != 6 {
		t.Errorf("mcp group = %d, want 6 (M1-M6)", groups["mcp"])
	}
	if groups["enforce"] != 3 {
		t.Errorf("enforce group = %d, want 3 (X1-X3)", groups["enforce"])
	}

	// M6 本工具標記完成
	var m6 *GovernanceItem
	for i := range out.Governance {
		if out.Governance[i].ID == "M6" {
			m6 = &out.Governance[i]
			break
		}
	}
	if m6 == nil {
		t.Fatal("M6 governance item missing")
	}
	if m6.Status != AuditStatusDone {
		t.Errorf("M6 status = %q, want done (audit_state 本工具已公開)", m6.Status)
	}

	// 統計欄位
	if out.Stats.GovernanceDone != 13 {
		t.Errorf("governance done = %d, want 13 (v1.1c: F1-F4 + M1-M6 + X1-X3)", out.Stats.GovernanceDone)
	}
	if out.Stats.GovernancePartial != 0 {
		t.Errorf("governance partial = %d, want 0 (v1.1c)", out.Stats.GovernancePartial)
	}
	if out.Stats.GovernanceNotStart != 1 {
		t.Errorf("governance not_start = %d, want 1 (F5, 待 T27)", out.Stats.GovernanceNotStart)
	}
}

func TestAuditState_JSONSerializable(t *testing.T) {
	s := &server{}
	_, out, err := s.handleAuditState(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handleAuditState: %v", err)
	}

	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal audit_state: %v", err)
	}
	var roundTrip map[string]any
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatalf("unmarshal audit_state: %v", err)
	}
	if roundTrip["doc_version"] != "v1.1c" {
		t.Errorf("round-trip doc_version = %v, want v1.1c", roundTrip["doc_version"])
	}
}
