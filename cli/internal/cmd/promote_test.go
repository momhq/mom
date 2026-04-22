package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/momhq/mom/cli/internal/memory"
)

// makeLeoPair creates two nested .mom/ installs under a tmpdir:
//   - root/.leo  (scope: user)
//   - root/sub/.leo  (scope: repo)
//
// Returns (rootDir, subDir, leoUser, leoRepo).
func makeLeoPair(t *testing.T) (string, string, string, string) {
	t.Helper()
	root := t.TempDir()
	sub := filepath.Join(root, "sub")

	leoUser := filepath.Join(root, ".mom")
	leoRepo := filepath.Join(sub, ".mom")

	for _, d := range []string{
		filepath.Join(leoUser, "memory"),
		filepath.Join(leoRepo, "memory"),
	} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("makeLeoPair: %v", err)
		}
	}

	writeConfigWithScope(t, leoUser, "user")
	writeConfigWithScope(t, leoRepo, "repo")

	return root, sub, leoUser, leoRepo
}

func writeConfigWithScope(t *testing.T, leoDir, label string) {
	t.Helper()
	content := "version: \"1\"\nscope: " + label + "\nruntimes:\n  claude:\n    enabled: true\n"
	if err := os.WriteFile(filepath.Join(leoDir, "config.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("writeConfigWithScope: %v", err)
	}
}

// writeMemoryTestDoc writes a minimal valid memory doc JSON file to leoDir/memory/<id>.json.
func writeMemoryTestDoc(t *testing.T, leoDir, id string) {
	t.Helper()
	doc := &memory.Doc{
		ID:        id,
		Scope:     "project",
		Tags:      []string{"test"},
		Created:   time.Now().UTC(),
		CreatedBy: "test",
		Content:   map[string]any{"body": "test content"},
	}
	path := filepath.Join(leoDir, "memory", id+".json")
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("writeMemoryTestDoc: %v", err)
	}
}

func runWithCwd(t *testing.T, cwd string, c *cobra.Command, args []string) (string, error) { //nolint:unused // kept for future promote tests
	t.Helper()
	// Change to cwd for the duration of the test.
	orig, _ := os.Getwd()
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) }) //nolint:errcheck

	buf := &bytes.Buffer{}
	c.SetOut(buf)
	c.SetErr(buf)
	err := c.RunE(c, args)
	return buf.String(), err
}

func TestPromote_HappyPath(t *testing.T) {
	root, sub, leoUser, leoRepo := makeLeoPair(t)
	t.Setenv("HOME", root)

	writeMemoryTestDoc(t, leoRepo, "my-fact")

	cmd := &cobra.Command{}
	cmd.Flags().String("to", "user", "")
	cmd.Flags().Set("to", "user") //nolint:errcheck

	// Work from the repo dir.
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir("/") }) //nolint:errcheck

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := runPromote(cmd, []string{"my-fact"})
	if err != nil {
		t.Fatalf("runPromote failed: %v\noutput: %s", err, buf.String())
	}

	// Source should be gone.
	srcPath := filepath.Join(leoRepo, "memory", "my-fact.json")
	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Error("source file should have been removed")
	}

	// Destination should exist.
	dstPath := filepath.Join(leoUser, "memory", "my-fact.json")
	if _, err := os.Stat(dstPath); err != nil {
		t.Errorf("destination file missing: %v", err)
	}

	// Provenance tag should be present.
	doc, err := memory.LoadDoc(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tag := range doc.Tags {
		if tag == "promoted_from:repo" {
			found = true
		}
	}
	if !found {
		t.Errorf("promoted_from:repo tag missing from tags: %v", doc.Tags)
	}
}

func TestPromote_NoAncestorWithScope(t *testing.T) {
	root, sub, _, _ := makeLeoPair(t)
	t.Setenv("HOME", root)

	_, leoRepo := filepath.Join(root, ".mom"), filepath.Join(sub, ".mom")
	writeMemoryTestDoc(t, leoRepo, "my-fact")

	cmd := &cobra.Command{}
	cmd.Flags().String("to", "org", "") // no org scope in tree
	cmd.Flags().Set("to", "org")        //nolint:errcheck

	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir("/") }) //nolint:errcheck

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := runPromote(cmd, []string{"my-fact"})
	if err == nil {
		t.Fatal("expected error when no ancestor has scope 'org'")
	}
}

func TestDemote_HappyPath(t *testing.T) {
	root, sub, leoUser, leoRepo := makeLeoPair(t)
	t.Setenv("HOME", root)

	writeMemoryTestDoc(t, leoUser, "user-fact")

	cmd := &cobra.Command{}
	cmd.Flags().String("to", "repo", "")
	cmd.Flags().Set("to", "repo") //nolint:errcheck

	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir("/") }) //nolint:errcheck

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := runDemote(cmd, []string{"user-fact"})
	if err != nil {
		t.Fatalf("runDemote failed: %v\noutput: %s", err, buf.String())
	}

	// Source should be gone.
	srcPath := filepath.Join(leoUser, "memory", "user-fact.json")
	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Error("source file should have been removed")
	}

	// Destination should exist.
	dstPath := filepath.Join(leoRepo, "memory", "user-fact.json")
	if _, err := os.Stat(dstPath); err != nil {
		t.Errorf("destination file missing: %v", err)
	}

	// Provenance tag should be present.
	doc, err := memory.LoadDoc(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tag := range doc.Tags {
		if tag == "demoted_from:user" {
			found = true
		}
	}
	if !found {
		t.Errorf("demoted_from:user tag missing from tags: %v", doc.Tags)
	}
}

func TestDemote_DocNotFoundAbove(t *testing.T) {
	root, sub, _, _ := makeLeoPair(t)
	t.Setenv("HOME", root)

	// Don't write any doc to leoUser — demote should fail.
	cmd := &cobra.Command{}
	cmd.Flags().String("to", "repo", "")
	cmd.Flags().Set("to", "repo") //nolint:errcheck

	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir("/") }) //nolint:errcheck

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := runDemote(cmd, []string{"nonexistent-fact"})
	if err == nil {
		t.Fatal("expected error when doc not found in any ancestor scope")
	}
}
