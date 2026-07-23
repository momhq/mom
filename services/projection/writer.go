package projection

import (
	"encoding/json"
	"fmt"
	"log"
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

	blockBeginMarker = "<!-- MOM:BEGIN -->"
	blockEndMarker   = "<!-- MOM:END -->"
)

// FoldState is the watermark persisted at .mom/vault/.fold-state.json.
type FoldState struct {
	LastOffset   uint64    `json:"last_offset"`
	FoldedAt     time.Time `json:"folded_at"`
	FilesWritten int       `json:"files_written"`
	EventsFolded int       `json:"events_folded"`
	// Chunks maps chunkID → vault-relative path for every chunk synthesized
	// in the last fold. Used by the next incremental fold to skip re-synthesis
	// of chunks whose source events haven't changed. Nil on fresh vaults.
	Chunks map[string]string `json:"chunks,omitempty"`
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
// the managed context block in each harness entry file.
type Writer struct {
	Root string
	// EntryFiles are additional project-root harness entry files that receive
	// the managed context block, beyond the always-written CLAUDE.md and
	// AGENTS.md. MOM is harness-agnostic: which extra files appear here is
	// the caller's (config-driven) decision.
	EntryFiles []string
}

// NewWriter binds a Writer to a project root.
func NewWriter(root string) *Writer { return &Writer{Root: root} }

// WriteResult reports what Write did.
type WriteResult struct {
	FilesWritten int
	VaultDir     string
	// EntryPaths are the absolute paths of the entry files whose managed
	// block was updated.
	EntryPaths []string
}

// Checkpoint persists an in-progress fold: the files produced so far and a
// fold-state carrying the chunk cache and the consecutive watermark. Called
// after every successful synthesis step, it bounds the cost of an
// interruption (Ctrl-C, crash) to at most one call — the next fold resumes
// from the watermark and the chunk cache skips everything already done.
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

	entryPaths, err := w.updateEntryFiles(res.ContextBlock)
	if err != nil {
		return WriteResult{}, err
	}

	if err := w.seedContextFile(); err != nil {
		log.Printf("mom: seed CONTEXT.md: %v", err)
	}

	st := FoldState{
		LastOffset:       head,
		FoldedAt:         time.Now().UTC(),
		FilesWritten:     written,
		EventsFolded:     eventsFolded,
		Chunks:           res.Chunks,
	}
	if err := writeFoldState(base, st); err != nil {
		return WriteResult{}, err
	}

	return WriteResult{FilesWritten: written, VaultDir: base, EntryPaths: entryPaths}, nil
}

// updateEntryFiles writes the managed context block into every configured
// harness entry file, always including CLAUDE.md and AGENTS.md, and returns
// their absolute paths.
func (w *Writer) updateEntryFiles(block string) ([]string, error) {
	files := unionStrings(w.EntryFiles, []string{"CLAUDE.md", "AGENTS.md"})
	paths := make([]string, 0, len(files))
	for _, name := range files {
		path := filepath.Join(w.Root, name)
		if err := updateContextBlock(path, block); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// seedContextFile writes the repo-root CONTEXT.md once, if absent. Unlike the
// entry files' managed block, CONTEXT.md carries no markers and is never
// touched again — it is a human map of MOM, not machine-owned output.
func (w *Writer) seedContextFile() error {
	path := filepath.Join(w.Root, "CONTEXT.md")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte(contextFileSeed), 0o600)
}

// contextFileSeed is the one-time CONTEXT.md content: a human orientation to
// MOM's architecture, written once and never overwritten by a fold.
const contextFileSeed = `# CONTEXT.md — human map of MOM

MOM writes this file once, the first time it folds this project, and never
overwrites it again. Edit it freely — unlike ` + "`.mom/vault/`" + `, it is yours.

## The one-way rule

corpus (the Ledger) → extraction (fold) → bundle (` + "`.mom/vault/`" + `)

Memory only ever flows this direction. Never edit the vault bundle by hand —
a fold regenerates it from the Ledger and any hand edits are lost.

## Vocabulary

- **Ledger** — the append-only source of truth: every captured event.
- **Librarian** — the only component allowed to read/write project data.
- **Editor / Watcher / Lens** — the capture pipeline that turns a session into Ledger events.
- **Fold** — the extraction step that projects the Ledger into the vault.
- **Concept / Episode** — a distilled subject file (concept) vs. a raw capture (episode).

## Invariants

- All data access routes through Librarian; nothing else touches the DB.
- Capture is privacy-gated — nothing is recorded without consent.
- English, TDD.

## Where to look

- ` + "`.mom/vault/identity.md`" + ` — what this project is.
- ` + "`.mom/vault/reference/INDEX.md`" + ` — decisions, conventions, durable facts.
- ` + "`.mom/vault/conventions/INDEX.md`" + ` — process/workflow rules.
`

// unionStrings returns the deduplicated concatenation of base then extra, in
// that stable order, first-occurrence wins.
func unionStrings(base, extra []string) []string {
	seen := make(map[string]bool, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, s := range append(append([]string{}, base...), extra...) {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
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
	legacyDirs := []string{"topics", "timeline", "summaries", "dev-log", "contracts"}
	pruneDirs := append([]string{referenceDir, conventionsDir, episodesDir}, legacyDirs...)
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

// updateContextBlock replaces the MOM-managed block in CLAUDE.md, or
// creates/appends it as needed, never clobbering human content.
func updateContextBlock(path, block string) error {
	wrapped := blockBeginMarker + "\n" + strings.TrimRight(block, "\n") + "\n" + blockEndMarker + "\n"

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return os.WriteFile(path, []byte(wrapped), 0o600)
	}
	if err != nil {
		return fmt.Errorf("read CLAUDE.md: %w", err)
	}
	existing := string(data)

	bIdx := strings.Index(existing, blockBeginMarker)
	eIdx := strings.Index(existing, blockEndMarker)
	if bIdx >= 0 && eIdx > bIdx {
		before := existing[:bIdx]
		after := existing[eIdx+len(blockEndMarker):]
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
