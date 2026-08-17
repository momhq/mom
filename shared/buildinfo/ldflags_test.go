package buildinfo

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
)

// TestLdflagsTargetBuildinfo guards against reintroducing the class of bug
// where a release pipeline stamps -X into a package other than
// shared/buildinfo (e.g. a var-with-initializer alias that silently gets
// overwritten by package init, or a package path that no longer exists).
// Every -X target across the build/release tooling must point at
// shared/buildinfo.Version or shared/buildinfo.Commit.
func TestLdflagsTargetBuildinfo(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	files := []string{
		filepath.Join(repoRoot, "Makefile"),
		filepath.Join(repoRoot, ".github", "workflows", "release.yml"),
		filepath.Join(repoRoot, "Formula", "mom.rb"),
	}

	ldflagsTarget := regexp.MustCompile(`-X\s+(\S+)\.(\S+)=`)
	found := 0

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		matches := ldflagsTarget.FindAllStringSubmatch(string(data), -1)
		for _, m := range matches {
			pkg, symbol := m[1], m[2]
			found++
			if pkg != "github.com/momhq/mom/shared/buildinfo" {
				t.Errorf("%s: -X target package %q must be github.com/momhq/mom/shared/buildinfo", path, pkg)
			}
			if symbol != "Version" && symbol != "Commit" {
				t.Errorf("%s: -X target symbol %q must be Version or Commit", path, symbol)
			}
		}
	}

	if found == 0 {
		t.Fatal("no -X ldflags targets found; expected at least one per file")
	}
}
