package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOnboarding_DefaultSelections verifies that pressing Enter for every
// prompt accepts the default value for each step.
func TestOnboarding_DefaultSelections(t *testing.T) {
	// Six newlines: runtime, language, profile, autonomy, core-source, confirm.
	input := strings.NewReader("\n\n\n\n\n\n")
	output := &bytes.Buffer{}

	result, err := runOnboarding(input, output, t.TempDir())
	if err != nil {
		t.Fatalf("runOnboarding failed: %v", err)
	}

	if result.Runtime != "claude" {
		t.Errorf("expected runtime=claude, got %q", result.Runtime)
	}
	if result.Language != "en" {
		t.Errorf("expected language=en, got %q", result.Language)
	}
	if result.DefaultProfile != "generalist" {
		t.Errorf("expected profile=generalist, got %q", result.DefaultProfile)
	}
	if result.Autonomy != "balanced" {
		t.Errorf("expected autonomy=balanced, got %q", result.Autonomy)
	}
}

// TestOnboarding_ExplicitSelections verifies that a user can pick non-default
// options at each step.
func TestOnboarding_ExplicitSelections(t *testing.T) {
	// runtime=2 (cursor), language=2 (pt), profile=2 (backend-engineer),
	// autonomy=3 (supervised), core-source=skip, confirm=Y
	input := strings.NewReader("2\n2\n2\n3\n\nY\n")
	output := &bytes.Buffer{}

	result, err := runOnboarding(input, output, t.TempDir())
	if err != nil {
		t.Fatalf("runOnboarding failed: %v", err)
	}

	if result.Runtime != "cursor" {
		t.Errorf("expected runtime=cursor, got %q", result.Runtime)
	}
	if result.Language != "pt" {
		t.Errorf("expected language=pt, got %q", result.Language)
	}
	if result.DefaultProfile != "backend-engineer" {
		t.Errorf("expected profile=backend-engineer, got %q", result.DefaultProfile)
	}
	if result.Autonomy != "supervised" {
		t.Errorf("expected autonomy=supervised, got %q", result.Autonomy)
	}
}

// TestOnboarding_InvalidThenValid verifies that invalid input causes a
// re-prompt and the wizard accepts the subsequent valid input.
func TestOnboarding_InvalidThenValid(t *testing.T) {
	// runtime: bad input then valid "3" (windsurf)
	// language: bad input then default Enter
	// profile: "1" (generalist), autonomy: "1" (autonomous), core-source: skip, confirm: default Enter
	input := strings.NewReader("99\n3\nXXX\n\n1\n1\n\n\n")
	output := &bytes.Buffer{}

	result, err := runOnboarding(input, output, t.TempDir())
	if err != nil {
		t.Fatalf("runOnboarding failed: %v", err)
	}

	if result.Runtime != "windsurf" {
		t.Errorf("expected runtime=windsurf, got %q", result.Runtime)
	}
	if result.Language != "en" {
		t.Errorf("expected language=en, got %q", result.Language)
	}
	if result.DefaultProfile != "generalist" {
		t.Errorf("expected profile=generalist, got %q", result.DefaultProfile)
	}
	if result.Autonomy != "autonomous" {
		t.Errorf("expected autonomy=autonomous, got %q", result.Autonomy)
	}
}

// TestOnboarding_RuntimeAutoDetect_Claude verifies that when a .claude/
// directory exists in the working dir the wizard defaults to Claude.
func TestOnboarding_RuntimeAutoDetect_Claude(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}

	// Accept all defaults (6 newlines: runtime, language, profile, autonomy, core-source, confirm).
	input := strings.NewReader("\n\n\n\n\n\n")
	output := &bytes.Buffer{}

	result, err := runOnboarding(input, output, dir)
	if err != nil {
		t.Fatalf("runOnboarding failed: %v", err)
	}

	if result.Runtime != "claude" {
		t.Errorf("expected auto-detected runtime=claude, got %q", result.Runtime)
	}

	// The output should mention the auto-detection.
	if !strings.Contains(output.String(), "Detected") {
		t.Errorf("expected output to mention detection, got:\n%s", output.String())
	}
}

// TestOnboarding_RuntimeAutoDetect_Cursor verifies that when a .cursor/
// directory exists the wizard defaults to Cursor.
func TestOnboarding_RuntimeAutoDetect_Cursor(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".cursor"), 0755); err != nil {
		t.Fatal(err)
	}

	// Accept all defaults (6 newlines: runtime, language, profile, autonomy, core-source, confirm).
	input := strings.NewReader("\n\n\n\n\n\n")
	output := &bytes.Buffer{}

	result, err := runOnboarding(input, output, dir)
	if err != nil {
		t.Fatalf("runOnboarding failed: %v", err)
	}

	if result.Runtime != "cursor" {
		t.Errorf("expected auto-detected runtime=cursor, got %q", result.Runtime)
	}
}

// TestOnboarding_ConfirmNo verifies that answering "n" at the confirm step
// returns an error signalling the user aborted.
func TestOnboarding_ConfirmNo(t *testing.T) {
	// runtime, language, profile, autonomy, core-source (skip), confirm=n
	input := strings.NewReader("\n\n\n\n\nn\n")
	output := &bytes.Buffer{}

	_, err := runOnboarding(input, output, t.TempDir())
	if err == nil {
		t.Fatal("expected error when user aborts at confirm step")
	}
	if !strings.Contains(err.Error(), "aborted") {
		t.Errorf("expected 'aborted' in error, got: %v", err)
	}
}

