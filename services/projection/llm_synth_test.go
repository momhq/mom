package projection

import (
	"context"
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

// fakeInvoker is a HarnessInvoker returning a canned response.
type fakeInvoker struct{ out string }

func (f *fakeInvoker) Name() string      { return "fake" }
func (f *fakeInvoker) IsAvailable() bool { return true }
func (f *fakeInvoker) Invoke(ctx context.Context, prompt string) (string, error) {
	return f.out, nil
}

func TestFoldDropsDisallowedPaths(t *testing.T) {
	block := func(path string) string {
		return "@@@FILE " + path + "@@@\n---\ntype: reference\n---\n# X\n@@@END@@@\n"
	}
	// Mixed batch: two valid concept files plus LLM junk of every rejected
	// shape — a script, an echoed hint key, a nested path, and traversal.
	out := block("reference/good.md") +
		block("conventions/release.md") +
		block("reference/generate_image.py") +
		block("_l0_hint") +
		block("episodes/sub/nested.md") +
		block("../escape.md") +
		block("reference/_hint.md")

	var warns []string
	s := NewLLMSynth(&fakeInvoker{out: out}, func(w string) { warns = append(warns, w) })
	res, err := s.Fold(context.Background(), FoldInput{ProjectID: "p"})
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}

	if len(res.Files) != 2 {
		t.Fatalf("want exactly the 2 valid files, got %d: %v", len(res.Files), res.Files)
	}
	for _, p := range []string{"reference/good.md", "conventions/release.md"} {
		if _, ok := res.Files[p]; !ok {
			t.Errorf("valid file %s missing from result", p)
		}
	}
	// Every dropped path is warned about, with the offending path included.
	for _, bad := range []string{"reference/generate_image.py", "_l0_hint", "episodes/sub/nested.md", "../escape.md", "reference/_hint.md"} {
		found := false
		for _, w := range warns {
			if strings.Contains(w, bad) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no warn for dropped path %q; warns: %v", bad, warns)
		}
	}
}

func TestAllowedVaultPath(t *testing.T) {
	cases := map[string]bool{
		"INDEX.md":               true,
		"identity.md":            true,
		"reference/voice.md":     true,
		"conventions/release.md":   true,
		"episodes/abc123.md":     true,
		"reference/x.py":         false, // not .md
		"_l0_hint":               false, // hint key, no extension
		"_l0_hint.md":            false, // not an allowed root file
		"notes.md":               false, // arbitrary root .md
		"reference/_hint.md":     false, // _-prefixed name
		"reference/.hidden.md":   false, // dot-file
		"episodes/sub/nested.md": false, // nested subdir
		"../escape.md":           false, // traversal
		"/etc/x.md":              false, // absolute
		"topics/legacy.md":       false, // legacy dir not in ICM allowlist
		"reference/.md":          false, // empty name
	}
	for p, want := range cases {
		if got := allowedVaultPath(p); got != want {
			t.Errorf("allowedVaultPath(%q) = %v, want %v", p, got, want)
		}
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

func (f *fakeInvoker) SetModel(string) error { return nil }
