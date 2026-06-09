package librarian

import (
	"path/filepath"
	"testing"
)

func TestResolversHonorMomVaultOverride(t *testing.T) {
	home := t.TempDir()
	vault := filepath.Join(home, ".mom", "mom.db")
	t.Setenv("MOM_VAULT", vault)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if want := filepath.Join(home, ".mom"); dir != want {
		t.Errorf("Dir = %q, want %q", dir, want)
	}

	ledgerDir, err := LedgerDir()
	if err != nil {
		t.Fatalf("LedgerDir: %v", err)
	}
	if want := filepath.Join(home, ".mom", "ledger"); ledgerDir != want {
		t.Errorf("LedgerDir = %q, want %q", ledgerDir, want)
	}

	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if path != vault {
		t.Errorf("Path = %q, want %q", path, vault)
	}
}

func TestDirDefaultsToHomeDotMom(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MOM_VAULT", "")
	t.Setenv("HOME", home)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if want := filepath.Join(home, ".mom"); dir != want {
		t.Errorf("Dir = %q, want %q", dir, want)
	}
}
