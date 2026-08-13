package repository

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type fakePGPool struct {
	execFunc      func(context.Context, string, ...any) (pgconn.CommandTag, error)
	queryFunc     func(context.Context, string, ...any) (pgx.Rows, error)
	queryRowFunc  func(context.Context, string, ...any) pgx.Row
	sendBatchFunc func(context.Context, *pgx.Batch) pgx.BatchResults
	beginFunc     func(context.Context) (pgx.Tx, error)
}

func (f *fakePGPool) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if f.execFunc != nil {
		return f.execFunc(ctx, sql, args...)
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (f *fakePGPool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if f.queryFunc != nil {
		return f.queryFunc(ctx, sql, args...)
	}
	return &fakeRows{}, nil
}

func (f *fakePGPool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if f.queryRowFunc != nil {
		return f.queryRowFunc(ctx, sql, args...)
	}
	return fakeRow{}
}

func (f *fakePGPool) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	if f.sendBatchFunc != nil {
		return f.sendBatchFunc(ctx, b)
	}
	return &fakeBatchResults{tag: pgconn.NewCommandTag("INSERT 0 1")}
}

func (f *fakePGPool) Begin(ctx context.Context) (pgx.Tx, error) {
	if f.beginFunc != nil {
		return f.beginFunc(ctx)
	}
	return &fakeTx{}, nil
}

type fakeRows struct {
	rows [][]any
	idx  int
	err  error
}

func (r *fakeRows) Close()                                       {}
func (r *fakeRows) Err() error                                   { return r.err }
func (r *fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("SELECT 1") }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeRows) RawValues() [][]byte                          { return nil }
func (r *fakeRows) Conn() *pgx.Conn                              { return nil }

func (r *fakeRows) Next() bool {
	if r.idx >= len(r.rows) {
		return false
	}
	r.idx++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx > len(r.rows) {
		return errors.New("scan without current row")
	}
	return scanValues(r.rows[r.idx-1], dest...)
}

func (r *fakeRows) Values() ([]any, error) {
	if r.idx == 0 || r.idx > len(r.rows) {
		return nil, errors.New("values without current row")
	}
	return r.rows[r.idx-1], nil
}

type fakeRow struct {
	values []any
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return scanValues(r.values, dest...)
}

func scanValues(values []any, dest ...any) error {
	if len(values) != len(dest) {
		return errors.New("scan arity mismatch")
	}
	for i := range dest {
		if dest[i] == nil {
			continue
		}
		dv := reflect.ValueOf(dest[i])
		if dv.Kind() != reflect.Pointer || dv.IsNil() {
			return errors.New("scan destination must be pointer")
		}
		if values[i] == nil {
			dv.Elem().Set(reflect.Zero(dv.Elem().Type()))
			continue
		}
		vv := reflect.ValueOf(values[i])
		if vv.Type().AssignableTo(dv.Elem().Type()) {
			dv.Elem().Set(vv)
			continue
		}
		if vv.Type().ConvertibleTo(dv.Elem().Type()) {
			dv.Elem().Set(vv.Convert(dv.Elem().Type()))
			continue
		}
		return errors.New("scan type mismatch")
	}
	return nil
}

type fakeBatchResults struct {
	tag pgconn.CommandTag
	err error
}

func (b *fakeBatchResults) Exec() (pgconn.CommandTag, error) { return b.tag, b.err }
func (b *fakeBatchResults) Query() (pgx.Rows, error)         { return &fakeRows{}, b.err }
func (b *fakeBatchResults) QueryRow() pgx.Row                { return fakeRow{err: b.err} }
func (b *fakeBatchResults) Close() error                     { return nil }

type fakeTx struct {
	execSQL []string
	err     error
}

func (tx *fakeTx) Begin(context.Context) (pgx.Tx, error) { return tx, nil }
func (tx *fakeTx) Commit(context.Context) error          { return tx.err }
func (tx *fakeTx) Rollback(context.Context) error        { return nil }
func (tx *fakeTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("not implemented")
}

func (tx *fakeTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return &fakeBatchResults{} }

func (tx *fakeTx) LargeObjects() pgx.LargeObjects { return pgx.LargeObjects{} }

func (tx *fakeTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("not implemented")
}

func (tx *fakeTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	tx.execSQL = append(tx.execSQL, sql)
	return pgconn.NewCommandTag("INSERT 0 1"), tx.err
}

func (tx *fakeTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return &fakeRows{}, tx.err
}
func (tx *fakeTx) QueryRow(context.Context, string, ...any) pgx.Row { return fakeRow{err: tx.err} }
func (tx *fakeTx) Conn() *pgx.Conn                                  { return nil }

