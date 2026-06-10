package scope_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/momhq/mom/shared/scope"
)

// makeTree creates a directory tree under a temp dir and returns the temp root.
// dirs is a list of paths relative to the temp root that should be created.
func makeTree(t *testing.T, dirs ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			t.Fatalf("makeTree: %v", err)
		}
	}
	return root
}

// writeConfig writes a minimal config.yaml with the given scope label.
func writeConfig(t *testing.T, momDir, label string) {
	t.Helper()
	content := "version: \"1\"\nscope: " + label + "\nruntimes:\n  claude:\n    enabled: true\n"
	if err := os.WriteFile(filepath.Join(momDir, "config.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}
}

func TestNearestWritable_NearestFirst(t *testing.T) {
	// Tree: root/.mom, root/a/.mom, root/a/b/.mom — cwd = root/a/b.
	// NearestWritable returns the most specific scope (root/a/b/.mom).
	root := makeTree(t,
		".mom",
		"a/.mom",
		"a/b/.mom",
	)
	writeConfig(t, filepath.Join(root, ".mom"), "user")
	writeConfig(t, filepath.Join(root, "a", ".mom"), "org")
	writeConfig(t, filepath.Join(root, "a", "b", ".mom"), "repo")

	// Patch HOME to root so the walk stops there.
	t.Setenv("HOME", root)

	cwd := filepath.Join(root, "a", "b")
	s, ok := scope.NearestWritable(cwd)
	if !ok {
		t.Fatal("expected NearestWritable to return ok=true")
	}
	if s.Path != filepath.Join(root, "a", "b", ".mom") {
		t.Errorf("Path = %q, want %q", s.Path, filepath.Join(root, "a", "b", ".mom"))
	}
	if s.Label != "repo" {
		t.Errorf("Label = %q, want repo", s.Label)
	}
}

func TestNearestWritable_Found(t *testing.T) {
	root := makeTree(t, ".mom")
	writeConfig(t, filepath.Join(root, ".mom"), "repo")
	t.Setenv("HOME", root)

	s, ok := scope.NearestWritable(root)
	if !ok {
		t.Fatal("expected NearestWritable to return ok=true")
	}
	if s.Path != filepath.Join(root, ".mom") {
		t.Errorf("Path = %q", s.Path)
	}
}

func TestNearestWritable_NotFound(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)

	_, ok := scope.NearestWritable(root)
	if ok {
		t.Fatal("expected ok=false when no .mom/ exists")
	}
}

func TestNearestWritable_FindsParentScope(t *testing.T) {
	// Single .mom/ one level above cwd is discovered via walk-up.
	root := makeTree(t, ".mom", "sub")
	writeConfig(t, filepath.Join(root, ".mom"), "repo")
	t.Setenv("HOME", root)

	s, ok := scope.NearestWritable(filepath.Join(root, "sub"))
	if !ok {
		t.Fatal("expected ok=true")
	}
	if s.Path != filepath.Join(root, ".mom") {
		t.Errorf("Path = %q", s.Path)
	}
}

func TestDefaultScope_MissingField(t *testing.T) {
	// A .mom/ with no scope field in config.yaml outside $HOME defaults to "repo".
	root := makeTree(t, ".mom")
	content := "version: \"1\"\nruntimes:\n  claude:\n    enabled: true\n"
	if err := os.WriteFile(filepath.Join(root, ".mom", "config.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	// HOME points elsewhere so the $HOME/.mom/ → "user" override does not trigger.
	t.Setenv("HOME", t.TempDir())

	s, ok := scope.NearestWritable(root)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if s.Label != "repo" {
		t.Errorf("Label = %q, want repo", s.Label)
	}
}

func TestDefaultScope_MissingField_AtHome(t *testing.T) {
	// $HOME/.mom/ with no scope field defaults to "user" (override added in #219).
	root := makeTree(t, ".mom")
	content := "version: \"1\"\nruntimes:\n  claude:\n    enabled: true\n"
	if err := os.WriteFile(filepath.Join(root, ".mom", "config.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)

	s, ok := scope.NearestWritable(root)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if s.Label != "user" {
		t.Errorf("Label = %q, want user", s.Label)
	}
}
