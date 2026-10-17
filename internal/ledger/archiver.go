// Automated archival of append-only JSONL ledger files (FU-20261012-05).
//
// Two policies, both bounded per run:
//   - retention: a ledger file that nothing has written to for
//     RetentionDays is moved into <baseDir>/archive as a verified .gz;
//   - size: a live append-only file at/above MaxFileSizeMB is copied into the
//     archive and truncated in place.
//
// Archival never destroys data: the archive is written and its line count is
// verified against the source before the live path is truncated or removed, and
// archives are kept forever unless ArchivePruneDays is explicitly set.
//
// Retention uses mtime durations only — no calendar-date arithmetic — so the
// host time zone cannot shift a boundary (see
// cmd/atlas/replay_backfill_window.go for the calendar-date convention).
package ledger

import (
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

const (
	defaultMaxFileSizeMB   = 100
	defaultRetentionDays   = 365
	defaultMaxFilesPerRun  = 50
	defaultArchivePruneDay = 0 // 0 = archives are never deleted (data is only moved, never destroyed)
	archiveSubdir          = "archive"
)

// DefaultArchiveGlobs lists the baseDir-relative patterns scanned for
// retention-based archiving. They cover every append-only JSONL file the
// ledger/monitoring stores write under data/state (2026-10-16 audit: the
// previous fixed 4-name list missed recommendation_outcomes.jsonl — by far the
// largest grower — and pointed at alerts.jsonl, which lives in alerts/).
var DefaultArchiveGlobs = []string{"*.jsonl", "alerts/*.jsonl"}

// ArchivePolicy is the operator-facing retention policy (wired from
// config.Config, i.e. ATLAS_LEDGER_ARCHIVE_* env vars). A zero field keeps the
// archiver's current setting, so callers can override selectively.
type ArchivePolicy struct {
	// RetentionDays archives (moves into <baseDir>/archive) ledger files whose
	// mtime is older than this many days. 0 keeps the default.
	RetentionDays int
	// MaxFileSizeMB rotates (gzip copy + truncate) an append-only file once it
	// reaches this size. 0 keeps the default.
	MaxFileSizeMB int64
	// MaxFilesPerRun bounds how many files one run touches, so a scheduled run
	// never turns into an unbounded IO burst. 0 keeps the default.
	MaxFilesPerRun int
	// ArchivePruneDays deletes archives older than this many days. 0 keeps the
	// default, which is "never delete": archival only moves data.
	ArchivePruneDays int
	// Globs overrides the scanned patterns (baseDir-relative). 0-length keeps
	// the default.
	Globs []string
	// Now injects a clock (tests). nil keeps time.Now.
	Now func() time.Time
}

// DefaultArchivePolicy returns the built-in defaults.
func DefaultArchivePolicy() ArchivePolicy {
	return ArchivePolicy{
		RetentionDays:    defaultRetentionDays,
		MaxFileSizeMB:    defaultMaxFileSizeMB,
		MaxFilesPerRun:   defaultMaxFilesPerRun,
		ArchivePruneDays: defaultArchivePruneDay,
		Globs:            append([]string(nil), DefaultArchiveGlobs...),
	}
}

// ArchivedFile records one archival action.
type ArchivedFile struct {
	// Path is the baseDir-relative live path that was archived.
	Path string
	// Archive is the baseDir-relative archive path that now holds the data.
	Archive string
	// Size is the number of bytes moved/rotated out of the live file.
	Size int64
	// Lines is the verified line count (live == archive).
	Lines int64
	// Mode is "retention" (file moved) or "size" (file rotated in place).
	Mode string
}

// RunReport summarizes one archival run.
type RunReport struct {
	Scanned    int
	Archived   []ArchivedFile
	Rotated    []ArchivedFile
	Skipped    int
	Pruned     int
	BytesFreed int64
	Errors     []string
}

// Archiver handles automated archival of ledger files.
type Archiver struct {
	baseDir          string
	maxFileSize      int64
	retentionDays    int
	maxFilesPerRun   int
	archivePruneDays int
	globs            []string
	now              func() time.Time
	ctx              context.Context
}

// NewArchiver creates a new ledger archiver with default settings.
func NewArchiver(baseDir string) *Archiver {
	return &Archiver{
		baseDir:          baseDir,
		maxFileSize:      defaultMaxFileSizeMB * 1024 * 1024,
		retentionDays:    defaultRetentionDays,
		maxFilesPerRun:   defaultMaxFilesPerRun,
		archivePruneDays: defaultArchivePruneDay,
		globs:            append([]string(nil), DefaultArchiveGlobs...),
		now:              time.Now,
	}
}

// WithPolicy applies a retention policy on top of the defaults.
func (a *Archiver) WithPolicy(p ArchivePolicy) *Archiver {
	if p.RetentionDays > 0 {
		a.retentionDays = p.RetentionDays
	}
	if p.MaxFileSizeMB > 0 {
		a.maxFileSize = p.MaxFileSizeMB * 1024 * 1024
	}
	if p.MaxFilesPerRun > 0 {
		a.maxFilesPerRun = p.MaxFilesPerRun
	}
	if p.ArchivePruneDays > 0 {
		a.archivePruneDays = p.ArchivePruneDays
	}
	if len(p.Globs) > 0 {
		a.globs = append([]string(nil), p.Globs...)
	}
	if p.Now != nil {
		a.now = p.Now
	}
	return a
}

// WithContext makes a run stop between files once ctx is done, so the
// background task's own timeout bounds the wall-clock time of one pass (the
// per-run file budget bounds the work; the context bounds the duration).
func (a *Archiver) WithContext(ctx context.Context) *Archiver {
	a.ctx = ctx
	return a
}

// ArchivableFiles returns the append-only ledger files that may be rotated by
// size. Paths are baseDir-relative; every entry is written by a store in this
// repository (see the 2026-10-16 audit in FU-20261012-05).
func (a *Archiver) ArchivableFiles() []string {
	return []string{
		"recommendation_outcomes.jsonl", // ledger.Store.RecordOutcomes — largest grower
		"darwinian_history.jsonl",       // orchestrator/monitoring weight history
		"metrics.jsonl",                 // monitoring.MetricsStore
		"quotes.jsonl",                  // ledger JSONL quote store
		"experiments.jsonl",             // ledger experiment store
		"human_interventions.jsonl",     // ledger human intervention log
		"spawn_records.jsonl",           // ledger spawn log
		"alerts/alerts.jsonl",           // monitoring alert store (lives in alerts/, not top level)
	}
}

// Run executes the archival process: retention (move), size rotation, then
// optional archive pruning. Per-file failures are logged and reported; they do
// not abort the remaining files.
func (a *Archiver) Run() error {
	_, err := a.RunWithReport()
	return err
}

// RunWithReport runs one archival pass and returns what it did.
func (a *Archiver) RunWithReport() (RunReport, error) {
	report := RunReport{}
	if err := os.MkdirAll(a.archiveDir(), 0o755); err != nil {
		return report, fmt.Errorf("mkdir archive: %w", err)
	}

	candidates := a.candidates()
	report.Scanned = len(candidates)
	now := a.clock()
	cutoff := now.AddDate(0, 0, -a.retentionDays)
	budget := a.budget()

	for _, rel := range candidates {
		if err := a.ctxErr(); err != nil {
			report.Errors = append(report.Errors, err.Error())
			return report, err
		}
		info, err := os.Stat(filepath.Join(a.baseDir, rel))
		if err != nil {
			report.Skipped++
			continue
		}
		if info.Size() == 0 {
			// An empty file holds nothing to preserve; leave it alone.
			report.Skipped++
			continue
		}
		if budget <= 0 {
			report.Skipped++
			continue
		}
		switch {
		case info.ModTime().Before(cutoff):
			// Retention: move the whole file into the archive. Never truncate
			// here — a stale file is dead weight, and moving keeps every byte.
			archived, moveErr := a.archiveAndRemove(rel, now)
			if moveErr != nil {
				report.Errors = append(report.Errors, moveErr.Error())
				logging.Warn("ledger_archiver", "retention_archive_failed", "file", rel, "error", moveErr.Error())
				continue
			}
			budget--
			report.Archived = append(report.Archived, archived)
			report.BytesFreed += archived.Size
		case info.Size() >= a.maxFileSize:
			// Size rotation: the file is still live, so copy it into the
			// archive and truncate in place (never delete the live path).
			rotated, rotErr := a.rotateBySize(rel, now)
			if rotErr != nil {
				report.Errors = append(report.Errors, rotErr.Error())
				logging.Warn("ledger_archiver", "size_rotation_failed", "file", rel, "error", rotErr.Error())
				continue
			}
			budget--
			report.Rotated = append(report.Rotated, rotated)
			report.BytesFreed += rotated.Size
		default:
			report.Skipped++
		}
	}

	pruned, pruneErr := a.pruneOldArchives(now)
	report.Pruned = pruned
	if pruneErr != nil {
		report.Errors = append(report.Errors, pruneErr.Error())
	}
	return report, nil
}

// candidates returns the baseDir-relative files covered by the current policy:
// the union of the glob matches and the known ledger files, sorted for a
// deterministic run order.
func (a *Archiver) candidates() []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(rel string) {
		rel = filepath.Clean(rel)
		if rel == "" || strings.HasPrefix(rel, "..") {
			return
		}
		if rel == archiveSubdir || strings.HasPrefix(rel, archiveSubdir+string(filepath.Separator)) {
			return
		}
		if _, ok := seen[rel]; ok {
			return
		}
		seen[rel] = struct{}{}
		out = append(out, rel)
	}

	for _, pattern := range a.patterns() {
		matches, err := filepath.Glob(filepath.Join(a.baseDir, pattern))
		if err != nil {
			logging.Warn("ledger_archiver", "glob_invalid", "pattern", pattern, "error", err.Error())
			continue
		}
		for _, m := range matches {
			info, statErr := os.Stat(m)
			if statErr != nil || info.IsDir() {
				continue
			}
			rel, relErr := filepath.Rel(a.baseDir, m)
			if relErr != nil {
				continue
			}
			add(rel)
		}
	}
	for _, rel := range a.ArchivableFiles() {
		if _, err := os.Stat(filepath.Join(a.baseDir, rel)); err != nil {
			continue
		}
		add(rel)
	}
	slices.Sort(out)
	return out
}

