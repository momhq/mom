package centralvault_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/momhq/mom/cli/internal/centralvault"
)

func TestPathUsesHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := centralvault.Path()
	if err != nil {
		t.Fatalf("Path returned error: %v", err)
	}

	want := filepath.Join(home, ".mom", "mom.db")
	if got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestOpenLibrarianCreatesOnlyTempHomeVault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	lib, closeFn, err := centralvault.OpenLibrarian()
	if err != nil {
		t.Fatalf("OpenLibrarian returned error: %v", err)
	}
	if lib == nil {
		t.Fatal("OpenLibrarian returned nil Librarian")
	}
	defer func() { _ = closeFn() }()

	dbPath := filepath.Join(home, ".mom", "mom.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("central vault not created at temp HOME path %s: %v", dbPath, err)
	}

	for _, sidecar := range []string{dbPath + "-wal", dbPath + "-shm"} {
		if strings.HasPrefix(sidecar, home) {
			continue
		}
		t.Fatalf("sidecar path escaped temp HOME: %s", sidecar)
	}
}

func TestOpenRunsFullCentralMigrations(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	v, err := centralvault.Open()
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer func() { _ = v.Close() }()

	for _, table := range []string{"memories", "tags", "entities", "op_events", "filter_audit"} {
		table := table
		err := v.Query(
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`,
			[]any{table},
			func(rows *sql.Rows) error {
				if !rows.Next() {
					t.Fatalf("table %s not found", table)
				}
				var name string
				if err := rows.Scan(&name); err != nil {
					return err
				}
				if name != table {
					t.Fatalf("got table %q, want %q", name, table)
				}
				return nil
			},
		)
		if err != nil {
			t.Fatalf("querying table %s: %v", table, err)
		}
	}
}

func TestNoCentralVaultPathAssemblyOutsideHelper(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	internalDir := filepath.Dir(filepath.Dir(file))
	allowedDir := filepath.Dir(file)

	patterns := []string{
		`filepath.Join(home, ".mom", "mom.db")`,
		`filepath.Join(momHome, "mom.db")`,
	}

	err := filepath.WalkDir(internalDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		if filepath.Dir(path) == allowedDir {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, p := range patterns {
			if strings.Contains(text, p) {
				t.Fatalf("central vault path assembly %q found outside helper in %s", p, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking internal dir: %v", err)
	}
}
