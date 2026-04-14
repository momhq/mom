package profiles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultProfiles_ContainsExpected(t *testing.T) {
	defaults := DefaultProfiles()

	expected := []string{"generalist", "backend-engineer", "ceo", "cpo", "cto", "cmo", "cfo"}
	for _, name := range expected {
		if _, ok := defaults[name]; !ok {
			t.Errorf("missing default profile %q", name)
		}
	}
}

func TestDefaultProfiles_HaveRequiredFields(t *testing.T) {
	for name, p := range DefaultProfiles() {
		t.Run(name, func(t *testing.T) {
			if p.Name == "" {
				t.Error("Name is empty")
			}
			if p.Description == "" {
				t.Error("Description is empty")
			}
			if len(p.Focus) == 0 {
				t.Error("Focus is empty")
			}
			if p.Tone == "" {
				t.Error("Tone is empty")
			}
			if p.DefaultModel == "" {
				t.Error("DefaultModel is empty")
			}
			if p.ContextInjection == "" {
				t.Error("ContextInjection is empty")
			}
		})
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	original := &Profile{
		Name:             "Test Profile",
		Description:      "A test profile",
		Focus:            []string{"testing", "validation"},
		Tone:             "precise",
		DefaultModel:     "sonnet",
		ContextInjection: "You are a test specialist.",
	}

	if err := Save(dir, "test-profile", original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(dir, "test-profile")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Name != original.Name {
		t.Errorf("Name mismatch: %q vs %q", original.Name, loaded.Name)
	}
	if loaded.Description != original.Description {
		t.Errorf("Description mismatch: %q vs %q", original.Description, loaded.Description)
	}
	if len(loaded.Focus) != len(original.Focus) {
		t.Errorf("Focus length mismatch: %d vs %d", len(original.Focus), len(loaded.Focus))
	}
	if loaded.ContextInjection != original.ContextInjection {
		t.Errorf("ContextInjection mismatch")
	}
}

func TestLoad_NotFound(t *testing.T) {
	if _, err := Load(t.TempDir(), "nonexistent"); err == nil {
		t.Fatal("expected error for missing profile")
	}
}

func TestList_ReturnsYAMLFiles(t *testing.T) {
	dir := t.TempDir()

	// Create some profile files.
	for _, name := range []string{"alpha.yaml", "beta.yaml", "not-a-profile.txt"} {
		os.WriteFile(filepath.Join(dir, name), []byte("name: test"), 0644)
	}
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)

	names, err := List(dir)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(names) != 2 {
		t.Fatalf("expected 2 profiles, got %d: %v", len(names), names)
	}

	expected := map[string]bool{"alpha": true, "beta": true}
	for _, n := range names {
		if !expected[n] {
			t.Errorf("unexpected profile name: %q", n)
		}
	}
}

func TestList_EmptyDir(t *testing.T) {
	names, err := List(t.TempDir())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(names))
	}
}

func TestList_DirNotFound(t *testing.T) {
	if _, err := List("/nonexistent/dir"); err == nil {
		t.Fatal("expected error for missing dir")
	}
}
