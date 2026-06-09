package lens_test

import (
	"testing"

	"github.com/momhq/mom/shared/archtest"
)

// TestLens_DoesNotImportSQLiteStack asserts the dashboard reads the
// append-only Ledger (the source of truth) directly and never touches
// the retired SQLite vault stack.
func TestLens_DoesNotImportSQLiteStack(t *testing.T) {
	archtest.AssertNoDirectImport(t, ".",
		"github.com/momhq/mom/storage/librarian",
		"github.com/momhq/mom/storage/vault",
		"github.com/momhq/mom/storage/canonical",
		"github.com/momhq/mom/storage/legacy",
		"github.com/momhq/mom/services/finder",
	)
}

// TestLens_DoesNotImportWritePipeline asserts Lens stays on the read
// side of the data flow; the event write pipeline is not in scope.
func TestLens_DoesNotImportEvents(t *testing.T) {
	archtest.AssertNoDirectImport(t, ".",
		"github.com/momhq/mom/events/editor",
		"github.com/momhq/mom/events/registry",
	)
}
