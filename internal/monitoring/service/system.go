package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/autobacktest"
	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/buildinfo"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// SystemService encapsulates system-level health and status monitoring.
type SystemService struct {
	WorkDir         string
	LedgerDir       string
	BaselinePath    string
	store           ledger.OutcomeStore
	healthStore     *apigateway.ChannelHealthStore
	historicalStore ledger.HistoricalStore
	JanusEngine     *janus.Engine
	CycleTracker    *industry.CycleTracker
}

// NewSystemService creates a new SystemService.
// healthStore is used to merge Gateway-managed channel health into the system
// health snapshot so the home page and the data channels page agree on
// channel status (see resolveChannelStatusFromStore).
func NewSystemService(workDir, ledgerDir, baselinePath string, store ledger.OutcomeStore, janusEngine *janus.Engine, healthStore *apigateway.ChannelHealthStore) *SystemService {
	return &SystemService{
		WorkDir:      workDir,
		LedgerDir:    ledgerDir,
		BaselinePath: baselinePath,
		store:        store,
		healthStore:  healthStore,
		JanusEngine:  janusEngine,
	}
}

// SetHistoricalStore wires the authoritative regime_history store so
// LoadSystemHealth can surface the same regime as /api/regime/history
// (macro_ingest 收盤權威值) instead of the session-execution-time regime.
func (s *SystemService) SetHistoricalStore(hs ledger.HistoricalStore) {
	s.historicalStore = hs
}

// LoadPhase3Status loads Phase 3 metrics from the well-known path.
func (s *SystemService) LoadPhase3Status() (orchestrator.Phase3Metrics, error) {
	return orchestrator.LoadPhase3Metrics("")
}

// SystemHealthResponse mirrors the dashboard API response structure.
//
// Runtime surfaces the linker-injected buildinfo (version / commit /
// build_time / go_version) so dashboards and the atlas-mcp tools can audit a
// deployed binary against the source commit (CF-INV-12,
// docs/specs/capital-flow-seven-dimension-spec.md §11.4). It is a pointer so
// callers can distinguish "block omitted" from "block present with all-empty
// fields"; LoadSystemHealth always populates it.
type SystemHealthResponse struct {
	BaselineVersion       string            `json:"baseline_version"`
	ReplayDataLatestDate  string            `json:"replay_data_latest_date"`
	ReplayDataPathOK      bool              `json:"replay_data_path_ok"`
	LastWindowID          string            `json:"last_window_id"`
	LastWindowGeneratedAt time.Time         `json:"last_window_generated_at"`
	Warnings              []string          `json:"warnings"`
	Regime                domain.Regime     `json:"regime"`
	RegimeSource          string            `json:"regime_source"`
	DataChannels          []DataChannelInfo `json:"data_channels,omitempty"`
	DegradedChannels      []string          `json:"degraded_channels,omitempty"`
	CycleStale            bool              `json:"cycle_stale"`
	CycleStaleCount       int               `json:"cycle_stale_count,omitempty"`
	CycleTrackedTotal     int               `json:"cycle_tracked_total,omitempty"`
	BacktestStale         bool              `json:"backtest_stale,omitempty"`
	Runtime               *buildinfo.Info   `json:"runtime,omitempty"`
}

// DataChannelInfo represents a single data channel status.
type DataChannelInfo struct {
	ChannelID  string `json:"channel_id"`
	Label      string `json:"label"`
	Status     string `json:"status"`
	StatusText string `json:"status_text"`
	UpdatedAt  string `json:"updated_at"`
}

// resolveRegime returns the authoritative market regime plus its provenance.
// The canonical source is the regime_history table (macro_ingest 收盤權威值),
// which is the same source served by /api/regime/history — keeping
// system-health and /api/regime/history consistent for consumers (frontend /
// hermes briefing). When regime_history is unavailable or empty we fall back
// to the latest session summary's regime (session 執行當下判定) and label the
// source accordingly.
func (s *SystemService) resolveRegime() (domain.Regime, string) {
	if s.historicalStore != nil {
		rows, err := s.historicalStore.LoadRegimeHistory(context.Background(), 1)
		if err == nil && len(rows) > 0 && rows[0].Regime != "" {
			return domain.Regime(rows[0].Regime), "regime_history"
		}
	}
	regime := domain.RegimeNeutral
	if summary, err := FindLatestSessionSummary(s.store, s.LedgerDir); err == nil && summary != nil {
		regime = summary.Regime
	}
	return regime, "session_summary"
}