// archiveAndRemove moves a stale file into the archive: rename to a staging
// name (atomic, so new writers open a fresh live file), gzip it, verify that the
// archive holds exactly as many lines as the source, then drop the staging copy.
// Any failure restores the live path.
func (a *Archiver) archiveAndRemove(rel string, now time.Time) (ArchivedFile, error) {
	live := filepath.Join(a.baseDir, rel)
	info, err := os.Stat(live)
	if err != nil {
		return ArchivedFile{}, fmt.Errorf("stat %s: %w", rel, err)
	}
	staging := filepath.Join(a.archiveDir(), fmt.Sprintf(".staging-%s-%d", sanitizeRel(rel), now.UnixNano()))
	if err := os.Rename(live, staging); err != nil {
		return ArchivedFile{}, fmt.Errorf("staging rename %s: %w", rel, err)
	}

	archivePath := a.nextArchivePath(rel, now)
	lines, err := a.compressVerified(staging, archivePath)
	if err != nil {
		if restoreErr := os.Rename(staging, live); restoreErr != nil {
			return ArchivedFile{}, fmt.Errorf("compress %s: %w (restore failed: %w)", rel, err, restoreErr)
		}
		return ArchivedFile{}, fmt.Errorf("compress %s: %w", rel, err)
	}
	if err := os.Remove(staging); err != nil {
		logging.Warn("ledger_archiver", "staging_remove_failed", "file", rel, "error", err.Error())
	}

	archiveRel, _ := filepath.Rel(a.baseDir, archivePath)
	logging.Info("ledger_archiver", "archived",
		"file", rel,
		"archive", archiveRel,
		"mode", "retention",
		"original_size_mb", info.Size()/1024/1024,
		"lines", lines,
	)
	return ArchivedFile{Path: rel, Archive: archiveRel, Size: info.Size(), Lines: lines, Mode: "retention"}, nil
}

