package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"

	"github.com/kaecer68/atlas-go/internal/replay"
)

// tempFileSeq 讓同一 process 內多個暫存檔名互不衝突（跨 process 由 pid 區分）。
var tempFileSeq atomic.Uint64

// ImportTWOpenDataCSVToJSONL converts the TWSE open-data CSV into the JSONL that
// the factor engine consumes.
//
// The target must be replaced **atomically** (FU-20260927-07): the JSONL is the
// production input of the composition stage
// (internal/orchestrator/composition.go → portfolio.HistoricalPrices.LoadFromExtendedJSONL)
// and conversion runs in a different goroutine (and at a different moment) than
// the read. The previous implementation called `os.Create(targetPath)` — which
// truncates immediately — and then encoded row by row, so a concurrent reader
// could observe a 0-byte or half-written file. Measured on the pre-fix code with
// 21,220 samples taken during one conversion: 7 samples saw 0 bytes and 17,300
// saw a truncated file.
//
// The replacement is "sibling temp file → fsync → rename". rename(2) inside one
// file system is atomic, so a reader only ever sees the complete old file or the
// complete new file. No failure path leaves a temp or partial file behind.
//
// # Row-order contract (FU-20261016-31)
//
// Rows are written in a total order: **date ascending, then symbol ascending**
// (byte order). The order is part of the file format, not an implementation
// artifact:
//
//   - `internal/replay.GetLatestDate` reads only the **last** non-empty line of a
//     `.jsonl` file and reports its `date`, so "the last line carries the newest
//     date" must hold;
//   - byte-level comparison of two conversions of the same CSV — replay diffs,
//     integrity checksums such as `internal/replay.ComputeChecksum` — must not
//     see noise that no data change can explain.
//
// Reading the dataset's inner `map[string]domain.DailyBar` directly (as the
// pre-fix code did) walks it in Go's randomized map order, so the same CSV
// produced a different file on every conversion: 10 consecutive conversions of
// `samples/replay/twse_stock_day_all_sample.csv` (53 rows) produced 10 distinct
// sha256 digests, with the first line already differing between two runs.
func ImportTWOpenDataCSVToJSONL(sourcePath, targetPath string) error {
	ds, err := replay.LoadTWSEOpenDataCSV(sourcePath)
	if err != nil {
		return err
	}

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := createSiblingTempFile(targetPath)
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			// Every failure path must leave the directory exactly as it was.
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	enc := json.NewEncoder(tmp)
	for _, date := range ds.Dates {
		// The outer loop is the loader's already-ascending date series; the inner
		// map is walked in sorted key order so that one CSV always yields the same
		// bytes (see the row-order contract above).
		day := ds.ByDate[date.Format("2006-01-02")]
		for _, symbol := range slices.Sorted(maps.Keys(day)) {
			if err := enc.Encode(day[symbol]); err != nil {
				return err
			}
		}
	}
	// fsync before rename: without it the rename may be durable while the data
	// is not, and a crash would leave a 0-byte target file.
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return err
	}
	committed = true
	return nil
}

// createSiblingTempFile creates a uniquely named temporary file in the same
// directory as targetPath (rename is only atomic inside one file system).
//
// It deliberately does not use os.CreateTemp for the new-file case: os.CreateTemp
// always creates the file with mode 0600, which would silently tighten the 0644
// that os.Create (0666 & ~umask) produced before. Two cases:
//
//   - the target already exists ⇒ create the temp file and chmod it to the
//     target's current permissions, so a conversion never changes file modes;
//   - the target does not exist yet ⇒ request mode 0666 at creation time and let
//     the kernel apply the umask, which is exactly os.Create's semantics.
func createSiblingTempFile(targetPath string) (*os.File, error) {
	dir := filepath.Dir(targetPath)
	base := filepath.Base(targetPath)

	if info, err := os.Stat(targetPath); err == nil {
		f, err := os.CreateTemp(dir, base+".tmp-*")
		if err != nil {
			return nil, err
		}
		if err := f.Chmod(info.Mode().Perm()); err != nil {
			_ = f.Close()
			_ = os.Remove(f.Name())
			return nil, err
		}
		return f, nil
	}

	for attempt := 0; attempt < 100; attempt++ {
		name := filepath.Join(dir, fmt.Sprintf("%s.tmp-%d-%d", base, os.Getpid(), tempFileSeq.Add(1)))
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("importer: cannot create a temporary file next to %s", targetPath)
}
