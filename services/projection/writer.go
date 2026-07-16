package projection

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// vaultDirName is the project-local vault directory, relative to root.
	vaultDirName = ".mom/vault"
	// indexFileName is the router file at the vault root.
	indexFileName = "INDEX.md"
	// foldStateFileName is the watermark JSON at the vault root.
	foldStateFileName = ".fold-state.json"
	// claudeFileName is the project-root managed-block host file.
	claudeFileName = "CLAUDE.md"

	claudeBeginMarker = "<!-- MOM:BEGIN -->"
	claudeEndMarker   = "<!-- MOM:END -->"
)

// FoldState is the watermark persisted at .mom/vault/.fold-state.json.
type FoldState struct {
	LastOffset   uint64    `json:"last_offset"`
	FoldedAt     time.Time `json:"folded_at"`
	FilesWritten int       `json:"files_written"`
	EventsFolded int       `json:"events_folded"`
	// LastGardenedAt records the last `mom vault garden` pass. Zero until
	// the vault has been gardened. Garden consumes no new events, so it
	// never changes LastOffset.
	LastGardenedAt time.Time `json:"last_gardened_at,omitempty"`
	// Chunks maps chunkID → vault-relative path for every chunk synthesized
	// in the last fold. Used by the next incremental fold to skip re-synthesis
	// of chunks whose source events haven't changed. Nil on fresh vaults.
	Chunks map[string]string `json:"chunks,omitempty"`
	// PendingSynthesis is true when the last fold completed L0 but aborted L1
	// or L2 due to a systemic harness failure. The next fold must re-run the
	// L1/L2 passes against the existing episodes even when there are no new
	// events.
	PendingSynthesis bool `json:"pending_synthesis,omitempty"`
}

// VaultDir returns the absolute vault directory for a project root.
func VaultDir(root string) string { return filepath.Join(root, vaultDirName) }

// LoadFoldState reads the watermark for a project root. A missing file
// is not an error — it returns a zero FoldState and found=false.
func LoadFoldState(root string) (FoldState, bool, error) {
	path := filepath.Join(VaultDir(root), foldStateFileName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return FoldState{}, false, nil
	}
	if err != nil {
		return FoldState{}, false, fmt.Errorf("read fold-state: %w", err)
	}
	var st FoldState
	if err := json.Unmarshal(data, &st); err != nil {
		return FoldState{}, false, fmt.Errorf("parse fold-state: %w", err)
	}
	return st, true, nil
}

// LoadExisting reads all current vault markdown files (except INDEX.md
// and the fold-state) into a path→content map keyed by vault-relative
// path. Used to feed incremental synthesis.
func LoadExisting(root string) (map[string]string, error) {
	base := VaultDir(root)
	out := map[string]string{}
	err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(base, path)
		if rerr != nil {
			return rerr
		}
		if rel == indexFileName || rel == foldStateFileName {
			return nil
		}
		if !strings.HasSuffix(rel, ".md") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan existing vault: %w", err)
	}
	return out, nil
}

// Writer materializes a FoldResult under <root>/.mom/vault and updates
// the CLAUDE.md managed block.
type Writer struct {
	Root string
}

// NewWriter binds a Writer to a project root.
func NewWriter(root string) *Writer { return &Writer{Root: root} }

// WriteResult reports what Write did.
type WriteResult struct {
	FilesWritten int
	VaultDir     string
	ClaudePath   string
}

// Checkpoint persists an in-progress fold: the files produced so far and a
// fold-state carrying the chunk cache and the consecutive watermark, marked
// PendingSynthesis so the next fold knows synthesis did not complete. Called
// after every successful synthesis step, it bounds the cost of an
// interruption (Ctrl-C, crash) to at most one call.
func (w *Writer) Checkpoint(files map[string]string, chunks map[string]string, watermark uint64) error {
	base := VaultDir(w.Root)
	if err := os.MkdirAll(base, 0o700); err != nil {
		return fmt.Errorf("mkdir vault: %w", err)
	}
	for rel, content := range files {
		if err := writeVaultFile(base, rel, content); err != nil {
			return err
		}
	}
	return writeFoldState(base, FoldState{
		LastOffset:       watermark,
		FoldedAt:         time.Now().UTC(),
		Chunks:           chunks,
		PendingSynthesis: true,
	})
}