func newUnitPostgresRepo(pool *fakePGPool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func TestPostgresRepository_MetricsCRUDWithFakePool(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	execCalls := 0
	pool := &fakePGPool{
		execFunc: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			execCalls++
			if !strings.Contains(sql, "INSERT INTO metrics") {
				t.Fatalf("unexpected exec SQL: %s", sql)
			}
			if args[0] != "screening_total" || args[1] != 42.0 {
				t.Fatalf("unexpected metric args: %#v", args)
			}
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
		queryFunc: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			if !strings.Contains(sql, "FROM metrics") {
				t.Fatalf("unexpected query SQL: %s", sql)
			}
			return &fakeRows{rows: [][]any{{now, "screening_total", 42.0, "agent", "session", "2330", "bull", []byte(`{"agent_id":"agent"}`)}}}, nil
		},
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if !strings.Contains(sql, "ORDER BY time DESC LIMIT 1") && !strings.Contains(sql, "AVG(value)") {
				t.Fatalf("unexpected query row SQL: %s", sql)
			}
			if strings.Contains(sql, "AVG(value)") {
				return fakeRow{values: []any{21.0}}
			}
			return fakeRow{values: []any{now, "screening_total", 42.0, "agent", "session", "2330", "bull", []byte(`{"symbol":"2330"}`)}}
		},
	}
	repo := newUnitPostgresRepo(pool)

	if err := repo.Record(ctx, "screening_total", 42, map[string]string{"agent_id": "agent", "session_id": "session", "symbol": "2330", "regime": "bull"}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	points, err := repo.QueryRange(ctx, "screening_total", now.Add(-time.Hour), now)
	if err != nil || len(points) != 1 || points[0].Name != "screening_total" {
		t.Fatalf("QueryRange() = %#v, %v", points, err)
	}
	latest, err := repo.QueryLatest(ctx, "screening_total", map[string]string{"agent_id": "agent", "symbol": "2330"})
	if err != nil || latest == nil || latest.Value != 42 {
		t.Fatalf("QueryLatest() = %#v, %v", latest, err)
	}
	agg, err := repo.Aggregate(ctx, "screening_total", now.Add(-time.Hour), now, "avg")
	if err != nil || agg != 21 {
		t.Fatalf("Aggregate() = %f, %v", agg, err)
	}
	if execCalls != 1 {
		t.Fatalf("exec calls = %d, want 1", execCalls)
	}
}

func TestPostgresRepository_SnapshotLoadersWithFakePool(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 14, 9, 30, 0, 0, time.UTC)
	queryCalls := 0
	pool := &fakePGPool{
		execFunc: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if !strings.Contains(sql, "INSERT INTO metrics") {
				t.Fatalf("unexpected snapshot exec SQL: %s", sql)
			}
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
		queryFunc: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			queryCalls++
			switch {
			case strings.Contains(sql, "DISTINCT time_bucket"):
				return &fakeRows{rows: [][]any{{now}}}, nil
			case strings.Contains(sql, "metric_name = $1"):
				return &fakeRows{rows: [][]any{{now, "screening_total", 10.0, "", "", "", "", []byte(`{}`)}}}, nil
			case strings.Contains(sql, "FROM metrics"):
				return &fakeRows{rows: [][]any{
					{now, "screening_total", 10.0, "", "", "", "", []byte(`{}`)},
					{now, "screening_passed", 7.0, "", "", "", "", []byte(`{}`)},
					{now, "screening_rate", 0.7, "", "", "", "", []byte(`{}`)},
					{now, "alerts_triggered", 2.0, "", "", "", "", []byte(`{}`)},
					{now, "alerts_acknowledged", 1.0, "", "", "", "", []byte(`{}`)},
				}}, nil
			default:
				t.Fatalf("unexpected snapshot query SQL: %s", sql)
				return nil, nil
			}
		},
	}
	repo := newUnitPostgresRepo(pool)

	if err := repo.SaveSnapshot(ctx, &MetricsSnapshot{ScreeningTotal: 10, ScreeningPassed: 7, ScreeningRate: 0.7, AlertsTriggered: 2, AlertsAcknowledged: 1, AlertsByType: map[string]int64{"warning": 2}}); err != nil {
		t.Fatalf("SaveSnapshot() error = %v", err)
	}
	today, err := repo.LoadToday(ctx)
	if err != nil || today == nil || today.ScreeningPassed != 7 || today.AlertsAcknowledged != 1 {
		t.Fatalf("LoadToday() = %#v, %v", today, err)
	}
	recent, err := repo.LoadRecent(ctx, 1)
	if err != nil || len(recent) != 1 || recent[0].ScreeningRate != 0.7 {
		t.Fatalf("LoadRecent() = %#v, %v", recent, err)
	}
	if queryCalls < 3 {
		t.Fatalf("query calls = %d, want at least 3", queryCalls)
	}
}