// LoadSystemHealth computes system health from baseline, replay, ledger, and session data.
func (s *SystemService) LoadSystemHealth() (SystemHealthResponse, error) {
	warnings := make([]string, 0)

	policy, err := baseline.Load(s.BaselinePath)
	baselineVersion := "未知"
	if err != nil {
		warnings = append(warnings, "基線策略未載入")
	} else {
		baselineVersion = fmt.Sprintf("v%d", policy.Version)
	}

	replayPath := config.GetReplayDataPath(s.WorkDir)
	replayOK := true
	latestReplayDate, err := replay.GetLatestDate(replayPath)
	if err != nil {
		replayOK = false
		warnings = append(warnings, "replay 資料無法讀取："+err.Error())
	}

	// Backtest staleness: warn when the latest auto-backtest snapshot is
	// missing or lags the replay data by more than ~2 trading days
	// (fix manifest #B03 — previously silent via snapshot_exists_skip).
	backtestStale := false
	if replayOK && latestReplayDate != "" {
		snaps, snapErr := autobacktest.NewHistory(s.LedgerDir).LatestN(1)
		if snapErr != nil || len(snaps) == 0 {
			warnings = append(warnings, "自動回測尚無快照（replay 最新 "+latestReplayDate+"）")
			backtestStale = true
		} else if replayDate, parseErr := time.Parse("2006-01-02", latestReplayDate); parseErr == nil && replayDate.Sub(snaps[0].Date) > 3*24*time.Hour {
			warnings = append(warnings, fmt.Sprintf("自動回測快照過舊：最新 %s（replay 已達 %s）", snaps[0].Date.Format("2006-01-02"), latestReplayDate))
			backtestStale = true
		}
	}

	lastWindow := ""
	var lastWindowTime time.Time
	windowsDir := filepath.Join(s.LedgerDir, "windows")
	if entries, err := os.ReadDir(windowsDir); err == nil {
		var latest time.Time
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.Contains(e.Name(), "mutation-brief") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(latest) {
				latest = info.ModTime()
				lastWindow = strings.TrimSuffix(e.Name(), ".json")
				lastWindowTime = info.ModTime()
			}
		}
	}

	if baselineVersion != "未知" && lastWindowTime.IsZero() {
		warnings = append(warnings, "找不到回測窗口摘要")
	}

	// Crowding check from latest session outcomes
	latestSummary, _ := FindLatestSessionSummary(s.store, s.LedgerDir)
	if latestSummary != nil {
		outcomes, _ := s.store.LoadSessionOutcomes(latestSummary.SessionID)
		symbolAgents := make(map[string]map[string]struct{})
		for _, outcome := range outcomes {
			if symbolAgents[outcome.Symbol] == nil {
				symbolAgents[outcome.Symbol] = make(map[string]struct{})
			}
			symbolAgents[outcome.Symbol][outcome.AgentID] = struct{}{}
		}
		for symbol, agents := range symbolAgents {
			count := len(agents)
			if count >= 4 {
				warnings = append(warnings, fmt.Sprintf("重疊過高：%s（%d 個 AI）", symbol, count))
			} else if count >= 3 {
				warnings = append(warnings, fmt.Sprintf("擁擠交易：%s（%d 個 AI）", symbol, count))
			}
		}
	}
	regime, regimeSource := s.resolveRegime()

	now := time.Now()
	channels := []DataChannelInfo{
		buildChannelInfo("us_yahoo", "Yahoo Finance Macro", checkMacroHealth, filepath.Join(s.WorkDir, "data/state/macro/latest.json"), now, s.healthStore),
		buildChannelInfo("us_spx", "S&P 500", makeMacroPointChecker("S&P 500", func(s marketdata.MacroDataSnapshot) marketdata.MacroDataPoint { return s.SPXIndex }), filepath.Join(s.WorkDir, "data/state/macro/latest.json"), now, s.healthStore),
		buildChannelInfo("us_ndx", "NASDAQ 100", makeMacroPointChecker("NASDAQ 100", func(s marketdata.MacroDataSnapshot) marketdata.MacroDataPoint { return s.NDXIndex }), filepath.Join(s.WorkDir, "data/state/macro/latest.json"), now, s.healthStore),
		buildChannelInfo("us_dji", "道瓊工業指數", makeMacroPointChecker("道瓊工業指數", func(s marketdata.MacroDataSnapshot) marketdata.MacroDataPoint { return s.DJIIndex }), filepath.Join(s.WorkDir, "data/state/macro/latest.json"), now, s.healthStore),
		buildChannelInfo("sox_index", "費城半導體指數 (SOX)", makeMacroPointChecker("費城半導體指數 (SOX)", func(s marketdata.MacroDataSnapshot) marketdata.MacroDataPoint { return s.SOXIndex }), filepath.Join(s.WorkDir, "data/state/macro/latest.json"), now, s.healthStore),
		buildChannelInfo("us_nvda", "NVIDIA (NVDA)", makeMacroPointChecker("NVIDIA (NVDA)", func(s marketdata.MacroDataSnapshot) marketdata.MacroDataPoint { return s.NVDA }), filepath.Join(s.WorkDir, "data/state/macro/latest.json"), now, s.healthStore),
		buildChannelInfo("us_aapl", "Apple (AAPL)", makeMacroPointChecker("Apple (AAPL)", func(s marketdata.MacroDataSnapshot) marketdata.MacroDataPoint { return s.AAPL }), filepath.Join(s.WorkDir, "data/state/macro/latest.json"), now, s.healthStore),
		buildChannelInfo("us_msft", "Microsoft (MSFT)", makeMacroPointChecker("Microsoft (MSFT)", func(s marketdata.MacroDataSnapshot) marketdata.MacroDataPoint { return s.MSFT }), filepath.Join(s.WorkDir, "data/state/macro/latest.json"), now, s.healthStore),
		buildChannelInfo("tsm_adr", "TSMC ADR", makeMacroPointChecker("TSMC ADR", func(s marketdata.MacroDataSnapshot) marketdata.MacroDataPoint { return s.TSMADR }), filepath.Join(s.WorkDir, "data/state/macro/latest.json"), now, s.healthStore),
		buildChannelInfo("twse_capital_flow", "TWSE 三大法人", checkCapitalFlowHealth, filepath.Join(s.WorkDir, constants.StateCapitalFlow), now, s.healthStore),
		buildChannelInfo("geopolitical", "地緣政治風險", checkGeopoliticalHealth, filepath.Join(s.WorkDir, constants.StateGeopolitical+"/latest.json"), now, s.healthStore),
		buildChannelInfo("twse_replay", "TWSE Replay", checkReplayHealth, config.GetReplayDataPath(s.WorkDir), now, s.healthStore),
		buildChannelInfo("frankfurter_fx", "日元匯率 (JPY)", checkJPYHealth, filepath.Join(s.WorkDir, "data/state/macro/latest.json"), now, s.healthStore),
		buildChannelInfo("twse_margin", "TWSE 融資融券", checkMarginHealth, filepath.Join(s.WorkDir, "data/state/margin"), now, s.healthStore),
		buildChannelInfo("export_statistics", "台灣海關進出口", checkExportHealth, filepath.Join(s.WorkDir, constants.StateExport), now, s.healthStore),
		buildChannelInfo("tsmc_revenue", "台積電月營收", checkTSMCRevenueHealth, filepath.Join(s.WorkDir, "data/state/tsmc_revenue"), now, s.healthStore),
		buildChannelInfo("geopolitical_taiwan", "台灣地緣政治", checkGeopoliticalHealth, filepath.Join(s.WorkDir, constants.StateGeopolitical+"/taiwan/latest.json"), now, s.healthStore),
	}
	channels = append(channels, buildAPIKeyChannel("fugle", "Fugle 富果", "FUGLE_API_KEY", "ATLAS_FUGLE_API_KEY", s.healthStore))
	channels = append(channels, buildAPIKeyChannel("fubon", "富邦證券", "FUBON_API_KEY", "ATLAS_FUBON_API_KEY", s.healthStore))
	channels = append(channels, buildAPIKeyChannel("finmind", "FinMind", "FINMIND_API_KEY", "", s.healthStore))
	channels = append(channels, buildAPIKeyChannel("tej", "TEJ 台灣經濟新報", "TEJ_API_KEY", "", s.healthStore))
	if s.JanusEngine != nil {
		janusFileStatus, janusFileUpdated := checkJanusHealth(s.JanusEngine, now)
		janusStatus, janusUpdated, _ := resolveChannelStatusFromStore(s.healthStore, "janus_regime", janusFileStatus, janusFileUpdated)
		channels = append(channels, DataChannelInfo{
			ChannelID:  "janus_regime",
			Label:      "JANUS 盤勢偵測",
			Status:     janusStatus,
			StatusText: StatusText(janusStatus),
			UpdatedAt:  janusUpdated,
		})
	}

	cycleStaleCount, cycleTotal := s.cycleStaleStats()
	runtimeInfo := buildinfo.Current()
	return SystemHealthResponse{
		BaselineVersion:       baselineVersion,
		ReplayDataLatestDate:  latestReplayDate,
		ReplayDataPathOK:      replayOK,
		LastWindowID:          lastWindow,
		LastWindowGeneratedAt: lastWindowTime,
		Warnings:              warnings,
		Regime:                regime,
		RegimeSource:          regimeSource,
		DataChannels:          channels,
		DegradedChannels:      degradedFrom(channels),
		CycleStaleCount:       cycleStaleCount,
		CycleTrackedTotal:     cycleTotal,
		CycleStale:            cycleStaleCount*2 > cycleTotal,
		BacktestStale:         backtestStale,
		Runtime:               &runtimeInfo,
	}, nil
}