// Write materializes the vault files, INDEX.md, the CLAUDE.md managed
// block, and the watermark. dirs are 0700, files 0600. When prune is true
// (rebuild), stale concept files not present in res.Files are deleted first so
// the on-disk vault exactly matches the freshly synthesized set — otherwise a
// structure change would leave the old layout lingering beside the new one.
func (w *Writer) Write(res FoldResult, head uint64, eventsFolded int, prune bool) (WriteResult, error) {
	base := VaultDir(w.Root)
	if err := os.MkdirAll(base, 0o700); err != nil {
		return WriteResult{}, fmt.Errorf("mkdir vault: %w", err)
	}

	// Refuse to prune against an empty fresh set regardless of what the
	// caller asked: "delete everything the synthesis didn't produce" with a
	// synthesis that produced nothing means deleting the whole vault. That
	// can only be a failed fold, never an intended rebuild.
	if prune && len(res.Files) > 0 {
		keep := map[string]bool{}
		for p := range res.Files {
			keep[filepath.ToSlash(filepath.Clean(filepath.FromSlash(p)))] = true
		}
		if err := pruneStaleConcepts(base, keep); err != nil {
			return WriteResult{}, err
		}
	}

	written := 0
	// Stable order for deterministic output.
	paths := make([]string, 0, len(res.Files))
	for p := range res.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		if err := writeVaultFile(base, rel, res.Files[rel]); err != nil {
			return WriteResult{}, err
		}
		written++
	}

	// INDEX.md
	if strings.TrimSpace(res.Index) != "" {
		if err := writeVaultFile(base, indexFileName, res.Index); err != nil {
			return WriteResult{}, err
		}
		written++
	}

	claudePath := filepath.Join(w.Root, claudeFileName)
	if err := updateClaudeBlock(claudePath, res.ClaudeBlock); err != nil {
		return WriteResult{}, err
	}

	st := FoldState{
		LastOffset:       head,
		FoldedAt:         time.Now().UTC(),
		FilesWritten:     written,
		EventsFolded:     eventsFolded,
		Chunks:           res.Chunks,
		PendingSynthesis: res.PendingSynthesis,
	}
	if err := writeFoldState(base, st); err != nil {
		return WriteResult{}, err
	}

	return WriteResult{FilesWritten: written, VaultDir: base, ClaudePath: claudePath}, nil
}

// WritePruned materializes a reorganized vault (e.g. from a garden pass):
// it writes res.Files and INDEX.md, updates the CLAUDE.md managed block,
// and DELETES any existing files under topics/ and timeline/ that are not
// in the new set. It never touches .fold-state.json, INDEX.md, .DS_Store,
// or anything outside topics//timeline/. The watermark offset is preserved
// (garden consumes no new events); LastGardenedAt is stamped.
func (w *Writer) WritePruned(res FoldResult) (WriteResult, error) {
	base := VaultDir(w.Root)
	if err := os.MkdirAll(base, 0o700); err != nil {
		return WriteResult{}, fmt.Errorf("mkdir vault: %w", err)
	}

	// Build the set of new paths (slash-relative) for prune comparison.
	keep := map[string]bool{}
	for p := range res.Files {
		keep[filepath.ToSlash(filepath.Clean(filepath.FromSlash(p)))] = true
	}
	if err := pruneStaleConcepts(base, keep); err != nil {
		return WriteResult{}, err
	}

	written := 0
	paths := make([]string, 0, len(res.Files))
	for p := range res.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		if err := writeVaultFile(base, rel, res.Files[rel]); err != nil {
			return WriteResult{}, err
		}
		written++
	}

	if strings.TrimSpace(res.Index) != "" {
		if err := writeVaultFile(base, indexFileName, res.Index); err != nil {
			return WriteResult{}, err
		}
		written++
	}

	claudePath := filepath.Join(w.Root, claudeFileName)
	if err := updateClaudeBlock(claudePath, res.ClaudeBlock); err != nil {
		return WriteResult{}, err
	}

	// Preserve the existing watermark offset; only stamp the garden time.
	st, _, lerr := LoadFoldState(w.Root)
	if lerr != nil {
		return WriteResult{}, lerr
	}
	st.FilesWritten = written
	st.LastGardenedAt = time.Now().UTC()
	if err := writeFoldState(base, st); err != nil {
		return WriteResult{}, err
	}

	return WriteResult{FilesWritten: written, VaultDir: base, ClaudePath: claudePath}, nil
}

