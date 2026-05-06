// Package centralvault resolves and opens MOM's canonical v0.30 vault.
package centralvault

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/momhq/mom/cli/internal/librarian"
	"github.com/momhq/mom/cli/internal/logbook"
	"github.com/momhq/mom/cli/internal/vault"
)

const dbName = "mom.db"

// Dir returns MOM's canonical central vault directory: $HOME/.mom.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve $HOME: %w", err)
	}
	return filepath.Join(home, ".mom"), nil
}

// Path returns MOM's canonical central vault path: $HOME/.mom/mom.db.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, dbName), nil
}

// Migrations returns the full v0.30 central-vault migration set.
func Migrations() []vault.Migration {
	migs := append([]vault.Migration{}, librarian.Migrations()...)
	migs = append(migs, logbook.Migrations()...)
	return migs
}

// Open opens the central vault, creating $HOME/.mom when needed and
// applying all registered v0.30 migrations.
func Open() (*vault.Vault, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("cannot create %s: %w", dir, err)
	}
	path := filepath.Join(dir, dbName)
	v, err := vault.Open(path, Migrations())
	if err != nil {
		return nil, fmt.Errorf("vault.Open %s: %w", path, err)
	}
	return v, nil
}

// OpenLibrarian opens the central vault and returns a Librarian bound to it.
// The returned close function releases the underlying database handle.
func OpenLibrarian() (*librarian.Librarian, func() error, error) {
	v, err := Open()
	if err != nil {
		return nil, nil, err
	}
	return librarian.New(v), v.Close, nil
}
