package ledger

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

const (
	defaultMaxFileSizeMB = 100
	defaultRetentionDays = 730
	archiveSubdir        = "archive"
)

// Archiver handles automated archival and cleanup of ledger files.
type Archiver struct {
	baseDir       string
	maxFileSize   int64
	retentionDays int
}

// NewArchiver creates a new ledger archiver with default settings.
func NewArchiver(baseDir string) *Archiver {
	return &Archiver{
		baseDir:       baseDir,
		maxFileSize:   defaultMaxFileSizeMB * 1024 * 1024,
		retentionDays: defaultRetentionDays,
	}
}

// ArchivableFiles returns the list of ledger files that should be archived.
func (a *Archiver) ArchivableFiles() []string {
	return []string{
		"experiments.jsonl",
		"human_interventions.jsonl",
		"spawn_records.jsonl",
		"alerts.jsonl",
	}
}

// Run executes the archival process: archive oversized files, then cleanup old archives.
func (a *Archiver) Run() error {
	if err := os.MkdirAll(a.archiveDir(), 0o755); err != nil {
		return fmt.Errorf("mkdir archive: %w", err)
	}

	for _, filename := range a.ArchivableFiles() {
		if err := a.archiveIfNeeded(filename); err != nil {
			logging.Warn("ledger_archiver", "archive_failed", "file", filename, "error", err.Error())
		}
	}

	if err := a.cleanupOldArchives(); err != nil {
		logging.Warn("ledger_archiver", "cleanup_failed", "error", err.Error())
	}

	return nil
}

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

	timestamp := time.Now().Format("20060102-150405")
	baseName := strings.TrimSuffix(filename, ".jsonl")
	archiveName := fmt.Sprintf("%s-%s.jsonl.gz", baseName, timestamp)
	archivePath := filepath.Join(a.archiveDir(), archiveName)

	if err := a.compressFile(path, archivePath); err != nil {
		return fmt.Errorf("compress file: %w", err)
	}

	if err := os.Truncate(path, 0); err != nil {
		return fmt.Errorf("truncate file: %w", err)
	}

	logging.Info("ledger_archiver", "archived",
		"file", filename,
		"archive", archiveName,
		"original_size_mb", info.Size()/1024/1024,
	)
	return nil
}

func (a *Archiver) compressFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	defer dstFile.Close()

	gzWriter := gzip.NewWriter(dstFile)
	defer gzWriter.Close()

	if _, err := io.Copy(gzWriter, srcFile); err != nil {
		return fmt.Errorf("compress data: %w", err)
	}

	return nil
}

func (a *Archiver) cleanupOldArchives() error {
	entries, err := os.ReadDir(a.archiveDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read archive dir: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -a.retentionDays)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(a.archiveDir(), entry.Name())
			if err := os.Remove(path); err != nil {
				logging.Warn("ledger_archiver", "cleanup_failed",
					"file", entry.Name(),
					"error", err.Error(),
				)
			} else {
				logging.Info("ledger_archiver", "cleaned_up",
					"file", entry.Name(),
					"age_days", int(time.Since(info.ModTime()).Hours()/24),
				)
			}
		}
	}

	return nil
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
	}, nil
}
