package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestContainsGitRepos_NoChildren returns false when no subdirs have .git/.
func TestContainsGitRepos_NoChildren(t *testing.T) {
	dir := t.TempDir()
	if containsGitRepos(dir) {
		t.Error("expected false for empty dir")
	}
}

// TestContainsGitRepos_WithGitChild returns true when a child has .git/.
func TestContainsGitRepos_WithGitChild(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "repo1")
	os.MkdirAll(filepath.Join(child, ".git"), 0755)

	if !containsGitRepos(dir) {
		t.Error("expected true when child contains .git/")
	}
}

// TestContainsGitRepos_DotGitOnlyAtRoot verifies that .git in root itself
// is not counted as a child git repo.
func TestContainsGitRepos_DotGitOnlyAtRoot(t *testing.T) {
	dir := t.TempDir()
	// Add .git to dir itself (not a child subdir).
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	if containsGitRepos(dir) {
		t.Error("expected false: .git in root itself does not count as child repos")
	}
}

// TestDetectParentDirs_OrgRole verifies that a parent whose children have .git
// gets the "org" label.
func TestDetectParentDirs_OrgRole(t *testing.T) {
	// Build: tmpBase/repo (.git present)
	tmpBase := t.TempDir()
	repoDir := filepath.Join(tmpBase, "repo")
	os.MkdirAll(filepath.Join(repoDir, ".git"), 0755)

	// Use a fake home above tmpBase so the walk doesn't stop early.
	fakeHome := filepath.Dir(tmpBase)

	parents := detectParentDirs(repoDir, fakeHome)

	if len(parents) == 0 {
		t.Fatal("expected at least one parent")
	}

	// tmpBase should appear and be labeled "org" (has child with .git).
	found := false
	for _, p := range parents {
		if p.Path == tmpBase {
			found = true
			if p.Label != "org" {
				t.Errorf("tmpBase label = %q, want org", p.Label)
			}
			if len(p.ChildRepos) == 0 {
				t.Error("expected tmpBase to have child repos")
			}
		}
	}
	if !found {
		t.Errorf("tmpBase not found in parents: %+v", parents)
	}
}

// TestDetectParentDirs_RepoRole verifies that a parent with .git/ itself
// gets the "repo" label (no git children).
func TestDetectParentDirs_RepoRole(t *testing.T) {
	tmpBase := t.TempDir()
	parentDir := filepath.Join(tmpBase, "parent")
	childDir := filepath.Join(parentDir, "child")
	os.MkdirAll(filepath.Join(parentDir, ".git"), 0755)
	os.MkdirAll(childDir, 0755)

	fakeHome := tmpBase

	parents := detectParentDirs(childDir, fakeHome)

	found := false
	for _, p := range parents {
		if p.Path == parentDir {
			found = true
			if p.Label != "repo" {
				t.Errorf("parentDir label = %q, want repo", p.Label)
			}
		}
	}
	if !found {
		t.Fatalf("parentDir not found in parents: %+v", parents)
	}
}

// TestResolveScopeChoice_Repo verifies cwd choice → repo label.
func TestResolveScopeChoice_Repo(t *testing.T) {
	cwd := t.TempDir()
	parents := []ParentScope{}
	dir, label := resolveScopeChoice("cwd", "", cwd, parents)
	if dir != cwd {
		t.Errorf("installDir = %q, want %q", dir, cwd)
	}
	if label != "repo" {
		t.Errorf("scopeLabel = %q, want repo", label)
	}
}

// TestResolveScopeChoice_ParentOrgLabel verifies parent with org label resolves correctly.
func TestResolveScopeChoice_ParentOrgLabel(t *testing.T) {
	home := t.TempDir()
	orgDir := filepath.Join(home, "org")
	os.MkdirAll(orgDir, 0755)

	parents := []ParentScope{
		{Path: orgDir, Label: "org", HasGit: false, ChildRepos: []string{filepath.Join(orgDir, "repo")}},
	}

	dir, label := resolveScopeChoice("parent:"+orgDir, "", home, parents)
	if dir != orgDir {
		t.Errorf("installDir = %q, want %q", dir, orgDir)
	}
	if label != "org" {
		t.Errorf("scopeLabel = %q, want org", label)
	}
}

// TestResolveScopeChoice_ParentRepoLabel verifies parent with repo label resolves correctly.
func TestResolveScopeChoice_ParentRepoLabel(t *testing.T) {
	cwd := t.TempDir()
	parentDir := filepath.Join(cwd, "parent")
	os.MkdirAll(filepath.Join(parentDir, ".git"), 0755)

	parents := []ParentScope{
		{Path: parentDir, Label: "repo", HasGit: true, ChildRepos: nil},
	}

	dir, label := resolveScopeChoice("parent:"+parentDir, "", cwd, parents)
	if dir != parentDir {
		t.Errorf("installDir = %q, want %q", dir, parentDir)
	}
	if label != "repo" {
		t.Errorf("scopeLabel = %q, want repo", label)
	}
}

// TestBuildScopeOptions_IncludesParents verifies parent options are included.
func TestBuildScopeOptions_IncludesParents(t *testing.T) {
	home := t.TempDir()
	orgDir := filepath.Join(home, "myorg")
	os.MkdirAll(orgDir, 0755)
	cwd := filepath.Join(orgDir, "myrepo")
	os.MkdirAll(cwd, 0755)

	parents := []ParentScope{
		{Path: orgDir, Label: "org", HasGit: false, ChildRepos: []string{cwd}},
	}

	opts := buildScopeOptions(cwd, parents)

	// Should have: parent option, cwd option, custom option.
	if len(opts) < 3 {
		t.Fatalf("expected >= 3 options, got %d", len(opts))
	}

	hasParent := false
	for _, opt := range opts {
		if strings.HasPrefix(opt.Value, "parent:") {
			hasParent = true
			break
		}
	}
	if !hasParent {
		t.Error("expected at least one parent option in scope opts")
	}
}
