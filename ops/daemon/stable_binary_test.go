package daemon_test

import (
	"os"
	"path/filepath"
	"sync"
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

// Equal-content source and dest is a no-op even if the source mtime is set
// (guards against a stale timestamp-only comparison creeping back in).
func TestReconcileStableBinary_NoOpWhenContentEqualDespiteMtime(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	src := filepath.Join(t.TempDir(), "mom")
	writeFakeBinaryAt(t, src, "v1")

	if _, copied, err := daemon.ReconcileStableBinary(src); err != nil || !copied {
		t.Fatalf("first reconcile: copied=%v err=%v", copied, err)
	}

	// Rewrite the same content with an arbitrary (older or newer) mtime.
	writeFakeBinaryAt(t, src, "v1")
	older := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(src, older, older); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	_, copied, err := daemon.ReconcileStableBinary(src)
	if err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if copied {
		t.Errorf("expected copied=false when content is unchanged")
	}
}

// The Homebrew regression case: a bottle install carries different content
// but an OLDER build mtime than the last stable copy. Content-based
// comparison must still copy it in.
func TestReconcileStableBinary_CopiesDifferentContentWithOlderMtime(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	src := filepath.Join(t.TempDir(), "mom")
	writeFakeBinaryAt(t, src, "v1")

	if _, copied, err := daemon.ReconcileStableBinary(src); err != nil || !copied {
		t.Fatalf("first reconcile: copied=%v err=%v", copied, err)
	}

	// Simulate a Homebrew bottle: different content, but an older mtime
	// than what's already installed at the stable path.
	writeFakeBinaryAt(t, src, "v2-brew-bottle")
	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(src, past, past); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	path, copied, err := daemon.ReconcileStableBinary(src)
	if err != nil {
		t.Fatalf("reconcile after brew upgrade: %v", err)
	}
	if !copied {
		t.Errorf("expected copied=true for different content despite older mtime")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stable binary: %v", err)
	}
	if string(data) != "v2-brew-bottle" {
		t.Errorf("stable binary content = %q, want v2-brew-bottle", data)
	}
}

// Two goroutines reconciling concurrently (init and the watch-sweep racing
// right after an upgrade) must never publish a corrupt binary — each
// writer uses its own temp file, so the final rename always publishes one
// writer's complete content, never an interleaving of both.
func TestReconcileStableBinary_ConcurrentReconcileDoesNotCorrupt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	src := filepath.Join(t.TempDir(), "mom")
	content := "v2-" + string(make([]byte, 4096))
	writeFakeBinaryAt(t, src, content)

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := daemon.ReconcileStableBinary(src)
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: ReconcileStableBinary: %v", i, err)
		}
	}

	stable, err := daemon.StableBinaryPath()
	if err != nil {
		t.Fatalf("StableBinaryPath: %v", err)
	}
	data, err := os.ReadFile(stable)
	if err != nil {
		t.Fatalf("read stable binary: %v", err)
	}
	if string(data) != content {
		t.Errorf("stable binary corrupted: got %d bytes, want %d bytes matching source", len(data), len(content))
	}
}
