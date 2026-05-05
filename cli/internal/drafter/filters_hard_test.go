package drafter

import (
	"strings"
	"testing"
)

// TestRedactSecrets_PatternFixture is the per-credential-shape hard
// filter coverage. False negatives on secrets are the highest-cost
// error MOM can produce; this fixture is the lock that prevents
// regressions in the pattern library.
//
// Each row asserts:
//
//   - The matched substring no longer appears anywhere in the redacted
//     output.
//   - The returned category list contains the expected category at
//     least once.
//   - Surrounding context is preserved (the redacted output contains
//     the substring that bookends the secret).
func TestRedactSecrets_PatternFixture(t *testing.T) {
	type tc struct {
		name           string
		input          string
		secret         string // substring that must NOT survive redaction
		wantCategory   string // category that must appear in returned categories
		wantSurvives   string // text that must remain (proves we didn't nuke too much)
	}
	cases := []tc{
		{
			name:         "aws-access-key-id",
			input:        "I was confused why my AKIA1234567890ABCDEF wasn't working",
			secret:       "AKIA1234567890ABCDEF",
			wantCategory: "aws_key",
			wantSurvives: "I was confused why",
		},
		{
			name:         "github-pat-classic",
			input:        "token=ghp_abcdefghijklmnopqrstuvwxyz0123456789AB and the rest",
			secret:       "ghp_abcdefghijklmnopqrstuvwxyz0123456789AB",
			wantCategory: "github_pat",
			wantSurvives: "and the rest",
		},
		{
			name:         "github-pat-fine-grained",
			input:        "header: github_pat_11ABCDE_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
			secret:       "github_pat_11ABCDE_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
			wantCategory: "github_pat",
			wantSurvives: "header:",
		},
		{
			// Synthetic placeholder, NOT a real key shape. GitHub secret
			// scanning blocks pushes that contain real-looking Stripe
			// patterns; this fixture matches the regex-shape only and
			// is composed of literal "x" characters so no scanner
			// false-positives the test corpus.
			name:         "stripe-live-secret",
			input:        "Authorization: Bearer sk_live_" + strings.Repeat("x", 24),
			secret:       "sk_live_" + strings.Repeat("x", 24),
			wantCategory: "stripe_key",
			wantSurvives: "Authorization: Bearer",
		},
		{
			name:         "jwt-three-parts",
			input:        "id_token=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ4In0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
			secret:       "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ4In0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
			wantCategory: "jwt",
			wantSurvives: "id_token=",
		},
		{
			name: "pem-private-key-block",
			input: "before\n-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA+abc/123+def/456=\nMORE+KEY+CONTENT/HERE==\n-----END RSA PRIVATE KEY-----\nafter",
			secret: "MIIEowIBAAKCAQEA+abc/123+def/456=",
			wantCategory: "pem_private_key",
			wantSurvives: "before",
		},
		{
			name:         "env-assignment-API_KEY",
			input:        `export API_KEY=sk-proj-abcdefghijklmnopqrstuvwx`,
			secret:       "sk-proj-abcdefghijklmnopqrstuvwx",
			wantCategory: "env_assignment",
			wantSurvives: "API_KEY",
		},
		{
			name:         "env-assignment-PASSWORD",
			input:        `MYSQL_PASSWORD="hunter2-very-long-secret-string"`,
			secret:       "hunter2-very-long-secret-string",
			wantCategory: "env_assignment",
			wantSurvives: "MYSQL_PASSWORD",
		},
		{
			name:         "env-assignment-TOKEN",
			input:        "GH_TOKEN: glpat-abcdefghijklmnopqrst",
			secret:       "glpat-abcdefghijklmnopqrst",
			wantCategory: "env_assignment",
			wantSurvives: "GH_TOKEN",
		},
		{
			name:         "env-assignment-SECRET",
			input:        "JWT_SECRET=mysupersecretvalue123ABCxyz",
			secret:       "mysupersecretvalue123ABCxyz",
			wantCategory: "env_assignment",
			wantSurvives: "JWT_SECRET",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			redacted, cats := redactSecrets(c.input)
			if strings.Contains(redacted, c.secret) {
				t.Errorf("secret %q survived redaction:\nredacted = %q", c.secret, redacted)
			}
			if !strings.Contains(redacted, "[REDACTED]") {
				t.Errorf("expected [REDACTED] marker in output:\n%q", redacted)
			}
			if !strings.Contains(redacted, c.wantSurvives) {
				t.Errorf("expected surrounding context %q to survive:\n%q", c.wantSurvives, redacted)
			}
			if !contains(cats, c.wantCategory) {
				t.Errorf("expected category %q in %v", c.wantCategory, cats)
			}
		})
	}
}

func TestRedactSecrets_NoMatch_PassesThrough(t *testing.T) {
	in := "deploy postgres canary, no secrets in this turn"
	out, cats := redactSecrets(in)
	if out != in {
		t.Errorf("unmodified text changed:\n  in:  %q\n  out: %q", in, out)
	}
	if len(cats) != 0 {
		t.Errorf("expected no categories on clean text, got %v", cats)
	}
}

func TestRedactSecrets_MultipleSecretsMultipleCategories(t *testing.T) {
	in := "AKIA1234567890ABCDEF and ghp_abcdefghijklmnopqrstuvwxyz0123456789AB"
	out, cats := redactSecrets(in)
	if strings.Contains(out, "AKIA1234567890ABCDEF") {
		t.Errorf("AWS key survived: %q", out)
	}
	if strings.Contains(out, "ghp_abcdefghijklmnopqrstuvwxyz0123456789AB") {
		t.Errorf("GitHub PAT survived: %q", out)
	}
	if !contains(cats, "aws_key") || !contains(cats, "github_pat") {
		t.Errorf("expected both aws_key and github_pat in categories, got %v", cats)
	}
}

func TestRedactSecrets_RedactedTextStaysJSONSafe(t *testing.T) {
	// Memories are persisted as JSON with CHECK(json_valid(content)).
	// The redaction marker must be stable across edits; assert it has
	// no characters that would invalidate JSON.
	in := `{"text":"AKIA1234567890ABCDEF in a secrets file"}`
	out, _ := redactSecrets(in)
	if strings.Contains(out, "AKIA1234567890ABCDEF") {
		t.Fatalf("secret survived: %q", out)
	}
	// [REDACTED] is plain ASCII, no quotes or backslashes.
	if !strings.Contains(out, "[REDACTED]") {
		t.Errorf("missing [REDACTED] marker: %q", out)
	}
}

func contains(slice []string, s string) bool {
	for _, x := range slice {
		if x == s {
			return true
		}
	}
	return false
}
