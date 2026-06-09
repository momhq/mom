// Package librarian resolves where MOM keeps its records on disk.
//
// The Librarian is MOM's keeper of locations: it knows where the central
// directory, the append-only Ledger, and the (legacy) SQLite vault live,
// so the rest of the system can find them without hard-coding paths. It
// is pure path logic — no database, no I/O beyond reading $HOME and the
// MOM_VAULT override (which repoints the central directory for local
// testing and contributor workflows).
//
// This package was, before v0.50, the SQLite CRUD layer over the global
// vault. That vault is retired; the Ledger is the source of truth and
// agents read the markdown vault directly. The Librarian name lives on
// for the role it always served — knowing where everything is shelved.
package librarian

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	dbName       = "mom.db"
	envVaultPath = "MOM_VAULT"
)

// Dir returns MOM's central directory (~/.mom). If MOM_VAULT is set, Dir
// returns that file's parent directory instead.
func Dir() (string, error) {
	if override := os.Getenv(envVaultPath); override != "" {
		return filepath.Dir(override), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve $HOME: %w", err)
	}
	return filepath.Join(home, ".mom"), nil
}

// LedgerDir returns the directory holding the append-only Ledger
// (~/.mom/ledger) — the surviving source of truth.
func LedgerDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ledger"), nil
}

// Path returns the legacy SQLite vault path (~/.mom/mom.db, or the
// MOM_VAULT override). Retained only so uninstall/upgrade/doctor can
// detect and delete a pre-v0.50 mom.db; nothing opens it anymore.
func Path() (string, error) {
	if override := os.Getenv(envVaultPath); override != "" {
		return override, nil
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, dbName), nil
}
