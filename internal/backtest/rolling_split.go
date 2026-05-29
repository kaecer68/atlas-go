package backtest

import (
	"fmt"
	"time"
)

// WindowRange describes a single train/valid/test split in a rolling window
// backtest. All dates are inclusive.
type WindowRange struct {
	TrainStart time.Time
	TrainEnd   time.Time
	ValidStart time.Time
	ValidEnd   time.Time
	TestStart  time.Time
	TestEnd    time.Time
}

// RollingWindowSplit produces an expanding-window time-series split following
// the SK-03 specification: training window expands forward by step_years each
// iteration, the validation window is fixed-length (valid_length_years), and
// the test window covers everything from valid_end+1 to test_end.
//
// Stop condition: iteration stops when valid_start.Year() > 2020.
type RollingWindowSplit struct {
	// FirstTrainEnd is the end date of the first training window.
	// Default: 2007-12-31.
	FirstTrainEnd time.Time

	// ValidLengthYears is the fixed validation window length in years.
	// Default: 2.
	ValidLengthYears int

	// TestEnd is the fixed end of the test window.
	// Default: 2022-04-30.
	TestEnd time.Time

	// StepYears is how many years to advance the training end per iteration.
	// Default: 1.
	StepYears int
}

// NewRollingWindowSplit returns a RollingWindowSplit with SK-03 defaults.
func NewRollingWindowSplit() RollingWindowSplit {
	return RollingWindowSplit{
		FirstTrainEnd:    time.Date(2007, 12, 31, 0, 0, 0, 0, time.UTC),
		ValidLengthYears: 2,
		TestEnd:          time.Date(2022, 4, 30, 0, 0, 0, 0, time.UTC),
		StepYears:        1,
	}
}

// Split returns the rolling window date ranges produced by this splitter.
// dataStart is the earliest date in the available dataset (training always
// begins from the data start — an expanding window).
//
// Returns an empty slice and an error if the parameters produce no windows
// (e.g., if valid_start would immediately exceed the stop year).
func (r RollingWindowSplit) Split(dataStart time.Time) ([]WindowRange, error) {
	if r.ValidLengthYears <= 0 {
		return nil, fmt.Errorf("rolling_split: ValidLengthYears must be positive, got %d", r.ValidLengthYears)
	}
	if r.StepYears <= 0 {
		return nil, fmt.Errorf("rolling_split: StepYears must be positive, got %d", r.StepYears)
	}
	if !r.TestEnd.After(r.FirstTrainEnd) {
		return nil, fmt.Errorf("rolling_split: TestEnd %s must be after FirstTrainEnd %s",
			r.TestEnd.Format("2006-01-02"), r.FirstTrainEnd.Format("2006-01-02"))
	}

	var windows []WindowRange
	stopYear := 2020

	for trainEnd := r.FirstTrainEnd; ; trainEnd = advanceYear(trainEnd, r.StepYears) {
		validStart := trainEnd.AddDate(0, 0, 1)
		// Stop when valid_start year exceeds stopYear.
		if validStart.Year() > stopYear {
			break
		}

		validEnd := validStart.AddDate(r.ValidLengthYears, 0, -1)

		// Test covers from validEnd+1 to testEnd.
		testStart := validEnd.AddDate(0, 0, 1)

		// Skip windows where the test window is empty or invalid.
		if !testStart.Before(r.TestEnd) && !testStart.Equal(r.TestEnd) {
			continue
		}
		// If testStart falls after testEnd, skip.
		if testStart.After(r.TestEnd) {
			continue
		}

		windows = append(windows, WindowRange{
			TrainStart: dataStart,
			TrainEnd:   trainEnd,
			ValidStart: validStart,
			ValidEnd:   validEnd,
			TestStart:  testStart,
			TestEnd:    r.TestEnd,
		})
	}

	if len(windows) == 0 {
		return nil, fmt.Errorf("rolling_split: no windows produced (first_train_end=%s, data_start=%s, stop_year=%d)",
			r.FirstTrainEnd.Format("2006-01-02"), dataStart.Format("2006-01-02"), stopYear)
	}

	return windows, nil
}

// advanceYear adds n years to t, handling leap-year edge cases by clamping
// the day to the last valid day of the target month.
func advanceYear(t time.Time, years int) time.Time {
	return t.AddDate(years, 0, 0)
}
