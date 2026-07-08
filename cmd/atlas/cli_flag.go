package main

import (
	"fmt"
	"strings"
)

// parseTriStateFlag parses CLI override flags that accept boolean values
// plus an empty "no-override" sentinel.
//
// Accepts (case-insensitive, whitespace-trimmed):
//
//	""                → (false, false, nil) — caller treats as no-override
//	"true"/"1"/"yes"  → (true,  true,  nil)
//	"false"/"0"/"no" → (false, true,  nil)
//	anything else     → (false, false, err) — caller fails fast
//
// The (explicit=false, err=nil) case for empty input is critical: it
// distinguishes "user didn't pass the flag" from "user explicitly said
// false". Without it, the flag could not be a non-mutating override
// (PR #828 chose String over Bool for exactly this reason).
func parseTriStateFlag(s string) (parsed bool, explicit bool, err error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return false, false, nil
	case "true", "1", "yes":
		return true, true, nil
	case "false", "0", "no":
		return false, true, nil
	default:
		return false, false, fmt.Errorf("expected true|false|1|0|yes|no, got %q", s)
	}
}
