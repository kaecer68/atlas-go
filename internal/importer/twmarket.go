package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
		for _, bar := range ds.ByDate[date.Format("2006-01-02")] {
			if err := enc.Encode(bar); err != nil {
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