func TestPostgresRepository_AlertCRUDWithFakePool(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	alertRow := []any{"alert-1", now, "rule", "warning", "message", 1.0, 2.0, false, (*time.Time)(nil), "", "open", "dedup", 3, &now, &now, (*time.Time)(nil), "", (*time.Time)(nil)}
	pool := &fakePGPool{
		execFunc: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if !strings.Contains(sql, "alerts") {
				t.Fatalf("unexpected alert exec SQL: %s", sql)
			}
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
		queryFunc: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			if !strings.Contains(sql, "FROM alerts") {
				t.Fatalf("unexpected alert query SQL: %s", sql)
			}
			return &fakeRows{rows: [][]any{alertRow}}, nil
		},
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if !strings.Contains(sql, "FROM alerts") {
				t.Fatalf("unexpected alert row SQL: %s", sql)
			}
			return fakeRow{values: alertRow}
		},
	}
	repo := newUnitPostgresRepo(pool)

	if err := repo.SaveAlert(ctx, domain.AlertRecord{ID: "alert-1", Timestamp: now, Rule: "rule", Severity: "warning"}); err != nil {
		t.Fatalf("SaveAlert() error = %v", err)
	}
	alerts, err := repo.LoadAllAlerts(ctx, 10)
	if err != nil || len(alerts) != 1 || alerts[0].ID != "alert-1" {
		t.Fatalf("LoadAllAlerts() = %#v, %v", alerts, err)
	}
	if err := repo.AcknowledgeAlert(ctx, "alert-1", "operator"); err != nil {
		t.Fatalf("AcknowledgeAlert() error = %v", err)
	}
	found, err := repo.FindAlertByDedupKey(ctx, "dedup")
	if err != nil || found == nil || found.DedupKey != "dedup" {
		t.Fatalf("FindAlertByDedupKey() = %#v, %v", found, err)
	}
	if err := repo.UpdateAlertByID(ctx, "alert-1", func(a *domain.AlertRecord) { a.Message = "updated" }); err != nil {
		t.Fatalf("UpdateAlertByID() error = %v", err)
	}
	if _, err := repo.LoadUnacknowledgedAlerts(ctx); err != nil {
		t.Fatalf("LoadUnacknowledgedAlerts() error = %v", err)
	}
	if err := repo.ResolveAlert(ctx, "alert-1", "operator"); err != nil {
		t.Fatalf("ResolveAlert() error = %v", err)
	}
	if _, err := repo.LoadAlertsBySeverity(ctx, "warning", 5); err != nil {
		t.Fatalf("LoadAlertsBySeverity() error = %v", err)
	}
	if _, err := repo.LoadAlertsByTimeRange(ctx, now.Add(-time.Hour), now); err != nil {
		t.Fatalf("LoadAlertsByTimeRange() error = %v", err)
	}
}