// degradedFrom returns the channel IDs that should be surfaced as degraded.
// Only warn/error/partial count — inactive (未啟用: operator-disabled or missing
// API key) and expected_delay (正常延遲) are intentional states, not degradation.
func degradedFrom(channels []DataChannelInfo) []string {
	var d []string
	for _, c := range channels {
		switch c.Status {
		case "warn", "error", "partial":
			d = append(d, c.ChannelID)
		}
	}
	return d
}

func buildChannelInfo(id, label string, checker func(string, time.Time) (string, string), path string, now time.Time, healthStore *apigateway.ChannelHealthStore) DataChannelInfo {
	fileStatus, fileUpdated := checker(path, now)
	status, updated, _ := resolveChannelStatusFromStore(healthStore, id, fileStatus, fileUpdated)
	return DataChannelInfo{
		ChannelID:  id,
		Label:      label,
		Status:     status,
		StatusText: StatusText(status),
		UpdatedAt:  updated,
	}
}

func buildAPIKeyChannel(id, label, primaryKey, fallbackKey string, healthStore *apigateway.ChannelHealthStore) DataChannelInfo {
	key := config.GetSecret(primaryKey)
	if key == "" && fallbackKey != "" {
		key = config.GetSecret(fallbackKey)
	}
	if key == "" {
		return DataChannelInfo{
			ChannelID:  id,
			Label:      label,
			Status:     "inactive",
			StatusText: StatusText("inactive"),
			UpdatedAt:  "未設定 API Key",
		}
	}
	status, updated, _ := resolveChannelStatusFromStore(healthStore, id, "ok", "API key 已設定")
	return DataChannelInfo{
		ChannelID:  id,
		Label:      label,
		Status:     status,
		StatusText: StatusText(status),
		UpdatedAt:  updated,
	}
}

