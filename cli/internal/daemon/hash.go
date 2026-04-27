package daemon

import (
	"crypto/sha256"
	"fmt"
)

// ProjectHash returns a short deterministic hash of the project path.
// Used to make service names unique per project.
func ProjectHash(projectDir string) string {
	h := sha256.Sum256([]byte(projectDir))
	return fmt.Sprintf("%x", h[:6]) // 12 hex chars
}