// rotateBySize archives an oversized live file and truncates it in place. The
// copy is repeated while the file keeps growing so a concurrent append is not
// silently dropped between copy and truncate.
func (a *Archiver) rotateBySize(rel string, now time.Time) (ArchivedFile, error) {
	live := filepath.Join(a.baseDir, rel)
	archivePath := a.nextArchivePath(rel, now)
	var (
		lines   int64
		size    int64
		stable  bool
		lastErr error
	)
	for attempt := 0; attempt < 3; attempt++ {
		before, err := os.Stat(live)
		if err != nil {
			return ArchivedFile{}, fmt.Errorf("stat %s: %w", rel, err)
		}
		lines, lastErr = a.compressVerified(live, archivePath)
		if lastErr != nil {
			return ArchivedFile{}, fmt.Errorf("compress %s: %w", rel, lastErr)
		}
		after, err := os.Stat(live)
		if err != nil {
			return ArchivedFile{}, fmt.Errorf("stat %s: %w", rel, err)
		}
		size = before.Size()
		if after.Size() == before.Size() {
			stable = true
			break
		}
	}
	if !stable {
		// The file is still being appended to at a rate that outpaces the
		// copy; keep the archive (no data lost) and retry on the next run.
		return ArchivedFile{}, fmt.Errorf("rotate %s: file still growing, deferred (archive kept at %s)", rel, archivePath)
	}
	if err := os.Truncate(live, 0); err != nil {
		return ArchivedFile{}, fmt.Errorf("truncate %s: %w", rel, err)
	}
	archiveRel, _ := filepath.Rel(a.baseDir, archivePath)
	logging.Info("ledger_archiver", "archived",
		"file", rel,
		"archive", archiveRel,
		"mode", "size",
		"original_size_mb", size/1024/1024,
		"lines", lines,
	)
	return ArchivedFile{Path: rel, Archive: archiveRel, Size: size, Lines: lines, Mode: "size"}, nil
}