// Health check functions

// isWeekendGap returns true if the data age is primarily explained by weekend,
// meaning the last trading day was Friday and markets are closed Sat+Sun.
// For US macro data, the gap from Friday close to Monday open is ~52 hours.
func isWeekendGap(dataTime, now time.Time, maxWeekendHours int) bool {
	if maxWeekendHours <= 0 {
		maxWeekendHours = 72
	}
	age := now.Sub(dataTime)
	if age <= 24*time.Hour {
		return false
	}
	dataWeekday := dataTime.Weekday()
	nowWeekday := now.Weekday()
	if (dataWeekday == time.Friday || dataWeekday == time.Saturday) &&
		(nowWeekday == time.Monday || nowWeekday == time.Tuesday || nowWeekday == time.Sunday) {
		return age < time.Duration(maxWeekendHours)*time.Hour
	}
	return false
}

func checkMacroHealth(path string, now time.Time) (string, string) {
	info, err := os.Stat(path)
	if err != nil {
		return "error", "檔案不存在"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "error", "無法讀取"
	}
	var snap struct {
		RecordedAt int64 `json:"recorded_at"`
		DXY        struct {
			Timestamp int64 `json:"timestamp"`
		} `json:"dxy"`
		Oil struct {
			Timestamp int64 `json:"timestamp"`
		} `json:"oil"`
		USD_TWD struct {
			Symbol    string  `json:"symbol"`
			ChangePct float64 `json:"change_pct"`
			Timestamp int64   `json:"timestamp"`
		} `json:"usd_twd"`
		JPY struct {
			Symbol    string  `json:"symbol"`
			ChangePct float64 `json:"change_pct"`
			Timestamp int64   `json:"timestamp"`
		} `json:"jpy"`
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		logging.Warn("system_service", "parse_macro_health", logging.Err(err))
	}

	// Check data validity: forex pairs with symbol but zero change_pct
	// indicate a data pipeline issue (e.g., Yahoo returning only 1 data point).
	// Only flag when the data timestamp is also missing or older than 24h,
	// since a zero change on recent data is legitimate (first run or flat market).
	var staleIndicators []string
	if snap.USD_TWD.Symbol != "" && snap.USD_TWD.ChangePct == 0 {
		if snap.USD_TWD.Timestamp == 0 || now.Sub(time.Unix(snap.USD_TWD.Timestamp, 0)) > 24*time.Hour {
			staleIndicators = append(staleIndicators, "USD/TWD")
		}
	}
	if snap.JPY.Symbol != "" && snap.JPY.ChangePct == 0 {
		if snap.JPY.Timestamp == 0 || now.Sub(time.Unix(snap.JPY.Timestamp, 0)) > 24*time.Hour {
			staleIndicators = append(staleIndicators, "JPY")
		}
	}

	latest := info.ModTime()
	if snap.RecordedAt > 0 {
		latest = time.Unix(snap.RecordedAt, 0)
	}
	if snap.DXY.Timestamp > 0 {
		dxyTime := time.Unix(snap.DXY.Timestamp, 0)
		if dxyTime.After(latest) {
			latest = dxyTime
		}
	}
	if snap.Oil.Timestamp > 0 {
		oilTime := time.Unix(snap.Oil.Timestamp, 0)
		if oilTime.After(latest) {
			latest = oilTime
		}
	}
	detail := latest.Format("2006-01-02 15:04:05")

	// Data validity check overrides timestamp-only status when indicators are stale.
	if len(staleIndicators) > 0 {
		return "warn", detail + " | 資料異常: " + strings.Join(staleIndicators, ", ") + " 日變動率為0"
	}

	age := now.Sub(latest)
	if age < 24*time.Hour {
		return "ok", detail
	}
	if age < 7*24*time.Hour {
		if isWeekendGap(latest, now, 72) {
			return "expected_delay", detail
		}
		return "warn", detail
	}
	return "error", detail
}

