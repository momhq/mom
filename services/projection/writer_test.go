package projection

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFoldStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st := FoldState{
		LastOffset:   42,
		FoldedAt:     time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		FilesWritten: 7,
		EventsFolded: 100,
		Chunks: map[string]string{
			"abc123": "episodes/abc123.md",
			"def456": "topics/voice.md",
		},
	}

	if err := writeFoldState(dir, st); err != nil {
		t.Fatalf("writeFoldState: %v", err)
	}

	// Use a fake root where vault dir == dir.
	// LoadFoldState expects <root>/.mom/vault/.fold-state.json.
	// Set up the expected path manually.
	root := t.TempDir()
	vaultBase := VaultDir(root)
	if err := os.MkdirAll(vaultBase, 0o700); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, foldStateFileName))
	if err := os.WriteFile(filepath.Join(vaultBase, foldStateFileName), data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, found, err := LoadFoldState(root)
	if err != nil {
		t.Fatalf("LoadFoldState: %v", err)
	}
	if !found {
		t.Fatal("expected fold state to be found")
	}
	if got.LastOffset != st.LastOffset {
		t.Errorf("LastOffset: got %d, want %d", got.LastOffset, st.LastOffset)
	}
	if len(got.Chunks) != len(st.Chunks) {
		t.Errorf("Chunks len: got %d, want %d", len(got.Chunks), len(st.Chunks))
	}
	if got.Chunks["abc123"] != "episodes/abc123.md" {
		t.Errorf("Chunks[abc123]: got %q", got.Chunks["abc123"])
	}
}

func TestLoadFoldStateMissing(t *testing.T) {
	root := t.TempDir()
	_, found, err := LoadFoldState(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected found=false for missing state")
	}
}

func TestPruneStaleConcepts_RootLevel(t *testing.T) {
	base := t.TempDir()

	// Pre-existing root-level files.
	writeFile := func(name, content string) {
		if err := os.WriteFile(filepath.Join(base, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("identity.md", "# identity\n") // stale — not in keep
	writeFile("INDEX.md", "# index\n")       // must NEVER be deleted
	writeFile(".fold-state.json", `{}`)      // dot-file — must survive
	writeFile("something-old.md", "old\n")   // stale — not in keep

	// keep contains a future identity.md (the new fold will write it).
	// In a real rebuild that emits identity.md, it would be in keep.
	// Here we test the case where it is NOT in keep (stale).
	keep := map[string]bool{
		// nothing at root level
	}

	if err := pruneStaleConcepts(base, keep); err != nil {
		t.Fatalf("pruneStaleConcepts: %v", err)
	}

	// INDEX.md must survive.
	if _, err := os.Stat(filepath.Join(base, "INDEX.md")); err != nil {
		t.Error("INDEX.md was deleted but must survive")
	}
	// .fold-state.json (dot-file) must survive.
	if _, err := os.Stat(filepath.Join(base, ".fold-state.json")); err != nil {
		t.Error(".fold-state.json was deleted but must survive")
	}
	// Stale .md files must be gone.
	for _, name := range []string{"identity.md", "something-old.md"} {
		if _, err := os.Stat(filepath.Join(base, name)); !os.IsNotExist(err) {
			t.Errorf("%s should have been pruned but still exists", name)
		}
	}
}

func TestPruneStaleConcepts_KeepsNewIdentity(t *testing.T) {
	base := t.TempDir()

	// identity.md exists from a prior fold.
	if err := os.WriteFile(filepath.Join(base, "identity.md"), []byte("# old identity\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// New fold emits identity.md → it is in keep.
	keep := map[string]bool{
		"identity.md": true,
	}

	if err := pruneStaleConcepts(base, keep); err != nil {
		t.Fatalf("pruneStaleConcepts: %v", err)
	}

	// identity.md must survive because it is in keep.
	if _, err := os.Stat(filepath.Join(base, "identity.md")); err != nil {
		t.Error("identity.md in keep was deleted but must survive")
	}
}
