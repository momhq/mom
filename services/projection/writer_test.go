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
