package projection

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDroidInvoker_Name(t *testing.T) {
	d := NewDroidInvoker("")
	if d.Name() != "droid" {
		t.Errorf("Name() = %q, want droid", d.Name())
	}
	if d.Bin != "droid" {
		t.Errorf("Bin = %q, want droid", d.Bin)
	}
}

func TestDroidInvoker_SetModel(t *testing.T) {
	d := NewDroidInvoker("")
	if err := d.SetModel("claude-opus-4-5"); err != nil {
		t.Fatalf("SetModel: %v", err)
	}
	if d.Model != "claude-opus-4-5" {
		t.Errorf("Model = %q, want claude-opus-4-5", d.Model)
	}
	if err := d.SetModel(""); err != nil {
		t.Fatalf("SetModel(\"\") should always succeed: %v", err)
	}
}

func TestDroidInvoker_IsAvailable(t *testing.T) {
	d := NewDroidInvoker("definitely-not-a-real-binary-xyz")
	if d.IsAvailable() {
		t.Error("expected IsAvailable=false for a nonexistent binary")
	}
}

// TestDroidInvoker_Invoke_PassesPromptOverStdinAndParsesResult locks two
// contracts: (1) the prompt goes over stdin, not argv — the same choice
// ClaudeInvoker makes to survive OS argv limits as a vault grows; (2)
// `droid exec --output-format json`'s {"result": "..."} envelope is exactly
// what extractAssistantText already unwraps, so no synthesis-side change was
// needed to support Droid.
func TestDroidInvoker_Invoke_PassesPromptOverStdinAndParsesResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script stub not supported on windows")
	}

	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	stdinFile := filepath.Join(dir, "stdin.txt")
	stub := filepath.Join(dir, "droid")

	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > \"" + argsFile + "\"\n" +
		"cat > \"" + stdinFile + "\"\n" +
		"printf '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"synthesized output\"}'\n"
	if err := os.WriteFile(stub, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	d := NewDroidInvoker(stub)
	if err := d.SetModel("test-model"); err != nil {
		t.Fatal(err)
	}

	out, err := d.Invoke(context.Background(), "hello droid")
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}

	argsData, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("reading args file: %v", err)
	}
	args := string(argsData)
	for _, want := range []string{"exec", "--output-format", "json", "-m", "test-model"} {
		if !strings.Contains(args, want) {
			t.Errorf("args %q missing %q", args, want)
		}
	}

	stdinData, err := os.ReadFile(stdinFile)
	if err != nil {
		t.Fatalf("reading stdin file: %v", err)
	}
	if string(stdinData) != "hello droid" {
		t.Errorf("stdin = %q, want %q (prompt must go over stdin, not argv)", string(stdinData), "hello droid")
	}

	assistantText := extractAssistantText(out)
	if assistantText != "synthesized output" {
		t.Errorf("extractAssistantText(%q) = %q, want %q", out, assistantText, "synthesized output")
	}
}

// TestDroidInvoker_Invoke_ReturnsErrorOnNonZeroExit locks the failure path:
// a non-zero exit must surface as an error the fold driver treats as an
// InvokeError (systemic), not a malformed-response retry.
func TestDroidInvoker_Invoke_ReturnsErrorOnNonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script stub not supported on windows")
	}

	dir := t.TempDir()
	stub := filepath.Join(dir, "droid")
	script := "#!/bin/sh\necho 'boom' 1>&2\nexit 1\n"
	if err := os.WriteFile(stub, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	d := NewDroidInvoker(stub)
	if _, err := d.Invoke(context.Background(), "hello"); err == nil {
		t.Fatal("expected error on non-zero exit")
	}
}
