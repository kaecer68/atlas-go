package shared

import (
	"fmt"
	"regexp"
	"strings"
)

// idPattern matches safe path components: word chars, hyphens, periods, colons, underscores.
// Rejects: /, \, .., null bytes, empty strings, any path separator.
var idPattern = regexp.MustCompile(`^[\w\-.:]+$`)

// datePattern matches ISO dates: YYYY-MM-DD.
var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// ValidatePathComponent rejects path traversal characters (/ \ ..) and null bytes.
// Use for any user-supplied string that will be joined into a file path.
func ValidatePathComponent(s string) error {
	if s == "" {
		return fmt.Errorf("path component must not be empty")
	}
	if strings.Contains(s, "/") {
		return fmt.Errorf("path component contains forbidden character '/'")
	}
	if strings.Contains(s, "\\") {
		return fmt.Errorf("path component contains forbidden character '\\'")
	}
	if strings.Contains(s, "..") {
		return fmt.Errorf("path component contains forbidden sequence '..'")
	}
	if strings.ContainsRune(s, '\x00') {
		return fmt.Errorf("path component contains null byte")
	}
	return nil
}

// ValidateExperimentID validates an experiment_id for path construction.
// Must match ^[\w\-.:]+$ (same as idPattern).
func ValidateExperimentID(id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("invalid experiment_id: %q (must match [a-zA-Z0-9_\\-.:]+)", id)
	}
	if len(id) > 128 {
		return fmt.Errorf("experiment_id too long: %d chars (max 128)", len(id))
	}
	return nil
}

// ValidateSessionID validates a session_id for path construction.
// Must match ^[\w\-.:]+$ (same as idPattern).
func ValidateSessionID(id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("invalid session_id: %q (must match [a-zA-Z0-9_\\-.:]+)", id)
	}
	if len(id) > 128 {
		return fmt.Errorf("session_id too long: %d chars (max 128)", len(id))
	}
	return nil
}

// ValidateDateParam validates a date query parameter (YYYY-MM-DD).
func ValidateDateParam(date string) error {
	if !datePattern.MatchString(date) {
		return fmt.Errorf("invalid date: %q (must match YYYY-MM-DD)", date)
	}
	return nil
}