func TestPostgresRepository_OutcomesAuditAndOtherCRUDWithFakePool(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 14, 11, 0, 0, 0, time.UTC)
	outcomeRow := []any{now, "session-1", "2330", "agent", "sector", 80, true, "", 100.5, []byte(`{"window":"session-1","symbol":"2330","agent_id":"agent","conviction":80}`)}
	rejectRow := []any{now, "session-1", "2330", "agent", "skill", "criterion", "label", "0.7", "0.5", []byte(`{"total":0.5}`)}
	summaryRow := []any{now, "session-1", "RISK_ON", 2, 1, 900000.0, 1000000.0, 3, []byte(`{}`), "agent-next", "proposal", "commit", "approval", []byte(`[]`), "test-risk-commentary", []byte(`null`), 0.0, 0.0, ""}
	interventionRow := []any{now, "hi-1", "pause_agent", "agent", "model", "sector", "2330", 0.5, "reason", "operator", "session-1"}
	pool := &fakePGPool{
		queryFunc: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			switch {
			case strings.Contains(sql, "recommendation_outcomes") && strings.Contains(sql, "COUNT(*) as count"):
				return &fakeRows{rows: [][]any{{"2330", 2}}}, nil
			case strings.Contains(sql, "recommendation_outcomes") && strings.Contains(sql, "GROUP BY session_id"):
				return &fakeRows{rows: [][]any{{"session-1", now, 2}}}, nil
			case strings.Contains(sql, "recommendation_outcomes"):
				return &fakeRows{rows: [][]any{outcomeRow}}, nil
			case strings.Contains(sql, "screening_rejects"):
				return &fakeRows{rows: [][]any{rejectRow}}, nil
			case strings.Contains(sql, "session_summaries"):
				return &fakeRows{rows: [][]any{summaryRow}}, nil
			case strings.Contains(sql, "human_interventions"):
				return &fakeRows{rows: [][]any{interventionRow}}, nil
			case strings.Contains(sql, "capital_flow"):
				return &fakeRows{rows: [][]any{{now, "foreign", 1.0, 2.0, 1.0}}}, nil
			default:
				t.Fatalf("unexpected query SQL: %s", sql)
				return nil, nil
			}
		},
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "COUNT(*) as total"):
				return fakeRow{values: []any{int64(3), int64(2)}}
			case strings.Contains(sql, "session_summaries"):
				return fakeRow{values: summaryRow}
			case strings.Contains(sql, "capital_flow"):
				return fakeRow{values: []any{now, "foreign", 1.0, 2.0, 1.0}}
			case strings.Contains(sql, "export_statistics"):
				return fakeRow{values: []any{now, 115, 5, 10.0, 9.0, 1.0}}
			default:
				t.Fatalf("unexpected row SQL: %s", sql)
				return fakeRow{}
			}
		},
	}
	repo := newUnitPostgresRepo(pool)

	if err := repo.RecordOutcomes(ctx, []domain.RecommendationOutcome{{Window: "session-1", Symbol: "2330", AgentID: "agent", Conviction: 80}}); err != nil {
		t.Fatalf("RecordOutcomes() error = %v", err)
	}
	outcomes, err := repo.QueryOutcomesBySession(ctx, "session-1")
	if err != nil || len(outcomes) != 1 || outcomes[0].Window != "session-1" {
		t.Fatalf("QueryOutcomesBySession() = %#v, %v", outcomes, err)
	}
	if _, err := repo.QueryOutcomesBySymbol(ctx, "2330", now.Add(-time.Hour), now); err != nil {
		t.Fatalf("QueryOutcomesBySymbol() error = %v", err)
	}
	if _, err := repo.QueryOutcomesByAgent(ctx, "agent", now.Add(-time.Hour), now); err != nil {
		t.Fatalf("QueryOutcomesByAgent() error = %v", err)
	}
	if _, err := repo.QueryAllOutcomes(ctx); err != nil {
		t.Fatalf("QueryAllOutcomes() error = %v", err)
	}
	passRate, err := repo.QueryPassRate(ctx, "agent", time.Hour)
	if err != nil || passRate != 2.0/3.0 {
		t.Fatalf("QueryPassRate() = %f, %v", passRate, err)
	}
	top, err := repo.QueryTopSymbols(ctx, 5, now.Add(-time.Hour), now)
	if err != nil || len(top) != 1 || top[0].Count != 2 {
		t.Fatalf("QueryTopSymbols() = %#v, %v", top, err)
	}
	sessions, err := repo.QuerySessions(ctx)
	if err != nil || len(sessions) != 1 || sessions[0].SessionID != "session-1" {
		t.Fatalf("QuerySessions() = %#v, %v", sessions, err)
	}
	if err := repo.RecordScreeningRejects(ctx, "session-1", []domain.ScreeningReject{{SessionID: "session-1", Symbol: "2330", RecordedAt: now}}); err != nil {
		t.Fatalf("RecordScreeningRejects() error = %v", err)
	}
	rejects, err := repo.QueryScreeningRejectsBySession(ctx, "session-1")
	if err != nil || len(rejects) != 1 {
		t.Fatalf("QueryScreeningRejectsBySession() = %#v, %v", rejects, err)
	}
	summary, err := repo.LoadSessionSummary(ctx, "session-1")
	if err != nil || summary == nil || summary.SessionID != "session-1" {
		t.Fatalf("LoadSessionSummary() = %#v, %v", summary, err)
	}
	if err := repo.SaveSessionSummary(ctx, *summary); err != nil {
		t.Fatalf("SaveSessionSummary() error = %v", err)
	}
	allSummaries, err := repo.LoadAllSessionSummaries(ctx)
	if err != nil || len(allSummaries) != 1 {
		t.Fatalf("LoadAllSessionSummaries() = %#v, %v", allSummaries, err)
	}
	if err := repo.RecordHumanIntervention(ctx, domain.HumanIntervention{ID: "hi-1", RecordedAt: now}); err != nil {
		t.Fatalf("RecordHumanIntervention() error = %v", err)
	}
	interventions, err := repo.LoadHumanInterventions(ctx)
	if err != nil || len(interventions) != 1 || interventions[0].ID != "hi-1" {
		t.Fatalf("LoadHumanInterventions() = %#v, %v", interventions, err)
	}
	flow, err := repo.QueryLatestCapitalFlow(ctx, "foreign")
	if err != nil || flow == nil || flow.Channel != "foreign" {
		t.Fatalf("QueryLatestCapitalFlow() = %#v, %v", flow, err)
	}
	if err := repo.RecordCapitalFlow(ctx, "foreign", 1, 2, 1); err != nil {
		t.Fatalf("RecordCapitalFlow() error = %v", err)
	}
	if flows, err := repo.QueryCapitalFlowRange(ctx, "foreign", now.Add(-time.Hour), now); err != nil || len(flows) != 1 {
		t.Fatalf("QueryCapitalFlowRange() = %#v, %v", flows, err)
	}
	exportStats, err := repo.QueryLatestExportStats(ctx)
	if err != nil || exportStats == nil || exportStats.Year != 115 {
		t.Fatalf("QueryLatestExportStats() = %#v, %v", exportStats, err)
	}
	if exportStats, err := repo.QueryExportStatsByYearMonth(ctx, 115, 5); err != nil || exportStats == nil || exportStats.Month != 5 {
		t.Fatalf("QueryExportStatsByYearMonth() = %#v, %v", exportStats, err)
	}
	if _, _, err := repo.QueryAllSessionScorecards(ctx); err != nil {
		t.Fatalf("QueryAllSessionScorecards() error = %v", err)
	}
}

