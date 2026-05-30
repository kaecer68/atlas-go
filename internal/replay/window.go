package replay

import "time"

// WindowDates returns all dates in the dataset that fall within [start, end]
// and have the requested forward window available.
func (ds *Dataset) WindowDates(start, end time.Time, requireForwardWindow int) []time.Time {
	dates := make([]time.Time, 0)
	for _, date := range ds.Dates {
		if date.Before(start) || date.After(end) {
			continue
		}
		if _, ok := ds.NextDate(date, requireForwardWindow); !ok {
			continue
		}
		dates = append(dates, date)
	}
	return dates
}