// archiveIfNeeded rotates a single named file when it exceeds maxFileSize.
func (a *Archiver) archiveIfNeeded(filename string) error {
	path := filepath.Join(a.baseDir, filename)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat file: %w", err)
	}
	if info.Size() < a.maxFileSize {
		return nil
	}
	_, err = a.rotateBySize(filename, a.clock())
	return err
}

// nextArchivePath returns a not-yet-used archive path for rel.
func (a *Archiver) nextArchivePath(rel string, now time.Time) string {
	baseName := sanitizeRel(strings.TrimSuffix(rel, ".jsonl"))
	stamp := now.Format("20060102-150405")
	candidate := filepath.Join(a.archiveDir(), fmt.Sprintf("%s-%s.jsonl.gz", baseName, stamp))
	for i := 1; ; i++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
		candidate = filepath.Join(a.archiveDir(), fmt.Sprintf("%s-%s-%d.jsonl.gz", baseName, stamp, i))
	}
}

// compressVerified gzips src into dst and verifies that the archive holds the
// same number of lines as the source. The archive only appears under its final
// name after verification succeeds, so a partial file is never mistaken for a
// complete archive. Returns the verified line count.
func (a *Archiver) compressVerified(src, dst string) (int64, error) {
	tmp := dst + ".tmp"
	srcLines, err := a.compressFileCount(src, tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}
	archivedLines, err := countGzipLines(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return 0, fmt.Errorf("verify archive: %w", err)
	}
	if archivedLines != srcLines {
		_ = os.Remove(tmp)
		return 0, fmt.Errorf("verify archive: line mismatch (source=%d archive=%d)", srcLines, archivedLines)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return 0, fmt.Errorf("finalize archive: %w", err)
	}
	return srcLines, nil
}

// compressFile gzips src into dst without verification (kept for callers and
// tests that only need the byte copy).
func (a *Archiver) compressFile(src, dst string) error {
	_, err := a.compressFileCount(src, dst)
	return err
}