func TestPostgresRepository_SessionSummary_TaxAndParamsFields(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	taxSnaps := []domain.TaxSnapshot{
		{Symbol: "2330", DividendTaxRate: 0.28, TransactionTaxRate: 0.003, DividendTax: 560.0, TransactionTax: 90.0, TotalTax: 650.0, AfterTaxPnL: 12345.67},
	}
	wantAfterTax := 12345.67
	wantTotalTax := 650.0
	wantParamsVer := "0.0.0.11"
	taxJSON, _ := json.Marshal(taxSnaps)

	summaryRow19 := []any{
		now, "session-tax-1", "RISK_ON", 2, 1, 900000.0, 1000000.0, 3,
		[]byte(`{}`), "agent-next", "proposal", "commit", "approval", []byte(`[]`),
		"", taxJSON, wantAfterTax, wantTotalTax, wantParamsVer,
	}

	var capturedExecArgs []any
	pool := &fakePGPool{
		execFunc: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "INSERT INTO session_summaries") {
				capturedExecArgs = args
			}
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
		queryFunc: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			if strings.Contains(sql, "session_summaries") {
				return &fakeRows{rows: [][]any{summaryRow19}}, nil
			}
			return &fakeRows{}, nil
		},
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "session_summaries") {
				return fakeRow{values: summaryRow19}
			}
			return fakeRow{}
		},
	}
	repo := newUnitPostgresRepo(pool)

	in := domain.SessionSummary{
		SessionID:             "session-tax-1",
		Regime:                domain.RegimeRiskOn,
		OrderCount:            2,
		PositionCount:         1,
		EndingCash:            900000.0,
		PortfolioValue:        1000000.0,
		OutcomeCount:          3,
		NextExperimentAgentID: "agent-next",
		ProposalID:            "proposal",
		CommitID:              "commit",
		ApprovalID:            "approval",
		GuardOutcomes:         []domain.GuardOutcome{},
		RecordedAt:            now,
		TaxSnapshots:          taxSnaps,
		AfterTaxPnL:           wantAfterTax,
		TotalTaxPaid:          wantTotalTax,
		ParametersVersion:     wantParamsVer,
	}
	if err := repo.SaveSessionSummary(ctx, in); err != nil {
		t.Fatalf("SaveSessionSummary() error = %v", err)
	}
	if len(capturedExecArgs) != 19 {
		t.Fatalf("SaveSessionSummary() exec args = %d, want 19", len(capturedExecArgs))
	}
	if got, _ := capturedExecArgs[14].(string); got != "" {
		t.Errorf("SaveSessionSummary arg[14] (risk_commentary) = %v, want \"\"", got)
	}
	if got, ok := capturedExecArgs[15].([]byte); !ok || string(got) != string(taxJSON) {
		t.Errorf("SaveSessionSummary arg[15] (tax_snapshots) = %v, want %s", capturedExecArgs[15], taxJSON)
	}
	if got, _ := capturedExecArgs[16].(float64); got != wantAfterTax {
		t.Errorf("SaveSessionSummary arg[16] (after_tax_pnl) = %v, want %v", got, wantAfterTax)
	}
	if got, _ := capturedExecArgs[17].(float64); got != wantTotalTax {
		t.Errorf("SaveSessionSummary arg[17] (total_tax_paid) = %v, want %v", got, wantTotalTax)
	}
	if got, _ := capturedExecArgs[18].(string); got != wantParamsVer {
		t.Errorf("SaveSessionSummary arg[18] (parameters_version) = %v, want %v", got, wantParamsVer)
	}

	loaded, err := repo.LoadSessionSummary(ctx, "session-tax-1")
	if err != nil {
		t.Fatalf("LoadSessionSummary() error = %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadSessionSummary() returned nil")
	}
	if len(loaded.TaxSnapshots) != 1 {
		t.Fatalf("TaxSnapshots len = %d, want 1", len(loaded.TaxSnapshots))
	}
	if loaded.TaxSnapshots[0].Symbol != "2330" || loaded.TaxSnapshots[0].TotalTax != 650.0 {
		t.Errorf("TaxSnapshots[0] = %+v, want Symbol=2330 TotalTax=650", loaded.TaxSnapshots[0])
	}
	if loaded.AfterTaxPnL != wantAfterTax {
		t.Errorf("AfterTaxPnL = %v, want %v", loaded.AfterTaxPnL, wantAfterTax)
	}
	if loaded.TotalTaxPaid != wantTotalTax {
		t.Errorf("TotalTaxPaid = %v, want %v", loaded.TotalTaxPaid, wantTotalTax)
	}
	if loaded.ParametersVersion != wantParamsVer {
		t.Errorf("ParametersVersion = %q, want %q", loaded.ParametersVersion, wantParamsVer)
	}

	all, err := repo.LoadAllSessionSummaries(ctx)
	if err != nil {
		t.Fatalf("LoadAllSessionSummaries() error = %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("LoadAllSessionSummaries() len = %d, want 1", len(all))
	}
	if len(all[0].TaxSnapshots) != 1 || all[0].AfterTaxPnL != wantAfterTax || all[0].TotalTaxPaid != wantTotalTax || all[0].ParametersVersion != wantParamsVer {
		t.Errorf("LoadAllSessionSummaries[0] tax/params fields mismatch: %+v", all[0])
	}
}

