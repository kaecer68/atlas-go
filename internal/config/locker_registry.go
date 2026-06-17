package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// fileLockers is the global singleton registry mapping lock file paths to FileLocker instances.
// Reusing the same FileLocker for the same path ensures process-level lock serialization
// (gofrs/flock's flock file lock works across processes; our sync.Mutex handles in-process reentrancy).
var fileLockers sync.Map

// GetFileLocker returns the singleton FileLocker for the given path.
// The actual lock file is path + ".lock" — colocated and simple.
func GetFileLocker(path string) *FileLocker {
	v, _ := fileLockers.LoadOrStore(path, NewFileLocker(path+".lock"))
	return v.(*FileLocker)
}

func LockedWriteFileWithRollback(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("locked write: create parent dir: %w", err)
		}
	}

	locker := GetFileLocker(path)
	unlock := locker.Lock()
	defer unlock()

	tmpPath := path + ".tmp"
	bakPath := path + ".bak"

	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("locked write: write temp file: %w", err)
	}

	f, err := os.OpenFile(tmpPath, os.O_RDONLY, 0)
	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("locked write: open temp file for sync: %w", err)
	}
	_ = f.Sync()
	_ = f.Close()

	if _, statErr := os.Stat(path); statErr == nil {
		if err := os.Rename(path, bakPath); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("locked write: backup existing file: %w", err)
		}
	}

	if err := os.Rename(tmpPath, path); err != nil {
		if _, bakErr := os.Stat(bakPath); bakErr == nil {
			_ = os.Rename(bakPath, path)
		}
		return fmt.Errorf("locked write: promote temp file: %w", err)
	}

	_ = os.Remove(bakPath)
	return nil
}
