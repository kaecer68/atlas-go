package eventquality

import (
	"regexp"
	"strings"
)

var (
	htmlTagRe  = regexp.MustCompile(`<[^>]*>`)
	allDigitRe = regexp.MustCompile(`^\d+$`)
)

type SanitizeResult struct {
	Clean    string
	Rejected bool
	Reasons  []string
}

func SanitizeTitle(title string) SanitizeResult {
	var reasons []string

	if htmlTagRe.MatchString(title) {
		reasons = append(reasons, "contains HTML tags")
	}

	plain := htmlTagRe.ReplaceAllString(title, "")
	if len([]rune(plain)) > 200 {
		reasons = append(reasons, "exceeds 200 characters")
	}

	trimmed := strings.TrimSpace(plain)
	if allDigitRe.MatchString(trimmed) {
		reasons = append(reasons, "title is all digits")
	}

	if len(reasons) > 0 {
		return SanitizeResult{Clean: plain, Rejected: true, Reasons: reasons}
	}
	return SanitizeResult{Clean: plain, Rejected: false}
}