func TestTaskExecutionStoreCRUDWithFakePool(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	execRow := []any{"exec-1", domain.TaskTypeRunExperiment, "run", []byte(`["--brief","x"]`), []byte(`{"brief":"x"}`), domain.TaskStatusQueued, "user", "api", "idem", "", "parent", "exp-1", "result.json", (*int)(nil), (*int)(nil), false, (*time.Time)(nil), (*time.Time)(nil), now, (*time.Time)(nil), (*time.Time)(nil), (*int)(nil), "", []byte(`{}`), now, now}
	eventRow := []any{"exec-1", int64(1), domain.TaskEventProgress, "stdout", "info", "started", []byte(`{}`), now}
	lineageRow := []any{"exp-1", "exec-1", "", "exp-1", 0, "agent", "skill", "param", "brief", "candidate", "result", "judged", "commit", []byte(`{}`), []byte(`{}`), (*float64)(nil), (*float64)(nil), (*float64)(nil), now, (*time.Time)(nil)}
	baselineRow := []any{"bh-1", "exec-1", "exp-1", 1, 2, "operator", now, "baseline.json", []byte(`{}`), "diff", []byte(`{}`)}
	metricRow := []any{"mt-1", "exec-1", "exp-1", "series", "sharpe", "agent", 1.2, (*float64)(nil), (*float64)(nil), now, []byte(`{"tag":"x"}`)}
	pool := &fakePGPool{
		execFunc: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if !strings.Contains(sql, "task_executions") && !strings.Contains(sql, "task_execution_events") && !strings.Contains(sql, "experiment_lineage") && !strings.Contains(sql, "baseline_history") {
				t.Fatalf("unexpected task exec SQL: %s", sql)
			}
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
		queryFunc: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			switch {
			case strings.Contains(sql, "task_executions"):
				return &fakeRows{rows: [][]any{execRow}}, nil
			case strings.Contains(sql, "task_execution_events"):
				return &fakeRows{rows: [][]any{eventRow}}, nil
			case strings.Contains(sql, "experiment_lineage"):
				return &fakeRows{rows: [][]any{lineageRow}}, nil
			case strings.Contains(sql, "baseline_history"):
				return &fakeRows{rows: [][]any{baselineRow}}, nil
			case strings.Contains(sql, "metric_trends"):
				return &fakeRows{rows: [][]any{metricRow}}, nil
			default:
				t.Fatalf("unexpected task query SQL: %s", sql)
				return nil, nil
			}
		},
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "task_executions") {
				return fakeRow{values: execRow}
			}
			if strings.Contains(sql, "experiment_lineage") {
				return fakeRow{values: lineageRow}
			}
			t.Fatalf("unexpected task row SQL: %s", sql)
			return fakeRow{}
		},
	}
	store := NewTaskExecutionStore(newUnitPostgresRepo(pool))
	exec := domain.TaskExecution{ID: "exec-1", TaskType: domain.TaskTypeRunExperiment, CommandName: "run", CommandArgs: []string{"--brief", "x"}, Status: domain.TaskStatusQueued, Actor: "user", ActorSource: "api", SubmittedAt: now}

	if err := store.CreateExecution(ctx, exec); err != nil {
		t.Fatalf("CreateExecution() error = %v", err)
	}
	if err := store.UpdateExecution(ctx, exec); err != nil {
		t.Fatalf("UpdateExecution() error = %v", err)
	}
	gotExec, err := store.GetExecution(ctx, "exec-1")
	if err != nil || gotExec == nil || len(gotExec.CommandArgs) != 2 {
		t.Fatalf("GetExecution() = %#v, %v", gotExec, err)
	}
	listed, err := store.ListExecutions(ctx, domain.ExecutionFilter{TaskType: string(domain.TaskTypeRunExperiment), Status: string(domain.TaskStatusQueued), ExperimentID: "exp-1", Actor: "user", Since: &now, Limit: 5})
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListExecutions() = %#v, %v", listed, err)
	}
	if err := store.AppendEvent(ctx, domain.TaskExecutionEvent{ExecutionID: "exec-1", Sequence: 1, EventType: domain.TaskEventProgress, CreatedAt: now}); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}
	events, err := store.ListEventsAfter(ctx, "exec-1", 0)
	if err != nil || len(events) != 1 || events[0].Sequence != 1 {
		t.Fatalf("ListEventsAfter() = %#v, %v", events, err)
	}
	lineage := domain.ExperimentLineageRecord{ExperimentID: "exp-1", ExecutionID: "exec-1", RootExperimentID: "exp-1", RecordedAt: now}
	if err := store.UpsertLineage(ctx, lineage); err != nil {
		t.Fatalf("UpsertLineage() error = %v", err)
	}
	gotLineage, err := store.GetLineage(ctx, "exp-1")
	if err != nil || gotLineage == nil || gotLineage.ExperimentID != "exp-1" {
		t.Fatalf("GetLineage() = %#v, %v", gotLineage, err)
	}
	children, err := store.GetLineageChildren(ctx, "exp-1")
	if err != nil || len(children) != 1 {
		t.Fatalf("GetLineageChildren() = %#v, %v", children, err)
	}
	if err := store.InsertBaselineHistory(ctx, domain.BaselineHistoryRecord{ExecutionID: "exec-1", ExperimentID: "exp-1", VersionBefore: 1, VersionAfter: 2, PromotedAt: now}); err != nil {
		t.Fatalf("InsertBaselineHistory() error = %v", err)
	}
	history, err := store.ListBaselineHistory(ctx, 10)
	if err != nil || len(history) != 1 || history[0].ID != "bh-1" {
		t.Fatalf("ListBaselineHistory() = %#v, %v", history, err)
	}
	if err := store.InsertMetricPoints(ctx, []domain.MetricTrendPoint{{ExecutionID: "exec-1", SeriesKey: "series", MetricName: "sharpe", SampledAt: now}}); err != nil {
		t.Fatalf("InsertMetricPoints() error = %v", err)
	}
	trends, err := store.QueryMetricTrends(ctx, domain.MetricTrendFilter{ExperimentID: "exp-1", SeriesKey: "series", MetricName: "sharpe", Start: now.Add(-time.Hour), End: now, Limit: 5})
	if err != nil || len(trends) != 1 || trends[0].MetricName != "sharpe" {
		t.Fatalf("QueryMetricTrends() = %#v, %v", trends, err)
	}
}

