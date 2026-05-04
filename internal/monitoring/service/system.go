package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// SystemService encapsulates system-level health and status monitoring.
type SystemService struct {
	WorkDir      string
	LedgerDir    string
	BaselinePath string
}

// NewSystemService creates a new SystemService.
func NewSystemService(workDir, ledgerDir, baselinePath string) *SystemService {
	return &SystemService{
		WorkDir:      workDir,
		LedgerDir:    ledgerDir,
		BaselinePath: baselinePath,
	}
}

// LoadPhase3Status loads Phase 3 metrics from the well-known path.
func (s *SystemService) LoadPhase3Status() (orchestrator.Phase3Metrics, error) {
	return orchestrator.LoadPhase3Metrics("")
}

// SystemHealthResponse mirrors the dashboard API response structure.
type SystemHealthResponse struct {
	BaselineVersion       string            `json:"baseline_version"`
	ReplayDataLatestDate  string            `json:"replay_data_latest_date"`
	ReplayDataPathOK      bool              `json:"replay_data_path_ok"`
	LastWindowID          string            `json:"last_window_id"`
	LastWindowGeneratedAt time.Time         `json:"last_window_generated_at"`
	Warnings              []string          `json:"warnings"`
	Regime                domain.Regime     `json:"regime"`
	DataChannels          []DataChannelInfo `json:"data_channels,omitempty"`
}

// DataChannelInfo represents a single data channel status.
type DataChannelInfo struct {
	ChannelID  string `json:"channel_id"`
	Label      string `json:"label"`
	Status     string `json:"status"`
	StatusText string `json:"status_text"`
	UpdatedAt  string `json:"updated_at"`
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

	replayPath := filepath.Join(s.WorkDir, "data/replay/tw_extended_90days.csv")
	replayOK := true
	latestReplayDate := ""
	ds, err := replay.LoadTWSEOpenDataCSV(replayPath)
	if err != nil {
		replayOK = false
		warnings = append(warnings, "replay 資料無法讀取："+err.Error())
	} else if len(ds.Dates) > 0 {
		latestReplayDate = ds.Dates[len(ds.Dates)-1].Format("2006-01-02")
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
	latestSummary, _ := LoadSessionSummary(s.LedgerDir, "")
	if latestSummary != nil {
		store := ledger.NewStore(s.LedgerDir)
		outcomes, _ := store.LoadSessionOutcomes(latestSummary.SessionID)
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
	regime := domain.RegimeNeutral
	if summary, err := LoadSessionSummary(s.LedgerDir, ""); err == nil && summary != nil {
		regime = summary.Regime
	}

	now := time.Now()
	channels := []DataChannelInfo{
		buildChannelInfo("us_yahoo", "Yahoo Finance Macro", checkMacroHealth, filepath.Join(s.WorkDir, "data/state/macro/latest.json"), now),
		buildChannelInfo("twse_capital_flow", "TWSE 三大法人", checkCapitalFlowHealth, filepath.Join(s.WorkDir, "data/state/capital_flow"), now),
		buildChannelInfo("geopolitical", "地緣政治風險", checkGeopoliticalHealth, filepath.Join(s.WorkDir, "data/state/geopolitical/latest.json"), now),
		buildChannelInfo("twse_replay", "TWSE Replay", checkReplayHealth, filepath.Join(s.WorkDir, "data/replay/tw_extended_90days.csv"), now),
	}

	return SystemHealthResponse{
		BaselineVersion:       baselineVersion,
		ReplayDataLatestDate:  latestReplayDate,
		ReplayDataPathOK:      replayOK,
		LastWindowID:          lastWindow,
		LastWindowGeneratedAt: lastWindowTime,
		Warnings:              warnings,
		Regime:                regime,
		DataChannels:          channels,
	}, nil
}

func buildChannelInfo(id, label string, checker func(string, time.Time) (string, string), path string, now time.Time) DataChannelInfo {
	status, updated := checker(path, now)
	return DataChannelInfo{
		ChannelID:  id,
		Label:      label,
		Status:     status,
		StatusText: StatusText(status),
		UpdatedAt:  updated,
	}
}

// Health check functions

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
	}
	_ = json.Unmarshal(data, &snap)

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

	age := now.Sub(latest)
	if age < 24*time.Hour {
		return "ok", latest.Format("2006-01-02 15:04:05")
	}
	if age < 7*24*time.Hour {
		return "warn", latest.Format("2006-01-02 15:04:05")
	}
	return "error", latest.Format("2006-01-02 15:04:05")
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
	_ = json.Unmarshal(data, &score)
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
	defer f.Close()

	var lastLine string
	scanner := bufio.NewScanner(f)
	lines := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		lastLine = line
	}
	if lastLine == "" {
		return "error", "空檔案"
	}
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
		for i := len(lines) - 1; i >= 0; i-- {
			row := strings.Split(lines[i], ",")
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
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if e.Name() > latestFile {
			latestFile = e.Name()
		}
	}
	if latestFile == "" {
		return "error", "無有效檔案"
	}
	dateStr := strings.TrimSuffix(latestFile, ".json")

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

func checkJPYHealth(path string, now time.Time) (string, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "error", "檔案不存在"
	}
	var snap struct {
		JPY struct {
			Timestamp int64 `json:"timestamp"`
		} `json:"jpy"`
	}
	_ = json.Unmarshal(data, &snap)
	if snap.JPY.Timestamp == 0 {
		return "error", "無 JPY 資料"
	}
	t := time.Unix(snap.JPY.Timestamp, 0)
	age := now.Sub(t)
	if age < 24*time.Hour {
		return "ok", t.Format("2006-01-02 15:04:05")
	}
	if age < 7*24*time.Hour {
		return "warn", t.Format("2006-01-02 15:04:05")
	}
	return "error", t.Format("2006-01-02 15:04:05")
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

func checkExportHealth(dir string, now time.Time) (string, string) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return "error", "無資料"
	}
	var latestFile string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_export.json") {
			continue
		}
		if e.Name() > latestFile {
			latestFile = e.Name()
		}
	}
	if latestFile == "" {
		return "error", "無有效檔案"
	}
	dateStr := strings.TrimSuffix(latestFile, "_export.json")

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
