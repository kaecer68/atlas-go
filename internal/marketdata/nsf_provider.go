package marketdata

import (
	"context"
	"fmt"
)

// NationalStabilizationProvider is a static provider of Taiwan's National
// Stabilization Fund (國安基金) intervention periods — the rare, high-impact
// market-support episodes used by the black-swan adjudication (R8).
//
// Source: 國安基金委員會公開新聞稿 / 維基百科「國家金融安定基金」歷次護盤表
// (verified 2026-08-25). Entries are the *effective* intervention windows:
// entry is the first trading day of the period (start date per public
// announcement) and exit is the announced unwinding/exit date.
//
// R8 notes:
//   - 2018-10-11 授權啟動但委員會決議未實際進場 → 不列為護盤期間（無動用資金）。
//   - 2025-04-09 ~ 2026-01-12（川普關稅）在 replay 窗口（2024-07 起）內，黑天鵝
//     判定與事件研究必須納入。
//   - 本表為人工維護（極罕見事件，不需資料通道）；新增護盤期間時同步更新
//     nsfInterventionPeriods 並跑 backfill-event-calendar -provider nsf。
//
// Maturity: evolving (static table; refresh on each new intervention)
type NationalStabilizationProvider struct{}

// NewNationalStabilizationProvider creates the static NSF provider.
func NewNationalStabilizationProvider() *NationalStabilizationProvider {
	return &NationalStabilizationProvider{}
}

// Name returns the provider name.
func (p *NationalStabilizationProvider) Name() string {
	return "nsf_static"
}

// nsfPeriod describes one intervention window.
type nsfPeriod struct {
	start        string // ISO YYYY-MM-DD（進場日）
	end          string // ISO YYYY-MM-DD（退場日，含）
	indexAtEntry string // 進場時加權指數
	event        string // 觸發事件（中文）
}

// nsfInterventionPeriods holds the 9 published NSF intervention windows.
var nsfInterventionPeriods = []nsfPeriod{
	{start: "2000-03-16", end: "2000-03-20", indexAtEntry: "8682.76", event: "首次政權交替，兩岸緊張"},
	{start: "2000-10-03", end: "2000-11-15", indexAtEntry: "5805.17", event: "網路泡沫、國際油價大漲"},
	{start: "2004-05-20", end: "2004-06-01", indexAtEntry: "5860.58", event: "三一九槍擊事件、兩岸關係緊張"},
	{start: "2008-09-18", end: "2008-12-17", indexAtEntry: "5641.95", event: "2008 金融海嘯"},
	{start: "2011-12-21", end: "2012-04-20", indexAtEntry: "6966.48", event: "歐洲主權債務危機"},
	{start: "2015-08-25", end: "2016-04-12", indexAtEntry: "7675.64", event: "人民幣劇貶、亞洲貨幣競貶"},
	{start: "2020-03-20", end: "2020-10-12", indexAtEntry: "8681.34", event: "COVID-19 全球大流行、油價暴跌"},
	{start: "2022-07-13", end: "2023-04-13", indexAtEntry: "13950.62", event: "俄烏戰爭、加速升息"},
	{start: "2025-04-09", end: "2026-01-12", indexAtEntry: "18337.44", event: "川普第二任期關稅"},
}

// FetchEvents returns NSF entry/exit events for the given year. Years with no
// published intervention return an empty slice.
func (p *NationalStabilizationProvider) FetchEvents(ctx context.Context, year int) ([]CalendarProviderData, error) {
	var events []CalendarProviderData
	for _, pr := range nsfInterventionPeriods {
		if hasYearPrefix(pr.start, year) {
			events = append(events, CalendarProviderData{
				Date:        pr.start,
				EventType:   "national_stabilization",
				Name:        fmt.Sprintf("國安基金進場 %s", pr.event),
				Symbol:      "",
				Direction:   "bullish",
				Weight:      0.95,
				Description: fmt.Sprintf("國安基金護盤進場（%s，進場指數 %s），護盤至 %s", pr.event, pr.indexAtEntry, pr.end),
				Source:      "nsf_static",
			})
		}
		if hasYearPrefix(pr.end, year) {
			events = append(events, CalendarProviderData{
				Date:        pr.end,
				EventType:   "national_stabilization_exit",
				Name:        fmt.Sprintf("國安基金退場 %s", pr.event),
				Symbol:      "",
				Direction:   "neutral",
				Weight:      0.8,
				Description: fmt.Sprintf("國安基金護盤退場（%s，進場 %s）", pr.event, pr.start),
				Source:      "nsf_static",
			})
		}
	}
	return events, nil
}

// IsInterventionActive reports whether the given date falls inside any NSF
// intervention window. Used by black-swan adjudication (R8).
func (p *NationalStabilizationProvider) IsInterventionActive(isoDate string) bool {
	for _, pr := range nsfInterventionPeriods {
		if isoDate >= pr.start && isoDate <= pr.end {
			return true
		}
	}
	return false
}
