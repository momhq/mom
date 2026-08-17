package projection

import (
	"os"
	"path/filepath"
	"testing"
)

func writeLintFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func goodFixtureVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	vault := VaultDir(root)

	writeLintFile(t, vault, "INDEX.md", "# MOM Vault\n\n"+
		"- [`identity.md`](identity.md)\n"+
		"- [`reference/INDEX.md`](reference/INDEX.md)\n"+
		"- [`conventions/INDEX.md`](conventions/INDEX.md)\n")
	writeLintFile(t, vault, "identity.md", "---\ntype: identity\nname: Test\n---\n# Test\n- A tight orientation node.\n")
	writeLintFile(t, vault, "reference/INDEX.md", "# Reference — index\n\n"+
		"| Concept | What it covers | Through |\n|---|---|---|\n"+
		"| [`voice.md`](voice.md) | Voice | 2024-01-01 |\n")
	writeLintFile(t, vault, "reference/voice.md", "---\ntype: reference\nname: Voice\n---\n# Voice\n- Terse, direct.\n")
	writeLintFile(t, vault, "conventions/INDEX.md", "# Conventions — index\n\n"+
		"| Concept | What it covers | Through |\n|---|---|---|\n"+
		"| [`releases.md`](releases.md) | Releases | 2024-01-01 |\n")
	writeLintFile(t, vault, "conventions/releases.md", "---\ntype: convention\nname: Releases\n---\n# Releases\n- Tag then publish.\n")
	writeLintFile(t, vault, "episodes/abc123.md", "---\ntype: episode\n---\n# abc123\n- Raw capture, not routed.\n")
	return root
}

func TestLintVault_GoodFixturePasses(t *testing.T) {
	root := goodFixtureVault(t)
	findings, err := LintVault(root)
	if err != nil {
		t.Fatalf("LintVault: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %+v", findings)
	}
}

func TestLintVault_OrphanFails(t *testing.T) {
	root := goodFixtureVault(t)
	vault := VaultDir(root)

	// Orphan: a reference concept the folder INDEX.md never links to.
	writeLintFile(t, vault, "reference/orphan.md", "---\ntype: reference\nname: Orphan\n---\n# Orphan\n- Unreachable.\n")

	findings, err := LintVault(root)
	if err != nil {
		t.Fatalf("LintVault: %v", err)
	}

	var gotOrphan bool
	for _, f := range findings {
		if f.Path == "reference/orphan.md" && f.Rule == "reachability" {
			gotOrphan = true
		}
	}
	if !gotOrphan {
		t.Errorf("expected a reachability finding for reference/orphan.md, got %+v", findings)
	}
}

func TestLintVault_PayloadInRouterFlagged(t *testing.T) {
	root := goodFixtureVault(t)
	vault := VaultDir(root)
	writeLintFile(t, vault, "reference/INDEX.md", "# Reference — index\n\n## Decisions\n- Leaked concept payload in a router.\n")

	findings, err := LintVault(root)
	if err != nil {
		t.Fatalf("LintVault: %v", err)
	}
	var got bool
	for _, f := range findings {
		if f.Path == "reference/INDEX.md" && f.Rule == "no-payload-in-router" {
			got = true
		}
	}
	if !got {
		t.Errorf("expected a no-payload-in-router finding for reference/INDEX.md, got %+v", findings)
	}
}
