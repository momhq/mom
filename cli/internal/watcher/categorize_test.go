package watcher

import "testing"

func TestCategorizeToolCall(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		// Memory tools — bare and MCP-prefixed.
		{"mom_recall", "mom_memory"},
		{"mom_record", "mom_memory"},
		{"mom_get", "mom_memory"},
		{"mom_landmarks", "mom_memory"},
		{"mom_status", "mom_memory"},
		{"mcp__mom__mom_recall", "mom_memory"},
		{"mcp__mom__mom_record", "mom_memory"},
		// Codebase reads.
		{"Read", "codebase_read"},
		{"Grep", "codebase_read"},
		{"Glob", "codebase_read"},
		// Codebase writes.
		{"Write", "codebase_write"},
		{"Edit", "codebase_write"},
		// Anything else falls to system.
		{"Bash", "system"},
		{"WebSearch", "system"},
		{"unknown_tool", "system"},
		{"", "system"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CategorizeToolCall(tc.name)
			if got != tc.want {
				t.Errorf("CategorizeToolCall(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestNormalizeToolName(t *testing.T) {
	cases := map[string]string{
		"Read":                 "Read",
		"mcp__mom__mom_recall": "mom_recall",
		"mcp__github__create":  "create",
		"mcp__":                "mcp__",     // malformed: no second separator
		"":                     "",
	}
	for in, want := range cases {
		if got := NormalizeToolName(in); got != want {
			t.Errorf("NormalizeToolName(%q) = %q, want %q", in, got, want)
		}
	}
}
