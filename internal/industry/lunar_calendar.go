package industry

import (
	"time"

	"github.com/6tail/lunar-go/calendar"
)

// computeLunarNewYear returns the Gregorian date of Chinese New Year
// (農曆正月初一) for the given year using astronomical lunar calendar computation.
func computeLunarNewYear(year int) time.Time {
	lunar := calendar.NewLunarFromYmd(year, 1, 1)
	solar := lunar.GetSolar()
	return time.Date(solar.GetYear(), time.Month(solar.GetMonth()), solar.GetDay(), 0, 0, 0, 0, time.UTC)
}

// computeDragonBoat returns the Gregorian date of Dragon Boat Festival
// (農曆五月初五) for the given year.
func computeDragonBoat(year int) time.Time {
	lunar := calendar.NewLunarFromYmd(year, 5, 5)
	solar := lunar.GetSolar()
	return time.Date(solar.GetYear(), time.Month(solar.GetMonth()), solar.GetDay(), 0, 0, 0, 0, time.UTC)
}

// computeMidAutumn returns the Gregorian date of Mid-Autumn Festival
// (農曆八月十五) for the given year.
func computeMidAutumn(year int) time.Time {
	lunar := calendar.NewLunarFromYmd(year, 8, 15)
	solar := lunar.GetSolar()
	return time.Date(solar.GetYear(), time.Month(solar.GetMonth()), solar.GetDay(), 0, 0, 0, 0, time.UTC)
}

// computeQingming returns the Gregorian date of Qingming (清明) for the given year.
// Qingming is a solar term, typically falling on April 4 or 5.
func computeQingming(year int) time.Time {
	for _, day := range []int{4, 5, 6} {
		solar := calendar.NewSolarFromYmd(year, 4, day)
		lunar := solar.GetLunar()
		jq := lunar.GetCurrentJieQi()
		if jq != nil && jq.GetName() == "清明" {
			return time.Date(year, 4, day, 0, 0, 0, 0, time.UTC)
		}
	}
	// Fallback to April 5 if computation fails.
	return time.Date(year, 4, 5, 0, 0, 0, 0, time.UTC)
}