func TestPostgresRepository_SaveExportStatsUsesTransaction(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTx{}
	pool := &fakePGPool{beginFunc: func(context.Context) (pgx.Tx, error) { return tx, nil }}
	repo := newUnitPostgresRepo(pool)

	if err := repo.SaveExportStats(ctx, 115, 6, 10, 9, 1); err != nil {
		t.Fatalf("SaveExportStats() error = %v", err)
	}
	if len(tx.execSQL) != 2 || !strings.Contains(tx.execSQL[0], "DELETE FROM export_statistics") || !strings.Contains(tx.execSQL[1], "INSERT INTO export_statistics") {
		t.Fatalf("transaction SQL = %#v", tx.execSQL)
	}
}

func TestDualWriteRepository_JSONLFallbacksCoverCRUDWrappers(t *testing.T) {
	ctx := context.Background()
	alerts := &mockAlertStore{}
	metrics := &mockMetricsStore{}
	outcomes := &mockOutcomeStore{}
	rejects := &recordingScreeningRejectStore{}
	summaries := &mockSessionSummaryStore{}
	human := &recordingHumanInterventionStore{}
	repo := &DualWriteRepository{jsonl: &JSONLRepository{
		alertStore:             alerts,
		metricsStore:           metrics,
		outcomeStore:           outcomes,
		screeningRejectStore:   rejects,
		sessionSummaryStore:    summaries,
		humanInterventionStore: human,
	}}

	if err := repo.SaveAlert(ctx, domain.AlertRecord{ID: "a1", Severity: "warning"}); err != nil {
		t.Fatalf("SaveAlert() error = %v", err)
	}
	if got, _ := repo.LoadAllAlerts(ctx, 0); len(got) != 1 {
		t.Fatalf("LoadAllAlerts fallback len = %d", len(got))
	}
	if err := repo.RecordOutcomes(ctx, []domain.RecommendationOutcome{{Window: "s1", Symbol: "2330"}}); err != nil {
		t.Fatalf("RecordOutcomes() error = %v", err)
	}
	if got, _ := repo.QueryOutcomesBySession(ctx, "s1"); len(got) != 1 {
		t.Fatalf("QueryOutcomesBySession fallback len = %d", len(got))
	}
	if err := repo.SaveSnapshot(ctx, &MetricsSnapshot{ScreeningTotal: 1}); err != nil {
		t.Fatalf("SaveSnapshot() error = %v", err)
	}
	if got, _ := repo.LoadToday(ctx); got == nil || got.ScreeningTotal != 1 {
		t.Fatalf("LoadToday fallback = %#v", got)
	}
	if err := repo.RecordScreeningRejects(ctx, "s1", []domain.ScreeningReject{{SessionID: "s1", Symbol: "2330"}}); err != nil {
		t.Fatalf("RecordScreeningRejects() error = %v", err)
	}
	if got, _ := repo.QueryScreeningRejectsBySession(ctx, "s1"); len(got) != 1 {
		t.Fatalf("QueryScreeningRejectsBySession fallback len = %d", len(got))
	}
	if err := repo.RecordHumanIntervention(ctx, domain.HumanIntervention{ID: "h1"}); err != nil {
		t.Fatalf("RecordHumanIntervention() error = %v", err)
	}
	if got, _ := repo.LoadHumanInterventions(ctx); len(got) != 1 {
		t.Fatalf("LoadHumanInterventions fallback len = %d", len(got))
	}
}

