package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/momhq/mom/bus/herald"
	"github.com/momhq/mom/storage/ledger"
	"github.com/momhq/mom/storage/librarian"
	"gopkg.in/yaml.v3"
)

// ── mom status tests (Ledger-backed) ─────────────────────────────────────────

// setupStatusLedger points momdir at a temp ~/.mom via MOM_VAULT and
// returns the ledger dir, so status reads a controlled Ledger.
func setupStatusLedger(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("MOM_VAULT", filepath.Join(home, ".mom", "mom.db"))
	dir, err := librarian.LedgerDir()
	if err != nil {
		t.Fatalf("LedgerDir: %v", err)
	}
	return dir
}

func TestStatusCmd_EmptyLedger_ShowsZeroEvents(t *testing.T) {
	setupStatusLedger(t)

	buf := new(bytes.Buffer)
	statusCmd.SetOut(buf)
	statusCmd.SetErr(buf)
	if err := runStatus(statusCmd, nil); err != nil {
		t.Fatalf("status failed: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"MOM", "cwd", "project", "ledger", "events", "watcher"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
	// Retired SQLite-only fields must be gone.
	for _, forbidden := range []string{"memories", "landmarks", "op events"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("did not expect retired field %q in output, got:\n%s", forbidden, out)
		}
	}
}

func TestStatusCmd_WithEvents_ShowsCountsAndSpan(t *testing.T) {
	dir := setupStatusLedger(t)
	l, err := ledger.Open(dir)
	if err != nil {
		t.Fatalf("ledger.Open: %v", err)
	}
	at := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if _, err := l.Append(herald.Event{Type: herald.TurnObserved, SessionID: "s1", Timestamp: at, Payload: map[string]any{"role": "user"}}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	_ = l.Close()

	buf := new(bytes.Buffer)
	statusCmd.SetOut(buf)
	statusCmd.SetErr(buf)
	if err := runStatus(statusCmd, nil); err != nil {
		t.Fatalf("status failed: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"total 3", "span", "by type", string(herald.TurnObserved)} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

// TestHelperYamlParsesBadYaml confirms that {unclosed fails yaml.Unmarshal.
func TestHelperYamlParsesBadYaml(t *testing.T) {
	var v map[string]any
	err := yaml.Unmarshal([]byte("{unclosed\n"), &v)
	if err == nil {
		t.Fatal("expected yaml.Unmarshal to fail for '{unclosed' input")
	}
}
