// Package redact removes credential material from text before it reaches a
// sink an operator can read: a log line, an error string, an alert payload, a
// channel-health row, a CLI's stderr or a file under logs/.
//
// Core symbols:
//
//	Text(text, secrets...)  — the full rule set for upstream bodies and
//	                          diagnostic strings (URL query values, JSON secret
//	                          members, key/value shapes, literal secrets,
//	                          provider-shaped keys)
//	URL(raw)                — mask sensitive query values and the userinfo
//	                          password, keeping the URL diagnosable
//	Error(err, secrets...)  — wrap an error with a redacted message while
//	                          keeping Unwrap, so errors.Is/As still match
//
// Cautions:
//   - Redact the text, do not drop it: an operator must still be able to tell
//     "token rejected" from "quota exhausted" from "IP banned"
//   - Text that parses as JSON never reaches the key/value rule, so a message
//     that merely mentions "token" is left alone
//   - A caller-supplied secret shorter than 8 bytes is not replaced literally
//     (it would gut unrelated text); it is still dropped as a JSON member
//   - internal/marketdata's redactUpstreamText is a thin wrapper over Text;
//     new call sites should import this package directly
//
// Maturity: stable
package redact