func checkGeopoliticalHealth(path string, now time.Time) (string, string) {
	info, err := os.Stat(path)
	if err != nil {
		return "error", "檔案不存在"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "error", "無法讀取"
	}
	var score struct {
		Timestamp time.Time `json:"timestamp"`
	}
	if err := json.Unmarshal(data, &score); err != nil {
		logging.Warn("system_service", "parse_geopolitical_health", logging.Err(err))
	}
	var latest time.Time
	if !score.Timestamp.IsZero() {
		latest = score.Timestamp
	} else {
		latest = info.ModTime()
	}

	age := now.Sub(latest)
	if age < 24*time.Hour {
		return "ok", latest.Format("2006-01-02 15:04:05")
	}
	if age < 7*24*time.Hour {
		return "warn", latest.Format("2006-01-02 15:04:05")
	}
	return "error", latest.Format("2006-01-02 15:04:05")
}

func checkReplayHealth(path string, now time.Time) (string, string) {
	f, err := os.Open(path)
	if err != nil {
		return "error", "檔案不存在"
	}
	defer func() { _ = f.Close() }()

	var lastLine string
	scanner := bufio.NewScanner(f)
	lines := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if strings.TrimSpace(line) != "" {
			lastLine = line
		}
	}
	if lastLine == "" {
		return "error", "空檔案"
	}

	trimmed := strings.TrimSpace(lastLine)
	if strings.HasPrefix(trimmed, "{") {
		var row struct {
			Date string `json:"date"`
		}
		if err := json.Unmarshal([]byte(lastLine), &row); err != nil {
			return "error", "JSON 解析失敗"
		}
		if row.Date == "" {
			return "error", "JSON 缺少 date 欄位"
		}
		t, err := time.Parse("2006-01-02", row.Date)
		if err != nil {
			return "error", "日期解析失敗"
		}
		age := now.Sub(t)
		if age < 3*24*time.Hour {
			return "ok", row.Date
		}
		if age < 14*24*time.Hour {
			return "warn", row.Date
		}
		return "error", row.Date
	}

	// CSV format: date,col2,col3,...
	parts := strings.Split(lastLine, ",")
	if len(parts) == 0 {
		return "error", "格式錯誤"
	}
	latestDate := strings.TrimSpace(parts[0])
	t, err := time.Parse("2006-01-02", latestDate)
	if err != nil {
		return "error", "日期解析失敗"
	}

	// Check zero-change ratio for last two dates
	if len(lines) > 1 {
		prevCloseByCode := make(map[string]float64)
		lastCloseByCode := make(map[string]float64)
		var prevDate string
		for _, line := range slices.Backward(lines) {
			row := strings.Split(line, ",")
			if len(row) < 9 || row[0] == "Date" {
				continue
			}
			date := row[0]
			if date != latestDate && prevDate == "" {
				prevDate = date
			}
			if date == latestDate && len(row) >= 9 {
				closeVal, _ := strconv.ParseFloat(strings.TrimSpace(row[8]), 64)
				lastCloseByCode[row[1]] = closeVal
			}
			if date == prevDate && len(row) >= 9 {
				closeVal, _ := strconv.ParseFloat(strings.TrimSpace(row[8]), 64)
				prevCloseByCode[row[1]] = closeVal
			}
		}
		zeroChange := 0
		compared := 0
		for code, lastClose := range lastCloseByCode {
			if prevClose, ok := prevCloseByCode[code]; ok && prevClose > 0 {
				compared++
				if lastClose == prevClose {
					zeroChange++
				}
			}
		}
		if compared > 0 {
			ratio := float64(zeroChange) / float64(compared)
			if ratio > 0.3 {
				return "warn", fmt.Sprintf("%s (%.0f%% 標的隔日收盤價無變動，請檢查 backfill 資料)", latestDate, ratio*100)
			}
		}
	}

	age := now.Sub(t)
	if age < 3*24*time.Hour {
		return "ok", latestDate
	}
	if age < 14*24*time.Hour {
		return "warn", latestDate
	}
	return "error", latestDate
}

