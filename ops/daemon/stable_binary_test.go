package daemon_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/momhq/mom/ops/daemon"
)

func writeFakeBinaryAt(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
}

// First reconcile with nothing installed yet: copies in and reports copied.
func TestReconcileStableBinary_FirstInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	src := filepath.Join(t.TempDir(), "mom")
	writeFakeBinaryAt(t, src, "v1")

	path, copied, err := daemon.ReconcileStableBinary(src)
	if err != nil {
		t.Fatalf("ReconcileStableBinary: %v", err)
	}
	if !copied {
		t.Errorf("expected copied=true on first install")
	}
	wantPath := filepath.Join(home, ".mom", "bin", "mom")
	if path != wantPath {
		t.Errorf("stable path = %q, want %q", path, wantPath)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stable binary: %v", err)
	}
	if string(data) != "v1" {
		t.Errorf("stable binary content = %q, want v1", data)
	}
}

// Reconciling the same (unchanged) source binary a second time is a no-op.
func TestReconcileStableBinary_NoOpWhenUnchanged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	src := filepath.Join(t.TempDir(), "mom")
	writeFakeBinaryAt(t, src, "v1")

	if _, copied, err := daemon.ReconcileStableBinary(src); err != nil || !copied {
		t.Fatalf("first reconcile: copied=%v err=%v", copied, err)
	}

	_, copied, err := daemon.ReconcileStableBinary(src)
	if err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if copied {
		t.Errorf("expected copied=false when source binary hasn't changed")
	}
}

// A newer source binary (simulating a Tauri sidecar update, or a Homebrew
// Cellar swap) is copied in on the next reconcile.
func TestReconcileStableBinary_CopiesNewerBinary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	src := filepath.Join(t.TempDir(), "mom")
	writeFakeBinaryAt(t, src, "v1")

	if _, _, err := daemon.ReconcileStableBinary(src); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}

	// Simulate an upgrade: new content, later mtime.
	writeFakeBinaryAt(t, src, "v2-longer-content")
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(src, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	path, copied, err := daemon.ReconcileStableBinary(src)
	if err != nil {
		t.Fatalf("reconcile after upgrade: %v", err)
	}
	if !copied {
		t.Errorf("expected copied=true when source is newer")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stable binary: %v", err)
	}
	if string(data) != "v2-longer-content" {
		t.Errorf("stable binary content = %q, want v2-longer-content", data)
	}
}

// Running from the stable path itself is a no-op — avoids copying a file
// onto itself.
func TestReconcileStableBinary_NoOpWhenAlreadyStable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	stable, err := daemon.StableBinaryPath()
	if err != nil {
		t.Fatalf("StableBinaryPath: %v", err)
	}
	writeFakeBinaryAt(t, stable, "v1")

	path, copied, err := daemon.ReconcileStableBinary(stable)
	if err != nil {
		t.Fatalf("ReconcileStableBinary: %v", err)
	}
	if copied {
		t.Errorf("expected copied=false when currentBinary is already the stable path")
	}
	if path != stable {
		t.Errorf("path = %q, want %q", path, stable)
	}
}
