package projection

import (
	"encoding/json"
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