func checkCapitalFlowHealth(dir string, now time.Time) (string, string) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return "error", "無資料"
	}
	var latestFile string
	var latestModTime time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if e.Name() > latestFile {
			latestFile = e.Name()
			info, _ := e.Info()
			if info != nil {
				latestModTime = info.ModTime()
			}
		}
	}
	if latestFile == "" {
		return "error", "無有效檔案"
	}
	dateStr := strings.TrimSuffix(latestFile, ".json")

	// Use file modification time to determine freshness, since TWSE data is always 1 day delayed.
	if !latestModTime.IsZero() {
		age := now.Sub(latestModTime)
		if age < 24*time.Hour {
			return "ok", dateStr
		}
		if age < 7*24*time.Hour {
			return "warn", dateStr
		}
		return "error", dateStr
	}

	var dataTs time.Time
	data, err := os.ReadFile(filepath.Join(dir, latestFile))
	if err == nil {
		var flow struct {
			Date string `json:"date"`
		}
		if json.Unmarshal(data, &flow) == nil && flow.Date != "" {
			if parsed, err := time.ParseInLocation("20060102", flow.Date, time.FixedZone("CST", 8*60*60)); err == nil {
				dataTs = parsed
			}
		}
	}

	var t time.Time
	if !dataTs.IsZero() {
		t = dataTs
	} else {
		parsed, err := time.Parse("20060102", dateStr)
		if err != nil {
			return "error", "日期解析失敗"
		}
		t = parsed
	}

	age := now.Sub(t)
	if age < 24*time.Hour {
		return "ok", dateStr
	}
	if age < 7*24*time.Hour {
		return "warn", dateStr
	}
	return "error", dateStr
}

func checkMarginHealth(dir string, now time.Time) (string, string) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return "error", "無資料"
	}
	var latestFile string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_margin.json") {
			continue
		}
		if e.Name() > latestFile {
			latestFile = e.Name()
		}
	}
	if latestFile == "" {
		return "error", "無有效檔案"
	}
	dateStr := strings.TrimSuffix(latestFile, "_margin.json")

	var dataTs time.Time
	data, err := os.ReadFile(filepath.Join(dir, latestFile))
	if err == nil {
		var margin struct {
			Date string `json:"date"`
		}
		if json.Unmarshal(data, &margin) == nil && margin.Date != "" {
			if parsed, err := time.ParseInLocation("20060102", margin.Date, time.FixedZone("CST", 8*60*60)); err == nil {
				dataTs = parsed
			}
		}
	}

	var t time.Time
	if !dataTs.IsZero() {
		t = dataTs
	} else {
		parsed, err := time.Parse("20060102", dateStr)
		if err != nil {
			return "error", "日期解析失敗"
		}
		t = parsed
	}

	age := now.Sub(t)
	if age < 3*24*time.Hour {
		return "ok", dateStr
	}
	if age < 7*24*time.Hour {
		return "warn", dateStr
	}
	return "error", dateStr
}

func checkJPYHealth(path string, now time.Time) (string, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "error", fmt.Sprintf("總經快照檔案不存在: %s", err)
	}
	var snap struct {
		JPY struct {
			Timestamp int64 `json:"timestamp"`
		} `json:"jpy"`
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		logging.Warn("system_service", "parse_jpy_health", logging.Err(err))
	}
	if snap.JPY.Timestamp == 0 {
		return "error", "無 JPY 資料 — Frankfurter API (USD/JPY) 尚未成功獲取"
	}
	t := time.Unix(snap.JPY.Timestamp, 0)
	age := now.Sub(t)
	if age < 24*time.Hour {
		return "ok", t.Format("2006-01-02 15:04:05")
	}
	if age < 7*24*time.Hour {
		if isWeekendGap(t, now, 72) {
			return "expected_delay", fmt.Sprintf("%s（%d 天前，週末非交易日）", t.Format("2006-01-02 15:04:05"), int(age.Hours()/24))
		}
		return "warn", fmt.Sprintf("%s（%d 天前）", t.Format("2006-01-02 15:04:05"), int(age.Hours()/24))
	}
	return "error", fmt.Sprintf("%s（%d 天前，已超過 7 天閾值）— Frankfurter API 連線失敗", t.Format("2006-01-02 15:04:05"), int(age.Hours()/24))
}

