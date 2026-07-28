package main

// liveDailyReportProvider replaces dailyreport's hardcoded defaultProvider
// with the same data sources that back /api/macro/snapshot/latest,
// /api/capital-flow/daily and /api/events/calendar, so the daily report can
// never contradict the live endpoints (fix manifest #B06).

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/dailyreport"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

type liveDailyReportProvider struct {
	macro    marketdata.MacroDataProvider
	capital  *capitalflow.Service
	calendar *industry.EventCalendar
}

func newLiveDailyReportProvider(macro marketdata.MacroDataProvider, capital *capitalflow.Service, calendar *industry.EventCalendar) *liveDailyReportProvider {
	return &liveDailyReportProvider{macro: macro, capital: capital, calendar: calendar}
}

func (p *liveDailyReportProvider) FetchMacro() (dailyreport.GlobalOverview, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	snap, err := p.macro.FetchSnapshot(ctx)
	if err != nil {
		// Honest placeholders over silently-wrong hardcoded numbers; the
		// Summary line is derived from regime in Generate() either way.
		return dailyreport.GlobalOverview{BondYield: "—", USDIndex: "—", JPY: "—", VIX: "—"}, nil
	}
	return dailyreport.GlobalOverview{
		BondYield: fmt.Sprintf("%.2f%%", snap.US10Y.Value),
		USDIndex:  fmt.Sprintf("%.1f", snap.DXY.Value),
		JPY:       fmt.Sprintf("%.1f", snap.JPY.Value),
		VIX:       fmt.Sprintf("%.1f", snap.VIX.Value),
	}, nil
}

func (p *liveDailyReportProvider) FetchCapital() (dailyreport.CapitalSection, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	daily, err := p.capital.LatestDaily(ctx)
	if err != nil {
		return dailyreport.CapitalSection{
			Foreign: "無資料", Institutional: "無資料", Dealer: "無資料",
			Government: "無資料", Retail: "無資料",
			Resonance: 0, Quality: "unavailable",
		}, nil
	}
	sec := dailyreport.CapitalSection{
		Resonance: daily.Resonance.Coefficient,
		Quality:   daily.QualityLabel,
	}
	for _, f := range daily.Forces {
		switch f.Force {
		case capitalflow.ForceForeign:
			sec.Foreign = capitalTrendLabel(f.Trend)
			sec.ForeignValue = f.RawValue
		case capitalflow.ForceInstitutional:
			sec.Institutional = capitalTrendLabel(f.Trend)
			sec.InstitutionalValue = f.RawValue
		case capitalflow.ForceDealer:
			sec.Dealer = capitalTrendLabel(f.Trend)
			sec.DealerValue = f.RawValue
		case capitalflow.ForceGovernment:
			sec.Government = capitalTrendLabel(f.Trend)
			sec.GovernmentValue = f.RawValue
		case capitalflow.ForceRetail:
			sec.Retail = capitalTrendLabel(f.Trend)
			sec.RetailValue = f.RawValue
		}
	}
	return sec, nil
}

func (p *liveDailyReportProvider) FetchEvents(now time.Time) (dailyreport.EventsSection, error) {
	active := p.calendar.DetectActiveEvents(now)
	sec := dailyreport.EventsSection{Tomorrow: []string{}, ThisWeek: []string{}, Count: len(active)}
	tomorrow := now.AddDate(0, 0, 1)
	weekEnd := now.AddDate(0, 0, 7)
	for _, ev := range active {
		if !ev.StartDate.After(tomorrow) && !ev.EndDate.Before(tomorrow) {
			sec.Tomorrow = append(sec.Tomorrow, ev.Name)
		}
		if !ev.EndDate.Before(now) && !ev.StartDate.After(weekEnd) {
			sec.ThisWeek = append(sec.ThisWeek, ev.Name)
		}
	}
	if len(sec.Tomorrow) == 0 {
		sec.Tomorrow = []string{"無重大事件"}
	}
	return sec, nil
}

func capitalTrendLabel(trend string) string {
	switch trend {
	case "bullish":
		return "偏多"
	case "bearish":
		return "偏空"
	default:
		return "中性"
	}
}