// compressFileCount gzips src into dst and returns the number of lines written.
func (a *Archiver) compressFileCount(src, dst string) (int64, error) {
	srcFile, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.Create(dst)
	if err != nil {
		return 0, fmt.Errorf("create archive: %w", err)
	}
	defer func() { _ = dstFile.Close() }()

	gzWriter := gzip.NewWriter(dstFile)
	counting := &lineCountingWriter{}
	if _, err := io.Copy(io.MultiWriter(gzWriter, counting), srcFile); err != nil {
		_ = gzWriter.Close()
		return 0, fmt.Errorf("compress data: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return 0, fmt.Errorf("close gzip writer: %w", err)
	}
	if err := dstFile.Sync(); err != nil {
		return 0, fmt.Errorf("sync archive: %w", err)
	}
	return counting.lines, nil
}

// ctxErr reports whether the caller's context has been canceled.
func (a *Archiver) ctxErr() error {
	if a.ctx == nil {
		return nil
	}
	select {
	case <-a.ctx.Done():
		return fmt.Errorf("archive run canceled: %w", a.ctx.Err())
	default:
		return nil
	}
}

// clock returns the archiver clock, tolerating a zero-value struct.
func (a *Archiver) clock() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

// budget returns the per-run file budget, tolerating a zero-value struct.
func (a *Archiver) budget() int {
	if a.maxFilesPerRun > 0 {
		return a.maxFilesPerRun
	}
	return defaultMaxFilesPerRun
}

// patterns returns the scanned globs, tolerating a zero-value struct.
func (a *Archiver) patterns() []string {
	if len(a.globs) > 0 {
		return a.globs
	}
	return DefaultArchiveGlobs
}

// cleanupOldArchives deletes archives older than the prune window. The default
// window is 0 = keep everything, because archival must not destroy data; pruning
// is an explicit operator decision (ATLAS_LEDGER_ARCHIVE_PRUNE_DAYS).
func (a *Archiver) cleanupOldArchives() error {
	_, err := a.pruneOldArchives(a.clock())
	return err
}

func (a *Archiver) pruneOldArchives(now time.Time) (int, error) {
	if a.archivePruneDays <= 0 {
		return 0, nil
	}
	entries, err := os.ReadDir(a.archiveDir())
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read archive dir: %w", err)
	}
	cutoff := now.AddDate(0, 0, -a.archivePruneDays)
	pruned := 0
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(a.archiveDir(), entry.Name())
			if err := os.Remove(path); err != nil {
				logging.Warn("ledger_archiver", "cleanup_failed", "file", entry.Name(), "error", err.Error())
				continue
			}
			pruned++
			logging.Info("ledger_archiver", "cleaned_up",
				"file", entry.Name(),
				"age_days", int(now.Sub(info.ModTime()).Hours()/24),
			)
		}
	}
	return pruned, nil
}

func (a *Archiver) archiveDir() string {
	return filepath.Join(a.baseDir, archiveSubdir)
}

// Stats returns archival statistics for monitoring.
func (a *Archiver) Stats() (map[string]any, error) {
	entries, err := os.ReadDir(a.archiveDir())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{
				"archive_count":    0,
				"total_size_mb":    0,
				"retention_days":   a.retentionDays,
				"max_file_size_mb": a.maxFileSize / 1024 / 1024,
				"prune_days":       a.archivePruneDays,
			}, nil
		}
		return nil, fmt.Errorf("read archive dir: %w", err)
	}

	var totalSize int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		totalSize += info.Size()
	}

	return map[string]any{
		"archive_count":    len(entries),
		"total_size_mb":    totalSize / 1024 / 1024,
		"retention_days":   a.retentionDays,
		"max_file_size_mb": a.maxFileSize / 1024 / 1024,
		"prune_days":       a.archivePruneDays,
	}, nil
}

// sanitizeRel turns a baseDir-relative path into a flat archive file-name stem.
func sanitizeRel(rel string) string {
	return strings.NewReplacer("/", "-", string(filepath.Separator), "-").Replace(rel)
}

// lineCountingWriter counts newline-terminated records while copying.
type lineCountingWriter struct {
	lines int64
}

func (w *lineCountingWriter) Write(p []byte) (int, error) {
	w.lines += int64(strings.Count(string(p), "\n"))
	return len(p), nil
}

// countGzipLines returns the number of newline-terminated records in a gzip file.
func countGzipLines(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	defer func() { _ = gz.Close() }()

	return countLines(gz)
}

// countLines counts newline-terminated records without a scanner line limit.
func countLines(r io.Reader) (int64, error) {
	br := bufio.NewReader(r)
	var lines int64
	for {
		_, err := br.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return lines, nil
			}
			return lines, err
		}
		lines++
	}
}