// checkMacroPointHealth evaluates freshness for a single MacroDataPoint field
// within the latest macro snapshot. It mirrors checkJPYHealth's thresholds.
func checkMacroPointHealth(path string, now time.Time, label string, extractor func(marketdata.MacroDataSnapshot) marketdata.MacroDataPoint) (string, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "error", fmt.Sprintf("%s 總經快照檔案不存在: %s", label, err)
	}
	var snap marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		logging.Warn("system_service", "parse_macro_point_health", logging.FStr("label", label), logging.Err(err))
	}
	point := extractor(snap)
	if point.Timestamp == 0 {
		return "error", fmt.Sprintf("無 %s 資料", label)
	}
	t := time.Unix(point.Timestamp, 0)
	age := now.Sub(t)
	if age < 24*time.Hour {
		return "ok", t.Format("2006-01-02 15:04:05")
	}
	if age < 7*24*time.Hour {
		if isWeekendGap(t, now, 72) {
			return "expected_delay", fmt.Sprintf("%s（%d 天前，週末非交易日）", t.Format("2006-01-02 15:04:05"), int(age.Hours()/24))
		}
		return "warn", fmt.Sprintf("%s（%d 天前）", t.Format("2006-01-02 15:04:05"), int(age.Hours()/24))
	}
	return "error", fmt.Sprintf("%s（%d 天前，已超過 7 天閾值）", t.Format("2006-01-02 15:04:05"), int(age.Hours()/24))
}

// makeMacroPointChecker returns a channel-health checker for a specific
// MacroDataSnapshot field. The returned closure has the signature expected by
// buildChannelInfo.
func makeMacroPointChecker(label string, extractor func(marketdata.MacroDataSnapshot) marketdata.MacroDataPoint) func(string, time.Time) (string, string) {
	return func(path string, now time.Time) (string, string) {
		return checkMacroPointHealth(path, now, label, extractor)
	}
}

func checkJanusHealth(engine *janus.Engine, now time.Time) (string, string) {
	if engine == nil {
		return "inactive", "JANUS engine 未啟用"
	}
	status := engine.GetStatus()
	if status.LastUpdated.IsZero() {
		return "warn", "JANUS 已載入但尚未更新"
	}
	age := now.Sub(status.LastUpdated)
	if age < 7*24*time.Hour {
		return "ok", status.LastUpdated.Format("2006-01-02 15:04:05")
	}
	if age < 30*24*time.Hour {
		return "warn", status.LastUpdated.Format("2006-01-02 15:04:05")
	}
	return "error", status.LastUpdated.Format("2006-01-02 15:04:05")
}

func checkTSMCRevenueHealth(dir string, now time.Time) (string, string) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return "error", "無資料"
	}
	var latestFile string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_revenue.json") {
			continue
		}
		if e.Name() > latestFile {
			latestFile = e.Name()
		}
	}
	if latestFile == "" {
		return "error", "無有效檔案"
	}
	dateStr := strings.TrimSuffix(latestFile, "_revenue.json")

	var dataTs time.Time
	data, err := os.ReadFile(filepath.Join(dir, latestFile))
	if err == nil {
		var rev struct {
			Date string `json:"date"`
		}
		if json.Unmarshal(data, &rev) == nil && rev.Date != "" {
			dataTs = parseROCYearMonth(rev.Date)
		}
	}

	var t time.Time
	if !dataTs.IsZero() {
		t = dataTs
	} else {
		t = parseROCYearMonth(dateStr)
	}
	if t.IsZero() {
		return "error", "日期解析失敗"
	}

	age := now.Sub(t)
	if age < 45*24*time.Hour {
		return "ok", dateStr
	}
	if age < 90*24*time.Hour {
		return "warn", dateStr
	}
	return "error", dateStr
}