// pruneStaleConcepts deletes every .md concept file under the known vault
// content dirs that is not in keep. Includes the legacy dirs (topics, timeline,
// summaries) so an old vault migrates cleanly to the ICM layout. keep paths
// are slash-relative and cleaned.
//
// It also prunes the vault root so a rebuild fully migrates an old vault:
//   - Stale .md files not in keep (e.g. identity.md from a prior fold).
//     INDEX.md is never deleted (the writer manages it separately).
//   - Legacy artifacts by exact shape: the extensionless `index` file and
//     leaked `_l*_hint` keys — the ONLY non-.md files it may delete.
//   - Never directories; never dot-files (.fold-state.json, .DS_Store
//     survive); never any other non-.md file.
//
// Finally, legacy dirs left empty by the prune (topics, timeline, summaries,
// dev-log) are removed; a dir still holding stray non-.md user content is
// left alone.
func pruneStaleConcepts(base string, keep map[string]bool) error {
	legacyDirs := []string{"topics", "timeline", "summaries", "dev-log"}
	pruneDirs := append([]string{referenceDir, contractsDir, episodesDir}, legacyDirs...)
	for _, d := range pruneDirs {
		dir := filepath.Join(base, d)
		walkErr := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if info.IsDir() {
				return nil
			}
			rel, rerr := filepath.Rel(base, path)
			if rerr != nil {
				return rerr
			}
			relSlash := filepath.ToSlash(rel)
			if !strings.HasSuffix(relSlash, ".md") || keep[relSlash] {
				return nil
			}
			return os.Remove(path)
		})
		if walkErr != nil && !os.IsNotExist(walkErr) {
			return fmt.Errorf("prune %s: %w", d, walkErr)
		}
	}

	// Prune the vault root: stale .md files (e.g. identity.md from a prior
	// fold) and legacy artifacts (`index`, leaked `_l*_hint` keys).
	entries, err := os.ReadDir(base)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("prune vault root: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Never delete dot-files (.fold-state.json, .DS_Store, etc.).
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !strings.HasSuffix(name, ".md") {
			// The only non-.md files ever deleted: the legacy extensionless
			// `index` and leaked `_l*_hint` keys.
			hint, _ := filepath.Match("_l*_hint", name)
			if name != "index" && !hint {
				continue
			}
			if err := os.Remove(filepath.Join(base, name)); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("prune vault root %s: %w", name, err)
			}
			continue
		}
		// INDEX.md is written after prune; never delete it here.
		if name == indexFileName {
			continue
		}
		relSlash := name // root-level: relative path is just the filename
		if keep[relSlash] {
			continue
		}
		if err := os.Remove(filepath.Join(base, name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("prune vault root %s: %w", name, err)
		}
	}

	// Drop legacy dirs the prune left empty. os.Remove fails on a non-empty
	// dir (ENOTEMPTY) — ignored silently, so a dir holding stray user content
	// (non-.md files) is left alone.
	for _, d := range legacyDirs {
		_ = os.Remove(filepath.Join(base, d))
	}
	return nil
}

func writeVaultFile(base, rel, content string) error {
	// Guard against path escapes from a synthesizer.
	clean := filepath.Clean(filepath.FromSlash(rel))
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return fmt.Errorf("vault file path escapes vault dir: %q", rel)
	}
	full := filepath.Join(base, clean)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return fmt.Errorf("mkdir for %s: %w", rel, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", rel, err)
	}
	return nil
}

func writeFoldState(base string, st FoldState) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(base, foldStateFileName), data, 0o600)
}

// updateClaudeBlock replaces the MOM-managed block in CLAUDE.md, or
// creates/appends it as needed, never clobbering human content.
func updateClaudeBlock(path, block string) error {
	wrapped := claudeBeginMarker + "\n" + strings.TrimRight(block, "\n") + "\n" + claudeEndMarker + "\n"

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return os.WriteFile(path, []byte(wrapped), 0o600)
	}
	if err != nil {
		return fmt.Errorf("read CLAUDE.md: %w", err)
	}
	existing := string(data)

	bIdx := strings.Index(existing, claudeBeginMarker)
	eIdx := strings.Index(existing, claudeEndMarker)
	if bIdx >= 0 && eIdx > bIdx {
		before := existing[:bIdx]
		after := existing[eIdx+len(claudeEndMarker):]
		updated := before + strings.TrimRight(wrapped, "\n") + after
		return os.WriteFile(path, []byte(updated), 0o600)
	}

	// No markers — append, preserving existing human content.
	sep := ""
	if !strings.HasSuffix(existing, "\n") {
		sep = "\n"
	}
	updated := existing + sep + "\n" + wrapped
	return os.WriteFile(path, []byte(updated), 0o600)
}