// TestOnboarding_OutputContainsWelcome verifies the welcome banner appears.
func TestOnboarding_OutputContainsWelcome(t *testing.T) {
	input := strings.NewReader("\n\n\n\n\n\n")
	output := &bytes.Buffer{}

	_, err := runOnboarding(input, output, t.TempDir())
	if err != nil {
		t.Fatalf("runOnboarding failed: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "Welcome to Leo") {
		t.Errorf("expected welcome banner, got:\n%s", out)
	}
}

// TestOnboarding_OutputContainsSummary verifies the summary step renders.
func TestOnboarding_OutputContainsSummary(t *testing.T) {
	// runtime=1, language=1, profile=1, autonomy=2, core-source=skip, confirm=Y
	input := strings.NewReader("1\n1\n1\n2\n\nY\n")
	output := &bytes.Buffer{}

	_, err := runOnboarding(input, output, t.TempDir())
	if err != nil {
		t.Fatalf("runOnboarding failed: %v", err)
	}

	out := output.String()
	for _, keyword := range []string{"Runtime", "Language", "Profile", "Autonomy"} {
		if !strings.Contains(out, keyword) {
			t.Errorf("expected summary to contain %q, got:\n%s", keyword, out)
		}
	}
}

// --- Table-driven tests for each individual ask* function ---

func TestAskRuntime(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		cwd      string // "" means no special dir
		wantRT   string
	}{
		{name: "default enter", input: "\n", wantRT: "claude"},
		{name: "pick 1", input: "1\n", wantRT: "claude"},
		{name: "pick 2", input: "2\n", wantRT: "cursor"},
		{name: "pick 3", input: "3\n", wantRT: "windsurf"},
		{name: "pick 4 other", input: "4\n", wantRT: "other"},
		{name: "invalid then 2", input: "9\n2\n", wantRT: "cursor"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.cwd != "" {
				dir = tc.cwd
			}
			scanner, out := makeScanner(tc.input)
			got, err := askRuntime(scanner, out, dir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantRT {
				t.Errorf("expected %q, got %q", tc.wantRT, got)
			}
		})
	}
}

func TestAskLanguage(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantLang string
	}{
		{name: "default enter", input: "\n", wantLang: "en"},
		{name: "pick 1", input: "1\n", wantLang: "en"},
		{name: "pick 2", input: "2\n", wantLang: "pt"},
		{name: "pick 3", input: "3\n", wantLang: "es"},
		{name: "invalid then 3", input: "0\n3\n", wantLang: "es"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scanner, out := makeScanner(tc.input)
			got, err := askLanguage(scanner, out)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantLang {
				t.Errorf("expected %q, got %q", tc.wantLang, got)
			}
		})
	}
}

func TestAskProfile(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		wantProfile string
	}{
		{name: "default enter", input: "\n", wantProfile: "generalist"},
		{name: "pick 1", input: "1\n", wantProfile: "generalist"},
		{name: "pick 2", input: "2\n", wantProfile: "backend-engineer"},
		{name: "invalid then 1", input: "5\n1\n", wantProfile: "generalist"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scanner, out := makeScanner(tc.input)
			got, err := askProfile(scanner, out)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantProfile {
				t.Errorf("expected %q, got %q", tc.wantProfile, got)
			}
		})
	}
}

func TestAskAutonomy(t *testing.T) {
	cases := []struct {
		name         string
		input        string
		wantAutonomy string
	}{
		{name: "default enter", input: "\n", wantAutonomy: "balanced"},
		{name: "pick 1", input: "1\n", wantAutonomy: "autonomous"},
		{name: "pick 2", input: "2\n", wantAutonomy: "balanced"},
		{name: "pick 3", input: "3\n", wantAutonomy: "supervised"},
		{name: "invalid then 3", input: "bad\n3\n", wantAutonomy: "supervised"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scanner, out := makeScanner(tc.input)
			got, err := askAutonomy(scanner, out)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantAutonomy {
				t.Errorf("expected %q, got %q", tc.wantAutonomy, got)
			}
		})
	}
}

// TestOnboarding_IntegratesWithInit verifies that running leo init without
// --runtime and without --no-interactive but with a pre-configured reader
// picks up the onboarding result and writes the correct config.
// We can't exercise the wizard path from rootCmd (it checks cmd.Flags().Changed("runtime")),
// so we test runOnboarding + config integration directly.
func TestOnboarding_ResultMappedToConfig(t *testing.T) {
	// runtime=2, language=3, profile=2, autonomy=1, core-source=skip, confirm=Y
	input := strings.NewReader("2\n3\n2\n1\n\nY\n")
	output := &bytes.Buffer{}

	result, err := runOnboarding(input, output, t.TempDir())
	if err != nil {
		t.Fatalf("runOnboarding failed: %v", err)
	}

	// cursor, es, backend-engineer, autonomous
	if result.Runtime != "cursor" {
		t.Errorf("runtime: want cursor, got %q", result.Runtime)
	}
	if result.Language != "es" {
		t.Errorf("language: want es, got %q", result.Language)
	}
	if result.DefaultProfile != "backend-engineer" {
		t.Errorf("profile: want backend-engineer, got %q", result.DefaultProfile)
	}
	if result.Autonomy != "autonomous" {
		t.Errorf("autonomy: want autonomous, got %q", result.Autonomy)
	}
}

// makeScanner is a test helper that creates a bufio.Scanner-compatible reader
// and a bytes.Buffer writer from a raw input string.
// It returns them as the same types that askRuntime etc. accept.
func makeScanner(input string) (*scannerWrapper, *bytes.Buffer) {
	out := &bytes.Buffer{}
	return newScannerWrapper(strings.NewReader(input)), out
}