func parseROCYearMonth(s string) time.Time {
	if len(s) != 5 {
		return time.Time{}
	}
	rocYear, err1 := strconv.Atoi(s[:3])
	month, err2 := strconv.Atoi(s[3:])
	if err1 != nil || err2 != nil || month < 1 || month > 12 {
		return time.Time{}
	}
	return time.Date(rocYear+1911, time.Month(month), 1, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
}

func checkExportHealth(dir string, now time.Time) (string, string) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return "error", "無資料"
	}
	var latestFile string
	var latestModTime time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_export.json") {
			continue
		}
		if e.Name() > latestFile {
			latestFile = e.Name()
			info, _ := e.Info()
			if info != nil {
				latestModTime = info.ModTime()
			}
		}
	}
	if latestFile == "" {
		return "error", "無有效檔案"
	}
	dateStr := strings.TrimSuffix(latestFile, "_export.json")

	// Customs data is released with a delay; use file modification time to check if the fetch task is running.
	if !latestModTime.IsZero() {
		age := now.Sub(latestModTime)
		if age < 24*time.Hour {
			return "ok", dateStr
		}
		if age < 7*24*time.Hour {
			return "warn", dateStr
		}
		return "error", dateStr
	}

	var dataTs time.Time
	data, err := os.ReadFile(filepath.Join(dir, latestFile))
	if err == nil {
		var exp struct {
			Year  int `json:"year"`
			Month int `json:"month"`
		}
		if json.Unmarshal(data, &exp) == nil && exp.Year > 0 && exp.Month >= 1 {
			dataTs = time.Date(exp.Year+1911, time.Month(exp.Month), 1, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
		}
	}

	var t time.Time
	if !dataTs.IsZero() {
		t = dataTs
	} else {
		if len(dateStr) != 5 {
			return "error", "日期解析失敗"
		}
		rocYear, err1 := strconv.Atoi(dateStr[:3])
		month, err2 := strconv.Atoi(dateStr[3:])
		if err1 != nil || err2 != nil || month < 1 || month > 12 {
			return "error", "日期解析失敗"
		}
		t = time.Date(rocYear+1911, time.Month(month), 1, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	}

	age := now.Sub(t)
	if age < 45*24*time.Hour {
		return "ok", dateStr
	}
	if age < 90*24*time.Hour {
		return "warn", dateStr
	}
	return "error", dateStr
}

func (s *SystemService) LoadClampingEvents(limit int) ([]eventbus.ClampingEventPayload, error) {
	path := filepath.Join(s.LedgerDir, "clamping_events.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []eventbus.ClampingEventPayload{}, nil
		}
		return nil, fmt.Errorf("open clamping events: %w", err)
	}
	defer func() { _ = f.Close() }()

	var events []eventbus.ClampingEventPayload
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e eventbus.ClampingEventPayload
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			logging.Warn("system_service", "corrupted_clamping_event_skipped", logging.Err(err))
			continue
		}
		events = append(events, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan clamping events: %w", err)
	}

	if len(events) > limit {
		return events[len(events)-limit:], nil
	}
	return events, nil
}

func (s *SystemService) LoadConvictionClampingEvents(limit int) ([]portfolio.ConvictionClampingEvent, error) {
	path := filepath.Join(s.LedgerDir, "clamping_events.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []portfolio.ConvictionClampingEvent{}, nil
		}
		return nil, fmt.Errorf("open clamping events: %w", err)
	}
	defer func() { _ = f.Close() }()

	var events []portfolio.ConvictionClampingEvent
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e portfolio.ConvictionClampingEvent
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			logging.Warn("system_service", "corrupted_conviction_clamping_event_skipped", logging.Err(err))
			continue
		}
		events = append(events, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan conviction clamping events: %w", err)
	}

	if len(events) > limit {
		return events[len(events)-limit:], nil
	}
	return events, nil
}

// cycleStaleStats returns (staleCount, total) for the industry cycle
// positions. Staleness is per-position: UpdatedAt older than 24h.
func (s *SystemService) cycleStaleStats() (staleCount, total int) {
	if s.CycleTracker == nil {
		return 0, 0
	}
	positions := s.CycleTracker.GetAllPositions()
	for _, pos := range positions {
		if time.Since(pos.UpdatedAt) > 24*time.Hour {
			staleCount++
		}
	}
	return staleCount, len(positions)
}

// checkCycleStale uses majority semantics (>50% of tracked positions stale).
// The old ANY-stale rule made the dashboard card permanently red: sub-industry
// positions without an aggregation target (e.g. etf_rotation, which has no
// representative stocks and is a strategy bucket, not a data-source industry)
// stay old forever and tripped the whole card (#1776 audit, 2026-08-31).
// Majority-stale still trips when aggregation is genuinely broken.
func (s *SystemService) checkCycleStale() bool {
	if s.CycleTracker == nil {
		return false
	}
	staleCount, total := s.cycleStaleStats()
	if total == 0 {
		return true
	}
	return staleCount*2 > total
}

func (s *SystemService) SetCycleTracker(ct *industry.CycleTracker) {
	s.CycleTracker = ct
}
