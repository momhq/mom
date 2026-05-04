package store

import (
	"strings"
	"unicode"
)

// NormalizeTagName returns the canonical form of a tag name. Tag
// identity in v0.30 is convention-driven kebab-case; this helper
// canonicalises caller-supplied tag strings so that variations of the
// "same" intent collapse to one tag row.
//
// Rules (the "T2" model — see ADR 0010):
//   - Lowercase (Unicode-aware)
//   - Trim outer whitespace
//   - Replace any run of non-alphanumeric characters (including
//     whitespace, underscores, periods, slashes, punctuation) with a
//     single hyphen
//   - Unicode letters and digits are preserved (alphanumeric is
//     unicode.IsLetter || unicode.IsDigit, NOT just [a-z0-9])
//   - Trim leading/trailing hyphens
//
// Examples:
//
//	"My Tag"   -> "my-tag"
//	"Foo_Bar"  -> "foo-bar"
//	"v0.30"    -> "v0-30"
//	"メモリ"    -> "メモリ"
//	"!!!"      -> ""
//
// An empty input (or input that reduces entirely to separators) returns
// the empty string. Callers that require a non-empty tag should check
// the result and reject before calling UpsertTag.
func NormalizeTagName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	wroteAny := false
	prevHyphen := true // suppress leading hyphens
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			wroteAny = true
			prevHyphen = false
			continue
		}
		if !prevHyphen {
			b.WriteRune('-')
			prevHyphen = true
		}
	}
	if !wroteAny {
		return ""
	}
	return strings.TrimRight(b.String(), "-")
}
