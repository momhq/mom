package drafter

import (
	"regexp"
	"strings"
)

// secretPattern is one rule in the hard filter library: a regex plus
// a stable category name. When the regex matches, the matched
// substring is replaced with "[REDACTED]" in the persisted memory and
// the category counter is bumped on filter_audit. The matched
// substance is NEVER stored anywhere — neither in the memory row nor
// in the audit row.
type secretPattern struct {
	category string
	re       *regexp.Regexp
}

// hardFilterPatterns is the v0.30 secret-detection library. ADR 0014
// names the categories MOM ships with; see lessons/PRD for the
// rationale on each.
//
// Order matters: more-specific patterns run before generic ones so
// a JWT or PEM block isn't half-matched by env_assignment first.
var hardFilterPatterns = []secretPattern{
	// AWS access key id — fixed prefix + 16 base32-ish chars.
	{
		category: "aws_key",
		re:       regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	},
	// GitHub personal access tokens. Two shapes ship today:
	//   ghp_*  / gho_*  / ghu_*  / ghs_* / ghr_* — classic, 36 alnum chars
	//   github_pat_<id>_<secret>                 — fine-grained, longer
	{
		category: "github_pat",
		re:       regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`),
	},
	{
		category: "github_pat",
		re:       regexp.MustCompile(`github_pat_[A-Za-z0-9_]{50,}`),
	},
	// Stripe live keys (sk_live_ / pk_live_). 24+ alnum chars after
	// the prefix.
	{
		category: "stripe_key",
		re:       regexp.MustCompile(`(sk|pk)_live_[A-Za-z0-9]{24,}`),
	},
	// PEM private key blocks. Match across newlines (?s) so the body
	// of the block is captured. Non-greedy on the body so only one
	// block is grabbed at a time.
	{
		category: "pem_private_key",
		re:       regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
	},
	// JWT — three base64url-encoded parts joined by dots. Header
	// always begins "eyJ..." (URL-safe base64 of `{"`); we require
	// that prefix to avoid false-positive matches on dotted
	// identifiers.
	{
		category: "jwt",
		re:       regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`),
	},
	// Env-style assignment: a name like API_KEY / SECRET / TOKEN /
	// PASSWORD followed by =, :, or "= " then a non-whitespace value.
	// The whole match is redacted; "API_KEY=…" surrounding context is
	// preserved by line/word boundaries on either side.
	//
	// We redact ONLY the value (the part after the operator), keeping
	// the variable name visible. That's intentional: the variable
	// name is what makes the surrounding text useful as a memory ("I
	// was confused why my AWS_ACCESS_KEY_ID wasn't working") while
	// the value is what must not be retained.
	{
		category: "env_assignment",
		re:       regexp.MustCompile(`(?i)(?:API[_-]?KEY|SECRET|TOKEN|PASSWORD|PASSWD|AUTH[_-]?KEY|ACCESS[_-]?KEY|PRIVATE[_-]?KEY)[\s]*[:=][\s]*"?([^\s"']+)"?`),
	},
}

// redactSecrets walks the input through every hard-filter pattern. It
// returns the redacted text (with each match replaced by "[REDACTED]")
// and the deduplicated list of categories that fired. The list is the
// signal Drafter passes to filter_audit — bump one counter per
// distinct category, even if the same category matched multiple
// times.
//
// Patterns run in declaration order; later patterns see the output of
// earlier ones, so once a region is redacted it cannot be matched by
// a less-specific later rule.
func redactSecrets(text string) (string, []string) {
	out := text
	categories := map[string]struct{}{}
	for _, p := range hardFilterPatterns {
		var (
			matched bool
			redacted string
		)
		if p.category == "env_assignment" {
			// For env-style assignments we only redact the VALUE
			// capture group, not the whole match. The variable name
			// stays so the surrounding prose remains useful.
			redacted = p.re.ReplaceAllStringFunc(out, func(s string) string {
				m := p.re.FindStringSubmatch(s)
				if len(m) < 2 || m[1] == "" {
					return s
				}
				matched = true
				// Replace just the value within the matched span.
				start := strings.Index(s, m[1])
				if start < 0 {
					return s
				}
				return s[:start] + "[REDACTED]" + s[start+len(m[1]):]
			})
		} else {
			locs := p.re.FindAllStringIndex(out, -1)
			matched = len(locs) > 0
			redacted = p.re.ReplaceAllString(out, "[REDACTED]")
		}
		if matched {
			categories[p.category] = struct{}{}
			out = redacted
		}
	}
	if len(categories) == 0 {
		return out, nil
	}
	cats := make([]string, 0, len(categories))
	for c := range categories {
		cats = append(cats, c)
	}
	return out, cats
}

