package cmd

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/vmarinogg/leo-core/cli/internal/profiles"
)

// oneByteReader wraps a reader to return one byte at a time, preventing
// bufio.Scanner from buffering ahead and consuming input meant for later fields.
// This is needed because huh's accessible mode creates a new bufio.Scanner
// per field, and each scanner would otherwise read the full input.
type oneByteReader struct {
	r io.Reader
}

func (o *oneByteReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return o.r.Read(p[:1])
}

func testReader(input string) io.Reader {
	return &oneByteReader{r: strings.NewReader(input)}
}

// Accessible-mode input format:
//   Note:        no input consumed
//   MultiSelect: enter number to toggle (1-N), 0 to confirm
//   Select:      enter number (1-N), empty = default
//   Input:       enter text, empty = default
//   Confirm:     y/n, empty = default
//
// Form flow:
//   Form 1: Note(welcome), MultiSelect(runtimes), Select(lang), Select(mode),
//           Select(profile), Select(autonomy), Input(coreSource)
//   Form 2: Note(summary), Confirm

// TestOnboarding_DefaultSelections verifies that accepting all defaults works.
func TestOnboarding_DefaultSelections(t *testing.T) {
	// 0=confirm runtimes (claude pre-selected), then empty for all selects + input + confirm
	input := testReader("0\n\n\n\n\n\n\n")
	output := &bytes.Buffer{}

	result, err := runOnboarding(input, output, t.TempDir())
	if err != nil {
		t.Fatalf("runOnboarding failed: %v\noutput:\n%s", err, output.String())
	}

	if len(result.Runtimes) == 0 {
		t.Fatal("expected at least one runtime")
	}
	if result.Runtimes[0] != "claude" {
		t.Errorf("expected first runtime=claude, got %q", result.Runtimes[0])
	}
	if result.Language != "en" {
		t.Errorf("expected language=en, got %q", result.Language)
	}
	if result.Mode != "concise" {
		t.Errorf("expected mode=concise, got %q", result.Mode)
	}
	if result.DefaultProfile != "general-manager" {
		t.Errorf("expected profile=general-manager, got %q", result.DefaultProfile)
	}
	if result.Autonomy != "balanced" {
		t.Errorf("expected autonomy=balanced, got %q", result.Autonomy)
	}
}

// TestOnboarding_ExplicitSelections verifies non-default choices.
func TestOnboarding_ExplicitSelections(t *testing.T) {
	// Compute the index of "cto" in the sorted user-scoped profile list.
	allProfiles := profiles.DefaultProfiles()
	names := make([]string, 0)
	for name, p := range allProfiles {
		if p.Scope == "user" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	ctoIdx := 1
	for i, name := range names {
		if name == "cto" {
			ctoIdx = i + 1
			break
		}
	}

	// MultiSelect: toggle 2 (codex) then 0 (confirm), lang=2 (pt), mode=3 (caveman),
	// profile=cto, autonomy=3 (supervised), core-source=skip, confirm=y
	input := testReader(fmt.Sprintf("2\n0\n2\n3\n%d\n3\n\ny\n", ctoIdx))
	output := &bytes.Buffer{}

	result, err := runOnboarding(input, output, t.TempDir())
	if err != nil {
		t.Fatalf("runOnboarding failed: %v\noutput:\n%s", err, output.String())
	}

	hasRuntime := func(name string) bool {
		for _, r := range result.Runtimes {
			if r == name {
				return true
			}
		}
		return false
	}
	if !hasRuntime("claude") {
		t.Error("expected claude in runtimes")
	}
	if !hasRuntime("codex") {
		t.Error("expected codex in runtimes")
	}
	if result.Language != "pt" {
		t.Errorf("expected language=pt, got %q", result.Language)
	}
	if result.Mode != "caveman" {
		t.Errorf("expected mode=caveman, got %q", result.Mode)
	}
	if result.DefaultProfile != "cto" {
		t.Errorf("expected profile=cto, got %q", result.DefaultProfile)
	}
	if result.Autonomy != "supervised" {
		t.Errorf("expected autonomy=supervised, got %q", result.Autonomy)
	}
}

// TestOnboarding_ConfirmNo verifies that answering "n" at the confirm step
// returns an error signalling the user aborted.
func TestOnboarding_ConfirmNo(t *testing.T) {
	// Accept all defaults, then reject at confirm.
	input := testReader("0\n\n\n\n\n\nn\n")
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
	input := testReader("0\n\n\n\n\n\n\n")
	output := &bytes.Buffer{}

	_, err := runOnboarding(input, output, t.TempDir())
	if err != nil {
		t.Fatalf("runOnboarding failed: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "Welcome") {
		t.Errorf("expected welcome banner, got:\n%s", out)
	}
}

// TestOnboarding_OutputContainsSummary verifies the summary step renders.
func TestOnboarding_OutputContainsSummary(t *testing.T) {
	input := testReader("0\n1\n2\n1\n2\n\ny\n")
	output := &bytes.Buffer{}

	_, err := runOnboarding(input, output, t.TempDir())
	if err != nil {
		t.Fatalf("runOnboarding failed: %v\noutput:\n%s", err, output.String())
	}

	out := output.String()
	for _, keyword := range []string{"Runtimes", "Language", "Mode", "Profile", "Autonomy"} {
		if !strings.Contains(out, keyword) {
			t.Errorf("expected summary to contain %q, got:\n%s", keyword, out)
		}
	}
}

// TestOnboarding_MultipleRuntimesSelected verifies toggling multiple runtimes.
func TestOnboarding_MultipleRuntimesSelected(t *testing.T) {
	// Toggle codex (2) and cline (3), confirm (0), then defaults for everything.
	input := testReader("2\n3\n0\n\n\n\n\n\n\n")
	output := &bytes.Buffer{}

	result, err := runOnboarding(input, output, t.TempDir())
	if err != nil {
		t.Fatalf("runOnboarding failed: %v\noutput:\n%s", err, output.String())
	}

	if len(result.Runtimes) != 3 {
		t.Fatalf("expected 3 runtimes, got %d: %v", len(result.Runtimes), result.Runtimes)
	}
}
