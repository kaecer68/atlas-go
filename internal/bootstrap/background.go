package bootstrap

import (
	"encoding/csv"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

func getLatestReplayDate(csvPath string) (time.Time, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	var latest time.Time
	_, _ = reader.Read()
	for {
		row, err := reader.Read()
		if err != nil {
			break
		}
		if len(row) == 0 {
			continue
		}
		d, err := time.Parse("2006-01-02", strings.TrimSpace(row[0]))
		if err != nil {
			continue
		}
		if d.After(latest) {
			latest = d
		}
	}
	if latest.IsZero() {
		return time.Time{}, errors.New("no valid dates found")
	}
	return latest, nil
}

func recoverPanic(taskName string) {
	if r := recover(); r != nil {
		logging.Error("bootstrap", "goroutine_panic_recovered",
			"task", taskName, "panic", r)
	}
}
