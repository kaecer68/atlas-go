package replay

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// VersionInfo holds replay dataset version metadata.
type VersionInfo struct {
	SourceFile string `json:"source_file"`
	Checksum   string `json:"checksum,omitempty"`
}

// ValidateReplayVersion checks the replay directory against its VERSION file.
func ValidateReplayVersion(replayDir string) error {
	versionPath := filepath.Join(replayDir, "VERSION")
	data, err := os.ReadFile(versionPath)
	if err != nil {
		return fmt.Errorf("read VERSION: %w", err)
	}

	sourceFile := strings.TrimSpace(string(data))
	if sourceFile == "" {
		return fmt.Errorf("VERSION file is empty")
	}

	sourcePath := filepath.Join(replayDir, sourceFile)
	if _, err := os.Stat(sourcePath); err != nil {
		return fmt.Errorf("source file missing: %s", sourcePath)
	}

	return nil
}

// ComputeChecksum calculates SHA256 of a file for integrity verification.
func ComputeChecksum(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// WriteVersion writes a VERSION file with source filename and optional checksum.
func WriteVersion(replayDir, sourceFile, checksum string) error {
	versionPath := filepath.Join(replayDir, "VERSION")
	content := sourceFile
	if checksum != "" {
		content = fmt.Sprintf("%s\nchecksum=%s", sourceFile, checksum)
	}
	return os.WriteFile(versionPath, []byte(content), 0o644)
}
