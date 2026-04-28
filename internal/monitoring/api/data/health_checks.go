package data

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/janus"
)

// ChannelAlert represents a single unhealthy channel.
type ChannelAlert struct {
	ChannelID string `json:"channel_id"`
	Status    string `json:"status"`
	Error     string `json:"error"`
	FetchAt   string `json:"fetch_at"`
}

// ChannelHealthRecord stores the last fetch result for a single channel.
type ChannelHealthRecord struct {
	Status        string `json:"status"`
	LastFetchAt   string `json:"last_fetch_at"`
	LastError     string `json:"last_error,omitempty"`
	LastSuccessAt string `json:"last_success_at,omitempty"`
}

// ChannelHealthRecorder records and retrieves channel health status.
type ChannelHealthRecorder interface {
	Record(channelID, status, errMsg string) error
	Get(channelID string) *ChannelHealthRecord
	Alerts() []ChannelAlert
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
