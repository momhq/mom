// Package buildinfo holds the binary version/commit stamped at build time
// via Makefile ldflags. It is the single source of truth for the MOM build
// version; other packages (CLI, HTTP services) read from here instead of
// keeping their own copies that drift out of sync.
package buildinfo

// Set via -ldflags at build time (see Makefile).
var (
	Version = "dev"
	Commit  = "none"
)
