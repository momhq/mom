package projection

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractJSONObject(t *testing.T) {
	envelope := `{"files":[{"path":"topics/x.md","content":"# x"}],"index":"# Index","claude_block":""}`

	cases := []struct {
		name string
		in   string
		want string // "" means: expect empty result
	}{
		{
			name: "plain envelope",
			in:   envelope,
			want: envelope,
		},
		{
			name: "fenced envelope",
			in:   "```json\n" + envelope + "\n```",
			want: envelope,
		},
		{
			name: "prose preamble with a brace, then envelope",
			in:   "Looking at the log, the key decision is the auth config {mode: gateway}. Here is the vault:\n" + envelope,
			want: envelope,
		},
		{
			name: "valid-but-wrong object before envelope (the 'F'-after-pair break)",
			in:   `{"summary":"auth in gateway" Found the files below} ` + envelope,
			want: envelope,
		},
		{
			name: "incidental valid object before envelope",
			in:   `{"note":"thinking..."}\n` + envelope,
			want: envelope,
		},
		{
			name: "trailing prose after envelope",
			in:   envelope + "\n\nThat's the synthesized vault.",
			want: envelope,
		},
		{
			name: "no json at all",
			in:   "I could not synthesize anything from this log.",
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractJSONObject(tc.in)
			if got != tc.want {
				t.Fatalf("extractJSONObject mismatch:\n in:   %q\n got:  %q\n want: %q", tc.in, got, tc.want)
			}
			// Anything non-empty we return must be valid JSON the caller can unmarshal.
			if got != "" && !json.Valid([]byte(got)) {
				t.Errorf("returned span is not valid JSON: %q", got)
			}
		})
	}
}

func TestParseDelimitedFiles(t *testing.T) {
	// Content that would break a JSON string: quotes, code, braces, newlines.
	out := "some preamble prose\n" +
		"@@@FILE reference/collision.md@@@\n" +
		"---\ntype: reference\nname: Collision\n---\n# Collision\n" +
		"- Fixed the \"float\" bug with `const rate = shards / 2;` and {braces}\n" +
		"- Multi\nline body\n" +
		"@@@END@@@\n" +
		"@@@FILE identity.md@@@\n---\ntype: identity\n---\n# Proj\n@@@END@@@\n"

	files := parseDelimitedFiles(out)
	if len(files) != 2 {
		t.Fatalf("want 2 files, got %d: %v", len(files), files)
	}
	c := files["reference/collision.md"]
	if !strings.Contains(c, `"float"`) || !strings.Contains(c, "const rate = shards / 2;") || !strings.Contains(c, "{braces}") {
		t.Errorf("content with quotes/code/braces not preserved:\n%s", c)
	}
	if _, ok := files["identity.md"]; !ok {
		t.Errorf("second file missing")
	}
}

func TestParseDelimitedFiles_DropsTruncatedTail(t *testing.T) {
	// A complete block followed by a truncated one: keep the complete, drop the rest.
	out := "@@@FILE reference/a.md@@@\n---\ntype: reference\n---\n# A\n@@@END@@@\n" +
		"@@@FILE reference/b.md@@@\n---\ntype: reference\n---\n# B\n- truncated mid-w"
	files := parseDelimitedFiles(out)
	if len(files) != 1 || files["reference/a.md"] == "" {
		t.Errorf("want only the complete file a.md, got %v", files)
	}
}

func TestParseJSONFilesFallback(t *testing.T) {
	// The legacy JSON envelope some models emit despite the delimiter
	// instruction — optionally wrapped in a code fence.
	text := "```json\n{\"files\":[{\"path\":\"episodes/abc.md\",\"content\":\"---\\ntype: episode\\n---\\n# Ep\\n\"}]}\n```"
	files := parseJSONFiles(text)
	if len(files) != 1 {
		t.Fatalf("want 1 file from JSON fallback, got %d", len(files))
	}
	if _, ok := files["episodes/abc.md"]; !ok {
		t.Errorf("missing episodes/abc.md, got %v", files)
	}
	if parseJSONFiles("no json here") != nil {
		t.Errorf("want nil for non-JSON text")
	}
}