type recordingScreeningRejectStore struct{ rejects []domain.ScreeningReject }

func (s *recordingScreeningRejectStore) RecordSessionScreeningRejects(_ string, rejects []domain.ScreeningReject) error {
	s.rejects = append(s.rejects, rejects...)
	return nil
}

func (s *recordingScreeningRejectStore) LoadSessionScreeningRejects(string) ([]domain.ScreeningReject, error) {
	return s.rejects, nil
}

// TestDualWriteRepository_PGUsableHealthProbe covers BL-06/Step2: pgUsable()
// must perform a live SELECT 1 probe (not just check pool != nil), so a dead
// PostgreSQL is surfaced instead of every dual-write call site silently
// swallowing the failure.
func TestDualWriteRepository_PGUsableHealthProbe(t *testing.T) {
	// Healthy: SELECT 1 succeeds → pgUsable true.
	healthy := &PostgresRepository{pool: &fakePGPool{
		queryRowFunc: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return fakeRow{values: []any{int32(1)}}
		},
	}}
	repoHealthy := &DualWriteRepository{pg: healthy}
	if !repoHealthy.pgUsable() {
		t.Errorf("expected pgUsable true for healthy PG")
	}
	// TTL cache: second call within TTL returns cached true without another probe.
	if !repoHealthy.pgUsable() {
		t.Errorf("expected cached pgUsable true")
	}

	// Dead: SELECT 1 fails → pgUsable false.
	dead := &PostgresRepository{pool: &fakePGPool{
		queryRowFunc: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return fakeRow{err: errors.New("timescaledb extension missing")}
		},
	}}
	repoDead := &DualWriteRepository{pg: dead}
	if repoDead.pgUsable() {
		t.Errorf("expected pgUsable false for dead PG")
	}

	// Nil pool → false.
	repoNil := &DualWriteRepository{pg: &PostgresRepository{pool: nil}}
	if repoNil.pgUsable() {
		t.Errorf("expected pgUsable false for nil pool")
	}
}

type recordingHumanInterventionStore struct{ interventions []domain.HumanIntervention }

func (s *recordingHumanInterventionStore) RecordHumanIntervention(intervention domain.HumanIntervention) error {
	s.interventions = append(s.interventions, intervention)
	return nil
}

func (s *recordingHumanInterventionStore) LoadHumanInterventions() ([]domain.HumanIntervention, error) {
	return s.interventions, nil
}
