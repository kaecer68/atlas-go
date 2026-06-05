package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/replay"
)

func loadLatestSessionSummaryFromDisk(ledgerDir string) (*domain.SessionSummary, error) {
	sessionsDir := filepath.Join(ledgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	summaries := make([]domain.SessionSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		summaryPath := filepath.Join(sessionsDir, entry.Name(), "summary.json")
		bytes, err := os.ReadFile(summaryPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		var summary domain.SessionSummary
		if err := json.Unmarshal(bytes, &summary); err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	if len(summaries) == 0 {
		return nil, nil
	}

	slices.SortFunc(summaries, func(a, b domain.SessionSummary) int {
		aDate := domain.SessionDateFromID(a.SessionID)
		bDate := domain.SessionDateFromID(b.SessionID)
		switch {
		case aDate.After(bDate):
			return -1
		case aDate.Before(bDate):
			return 1
		case a.RecordedAt.After(b.RecordedAt):
			return -1
		case a.RecordedAt.Before(b.RecordedAt):
			return 1
		default:
			return 0
		}
	})
	for i := range summaries {
		if summaries[i].OutcomeCount > 0 {
			selected := summaries[i]
			return &selected, nil
		}
	}
	latest := summaries[0]
	return &latest, nil
}

func LoadSessionSummary(ledgerDir, sessionID string) (*domain.SessionSummary, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("LoadSessionSummary requires non-empty sessionID; use FindLatestSessionSummary for latest")
	}

	sessionsDir := filepath.Join(ledgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() != sessionID {
			continue
		}
		summaryPath := filepath.Join(sessionsDir, entry.Name(), "summary.json")
		bytes, err := os.ReadFile(summaryPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		var summary domain.SessionSummary
		if err := json.Unmarshal(bytes, &summary); err != nil {
			return nil, err
		}
		return &summary, nil
	}
	return nil, nil
}

func FindLatestSessionSummary(store ledger.OutcomeStore, ledgerDir string) (*domain.SessionSummary, error) {
	if store != nil {
		summaries, err := store.LoadSessionSummaries()
		if err == nil && len(summaries) > 0 {
			slices.SortFunc(summaries, func(a, b domain.SessionSummary) int {
				aDate := domain.SessionDateFromID(a.SessionID)
				bDate := domain.SessionDateFromID(b.SessionID)
				switch {
				case aDate.After(bDate):
					return -1
				case aDate.Before(bDate):
					return 1
				case a.RecordedAt.After(b.RecordedAt):
					return -1
				case a.RecordedAt.Before(b.RecordedAt):
					return 1
				default:
					return 0
				}
			})
			for i := range summaries {
				if summaries[i].OutcomeCount > 0 {
					selected := summaries[i]
					return &selected, nil
				}
			}
			latest := summaries[0]
			return &latest, nil
		}
	}
	return loadLatestSessionSummaryFromDisk(ledgerDir)
}

func StatusText(status string) string {
	switch status {
	case "ok":
		return "正常"
	case "warn":
		return "待更新"
	case "expected_delay":
		return "正常延遲"
	case "error":
		return "異常"
	case "partial":
		return "部分異常"
	case "inactive":
		return "未啟用"
	default:
		return "未知"
	}
}

func ComputePipelineTags(_ context.Context, ds *replay.Dataset, symbol string, date time.Time) ([]string, error) {
	tags := computePipelineTags(ds, symbol, date)
	return tags, nil
}

func computePipelineTags(ds *replay.Dataset, symbol string, date time.Time) []string {
	if ds == nil {
		return nil
	}
	dateKey := date.Format("2006-01-02")
	bar, ok := ds.ByDate[dateKey][symbol]
	if !ok {
		return nil
	}
	var prevBar domain.DailyBar
	var hasPrev bool
	for i, d := range ds.Dates {
		if d.Format("2006-01-02") == dateKey && i > 0 {
			prevBar = ds.ByDate[ds.Dates[i-1].Format("2006-01-02")][symbol]
			hasPrev = prevBar.Close > 0
			break
		}
	}

	tags := make([]string, 0, 3)
	changePct := 0.0
	if bar.Open > 0 {
		changePct = (bar.Close - bar.Open) / bar.Open
	}
	if changePct > 0.035 {
		tags = append(tags, "長紅")
	} else if changePct < -0.035 {
		tags = append(tags, "長黑")
	}
	if hasPrev && prevBar.Volume > 0 && bar.Volume > int64(float64(prevBar.Volume)*1.5) {
		tags = append(tags, "放量")
	}

	high5 := math.Inf(-1)
	low5 := math.Inf(1)
	for i, d := range ds.Dates {
		if d.Format("2006-01-02") == dateKey {
			start := max(i-4, 0)
			// 排除今日 (i)，僅比較 start..i-1 這 4 個過往交易日，
			// 避免 bar.Close 與自身比較導致 high5/low5 永遠 == bar.Close。
			for _, pd := range ds.Dates[start:i] {
				b := ds.ByDate[pd.Format("2006-01-02")][symbol]
				if b.Close > high5 {
					high5 = b.Close
				}
				if b.Close > 0 && b.Close < low5 {
					low5 = b.Close
				}
			}
			break
		}
	}
	if bar.Close > 0 && bar.Close > high5 {
		tags = append(tags, "創5日高")
	}
	if bar.Close > 0 && low5 > 0 && bar.Close < low5 {
		tags = append(tags, "創5日低")
	}
	return tags
}

func FallbackPriceTargets(_ context.Context, skill string, price float64, side string) (float64, float64, error) {
	target, stopLoss := fallbackPriceTargets(skill, price, side)
	return target, stopLoss, nil
}

func fallbackPriceTargets(skill string, price float64, side string) (float64, float64) {
	targets := config.GetParametersConfig().FallbackPriceTargets
	var targetMult, stopLossMult float64
	if t, ok := targets[skill]; ok {
		targetMult = t.TargetMultiplier.Value
		stopLossMult = t.StopLossMultiplier.Value
	} else if t, ok := targets["_default"]; ok {
		targetMult = t.TargetMultiplier.Value
		stopLossMult = t.StopLossMultiplier.Value
	} else {
		targetMult = 1.05
		stopLossMult = 0.95
	}
	if side == "SELL" {
		return price * stopLossMult, price * targetMult
	}
	return price * targetMult, price * stopLossMult
}

func isStockPickingLayer(layer string) bool {
	return layer == "sector" || layer == "style" || layer == "superinvestor"
}

func isStockPickingLayerByID(agentID string, views []AgentUniverseViewData) bool {
	for _, v := range views {
		if v.AgentID == agentID {
			return isStockPickingLayer(v.Layer)
		}
	}
	return false
}
