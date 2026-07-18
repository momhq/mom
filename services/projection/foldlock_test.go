package projection

import (
	"errors"
	"testing"
)

// The fold lock is exclusive: while held, a second acquisition (e.g. a
// daemon auto-fold racing a manual `mom vault fold`) fails with
// ErrFoldLocked; after release it succeeds.
func TestFoldLock_Exclusive(t *testing.T) {
	root := t.TempDir()

	l1, err := AcquireFoldLock(root)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	if _, err := AcquireFoldLock(root); !errors.Is(err, ErrFoldLocked) {
		t.Fatalf("second acquire err = %v, want ErrFoldLocked", err)
	}

	l1.Release()

	l2, err := AcquireFoldLock(root)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	l2.Release()
}

// Locks on different project roots do not contend.
func TestFoldLock_PerProject(t *testing.T) {
	l1, err := AcquireFoldLock(t.TempDir())
	if err != nil {
		t.Fatalf("acquire a: %v", err)
	}
	defer l1.Release()
	l2, err := AcquireFoldLock(t.TempDir())
	if err != nil {
		t.Fatalf("acquire b: %v", err)
	}
	l2.Release()
}

// Release is idempotent.
func TestFoldLock_ReleaseIdempotent(t *testing.T) {
	l, err := AcquireFoldLock(t.TempDir())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	l.Release()
	l.Release() // must not panic
}

// RunProjectFold refuses to run while the project fold lock is held —
// the manual CLI path and the daemon both go through this gate.
func TestRunProjectFold_HonorsFoldLock(t *testing.T) {
	root := t.TempDir()
	l, err := AcquireFoldLock(root)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer l.Release()

	_, err = RunProjectFold(t.Context(), RunOptions{
		ProjectID: "p",
		Root:      root,
		LedgerDir: t.TempDir(),
	})
	if !errors.Is(err, ErrFoldLocked) {
		t.Fatalf("RunProjectFold err = %v, want ErrFoldLocked", err)
	}
}
