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

func TestPruneStaleConcepts_LegacyArtifactsAndDirs(t *testing.T) {
	base := t.TempDir()

	writeFile := func(rel, content string) {
		full := filepath.Join(base, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Legacy artifacts at root.
	writeFile("index", "legacy router\n")
	writeFile("_l0_hint", "leaked hint\n")
	// Root files that must survive.
	writeFile(".DS_Store", "junk")
	writeFile("notes.txt", "user notes\n") // other non-.md file — never touched
	// Legacy dir whose .md content the prune removes → dir becomes empty.
	writeFile("topics/old.md", "# old\n")
	// Legacy dir with stray non-.md user content → dir and file survive.
	writeFile("timeline/keep.txt", "user file\n")
	writeFile("timeline/stale.md", "# stale\n")

	if err := pruneStaleConcepts(base, map[string]bool{}); err != nil {
		t.Fatalf("pruneStaleConcepts: %v", err)
	}

	// Legacy artifacts deleted.
	for _, name := range []string{"index", "_l0_hint"} {
		if _, err := os.Stat(filepath.Join(base, name)); !os.IsNotExist(err) {
			t.Errorf("legacy artifact %s should have been pruned but still exists", name)
		}
	}
	// Dot-files and other non-.md root files survive.
	for _, name := range []string{".DS_Store", "notes.txt"} {
		if _, err := os.Stat(filepath.Join(base, name)); err != nil {
			t.Errorf("%s was deleted but must survive", name)
		}
	}
	// topics/ emptied by the prune → removed.
	if _, err := os.Stat(filepath.Join(base, "topics")); !os.IsNotExist(err) {
		t.Error("empty legacy dir topics/ should have been removed")
	}
	// timeline/ still holds a stray .txt → dir kept, .txt untouched, .md pruned.
	if _, err := os.Stat(filepath.Join(base, "timeline", "keep.txt")); err != nil {
		t.Error("timeline/keep.txt was deleted but must survive")
	}
	if _, err := os.Stat(filepath.Join(base, "timeline", "stale.md")); !os.IsNotExist(err) {
		t.Error("timeline/stale.md should have been pruned")
	}
	if _, err := os.Stat(filepath.Join(base, "timeline")); err != nil {
		t.Error("timeline/ holding user content must survive")
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

// TestWritePruneRefusesEmptyFreshSet guards against the failure that wiped a
// 2500-episode vault: a rebuild whose synthesis aborted immediately returns an
// empty file set, and pruning against it would delete every existing file.
func TestWritePruneRefusesEmptyFreshSet(t *testing.T) {
	root := t.TempDir()
	w := NewWriter(root)

	// Seed a vault with one episode via a normal write.
	seed := FoldResult{Files: map[string]string{"episodes/aaa.md": "# Episode\n"}, Index: "# idx\n"}
	if _, err := w.Write(seed, 10, 1, false); err != nil {
		t.Fatal(err)
	}

	// A failed rebuild: prune requested, but the fresh set is empty.
	empty := FoldResult{Files: map[string]string{}, Index: "# idx\n"}
	if _, err := w.Write(empty, 0, 0, true); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(VaultDir(root), "episodes", "aaa.md")); err != nil {
		t.Errorf("existing episode was pruned by an empty synthesis: %v", err)
	}
}
