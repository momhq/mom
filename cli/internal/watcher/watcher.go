package watcher

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/momhq/mom/cli/internal/herald"
	"github.com/momhq/mom/cli/internal/recorder"
	"github.com/momhq/mom/cli/internal/ux"
)

// Source describes one Harness's transcript directory and parser.
type Source struct {
	// Harness is the name of the Harness (e.g. "claude", "windsurf").
	Harness string
	// TranscriptDir is the directory to watch (e.g. ~/.claude/projects/).
	// Tilde expansion is performed automatically.
	TranscriptDir string
	// Adapter parses Harness-specific JSONL lines.
	Adapter Adapter
}

// Config holds watcher configuration (mirrors .mom/config.yaml watcher block).
type Config struct {
	// TranscriptDir is the directory to watch (e.g. ~/.claude/projects/).
	// Tilde expansion is performed automatically.
	// DEPRECATED: use Sources instead. Kept for single-Harness compat.
	TranscriptDir string
	// ProjectDir is the absolute path of the project being watched.
	// Used to scope ingestion to the matching transcript subdirectory.
	// If empty, all transcripts are ingested (legacy behavior).
	ProjectDir string
	// MomDir is the path to .mom/ where raw/ and cursor files are written.
	MomDir string
	// Adapter parses Harness-specific JSONL lines.
	// DEPRECATED: use Sources instead. Kept for single-Harness compat.
	Adapter Adapter
	// Sources lists all Harness transcript directories to watch.
	// When set, TranscriptDir and Adapter are ignored.
	Sources []Source
	// DebounceMs is how long to wait after a Write event before reading.
	// Defaults to 300ms if zero.
	DebounceMs int
	// Bus is the Herald event bus. When set, the watcher publishes
	// RecordAppended events after each ingestion so Herald can trigger
	// downstream processors (Logbook, Drafter). May be nil.
	Bus *herald.Bus
	// SweepOnly when true skips fsnotify setup. The watcher can only be
	// used for one-shot Sweep() calls, not Run().
	SweepOnly bool
}

// resolvedSource is a Source after tilde expansion and project scoping.
type resolvedSource struct {
	harness string
	dir     string // resolved absolute path
	adapter Adapter
}

// Watcher watches Harness transcript directories and ingests new entries
// into .mom/raw/ using cursor-based incremental reads.
type Watcher struct {
	cfg        Config
	sources    []resolvedSource // resolved transcript sources
	fw         *fsnotify.Watcher
	mu         sync.Mutex
	timers     map[string]*time.Timer // debounce timers keyed by file path
	rawDir     string
	logFile    string
	p          *ux.Printer
	catchingUp bool // true during catchUp phase — suppresses per-file output
}

// New creates a Watcher. Call Run to start watching.
func New(cfg Config) (*Watcher, error) {
	if cfg.DebounceMs == 0 {
		cfg.DebounceMs = 300
	}

	// Normalize sources: if Sources is empty, build from legacy single fields.
	sources := cfg.Sources
	if len(sources) == 0 && cfg.TranscriptDir != "" {
		sources = []Source{{
			Harness:       "default",
			TranscriptDir: cfg.TranscriptDir,
			Adapter:       cfg.Adapter,
		}}
	}

	// Resolve each source: tilde expansion + project scoping.
	var resolved []resolvedSource
	for _, src := range sources {
		dir, err := expandTilde(src.TranscriptDir)
		if err != nil {
			return nil, fmt.Errorf("expanding transcript dir for %s: %w", src.Harness, err)
		}

		// Scope to project-specific subdirectory when ProjectDir is set.
		// Adapters that use a non-default slug convention (e.g. pi) implement
		// ProjectScoper to override the rule — critical for tight scoping,
		// otherwise the watcher falls back to scanning the entire transcript
		// dir and ingests sessions from other projects.
		if cfg.ProjectDir != "" {
			var slug string
			if scoper, ok := src.Adapter.(ProjectScoper); ok {
				slug = scoper.ProjectSlug(cfg.ProjectDir)
			} else {
				slug = projectSlug(cfg.ProjectDir)
			}
			scoped := filepath.Join(dir, slug)
			if info, serr := os.Stat(scoped); serr == nil && info.IsDir() {
				dir = scoped
			}
		}

		resolved = append(resolved, resolvedSource{
			harness: src.Harness,
			dir:     dir,
			adapter: src.Adapter,
		})
	}

	// Update legacy field for TranscriptDir() accessor.
	if len(resolved) > 0 {
		cfg.TranscriptDir = resolved[0].dir
	}

	rawDir := filepath.Join(cfg.MomDir, "raw")
	if err := os.MkdirAll(rawDir, 0755); err != nil {
		return nil, fmt.Errorf("creating raw dir: %w", err)
	}

	logsDir := filepath.Join(cfg.MomDir, "logs")
	_ = os.MkdirAll(logsDir, 0755)

	w := &Watcher{
		cfg:     cfg,
		sources: resolved,
		timers:  make(map[string]*time.Timer),
		rawDir:  rawDir,
		logFile: filepath.Join(logsDir, "watch.log"),
		p:       ux.NewPrinter(os.Stderr),
	}

	if !cfg.SweepOnly {
		fw, err := fsnotify.NewWatcher()
		if err != nil {
			return nil, fmt.Errorf("creating fsnotify watcher: %w", err)
		}
		w.fw = fw
	}

	return w, nil
}

// Run starts the watcher loop. It blocks until ctx-equivalent stop is called.
// Returns when the watcher is stopped or encounters an unrecoverable error.
// Call Stop to terminate.
func (w *Watcher) Run() error {
	// Watch all transcript directories recursively.
	for _, src := range w.sources {
		if err := w.addDir(src.dir); err != nil {
			w.logf("watching %s (%s): %v — skipping", src.dir, src.harness, err)
		}
	}

	// Process any existing files on startup (catch up on offline turns).
	w.catchingUp = true
	sessions, turns := w.catchUp()
	w.catchingUp = false

	if w.p != nil && sessions > 0 {
		w.p.Checkf("caught up: %s sessions, %s turns",
			w.p.HighlightValue(fmt.Sprintf("%d", sessions)),
			w.p.HighlightValue(fmt.Sprintf("%d", turns)))
		w.p.Blank()
	}

	for _, src := range w.sources {
		w.logf("watcher started on %s (%s)", src.dir, src.harness)
	}

	for {
		select {
		case event, ok := <-w.fw.Events:
			if !ok {
				return nil // watcher closed
			}
			w.handleEvent(event)

		case err, ok := <-w.fw.Errors:
			if !ok {
				return nil
			}
			w.logf("fsnotify error: %v", err)
		}
	}
}

// Stop shuts down the underlying fsnotify watcher.
func (w *Watcher) Stop() error {
	if w.fw == nil {
		return nil
	}
	return w.fw.Close()
}

// Sweep processes all existing transcript files (one-shot catch-up) and returns.
// Unlike Run(), it does not start the fsnotify event loop.
// Safe to call on a watcher created with SweepOnly: true.
func (w *Watcher) Sweep() (sessions int, turns int) {
	w.catchingUp = true
	sessions, turns = w.catchUp()
	w.catchingUp = false
	return
}

// TranscriptDir returns the resolved (scoped, tilde-expanded) transcript directory
// of the first source. For multi-source watchers, use TranscriptDirs().
func (w *Watcher) TranscriptDir() string {
	if len(w.sources) > 0 {
		return w.sources[0].dir
	}
	return w.cfg.TranscriptDir
}

// TranscriptDirs returns all resolved transcript directories with their runtime names.
func (w *Watcher) TranscriptDirs() map[string]string {
	dirs := make(map[string]string, len(w.sources))
	for _, src := range w.sources {
		dirs[src.harness] = src.dir
	}
	return dirs
}

// adapterForPath returns the adapter that owns the given file path
// by matching against resolved source directories.
func (w *Watcher) adapterForPath(path string) Adapter {
	for _, src := range w.sources {
		if strings.HasPrefix(path, src.dir) {
			return src.adapter
		}
	}
	// Fallback: legacy single-adapter config.
	return w.cfg.Adapter
}

// handleEvent dispatches fsnotify events.
func (w *Watcher) handleEvent(event fsnotify.Event) {
	path := event.Name

	// New directory created — watch it (Claude Code creates project dirs).
	if event.Has(fsnotify.Create) {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			_ = w.addDir(path)
			return
		}
	}

	// Only care about .jsonl files.
	if !strings.HasSuffix(path, ".jsonl") {
		return
	}

	// Skip subagent files (Phase 1 scope: top-level sessions only).
	if strings.Contains(path, "subagents") {
		return
	}

	if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) {
		adapter := w.adapterForPath(path)
		// Check project filter for adapters that need it (e.g. Windsurf).
		if pf, ok := adapter.(ProjectFilter); ok {
			if !pf.BelongsToProject(path) {
				return
			}
		}
		w.scheduleRead(path)
	}
}

// scheduleRead debounces rapid writes: resets the timer for the given path.
func (w *Watcher) scheduleRead(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	d := time.Duration(w.cfg.DebounceMs) * time.Millisecond
	if t, ok := w.timers[path]; ok {
		t.Reset(d)
		return
	}
	w.timers[path] = time.AfterFunc(d, func() {
		w.mu.Lock()
		delete(w.timers, path)
		w.mu.Unlock()
		w.ingestFile(path)
	})
}

// catchUp processes all existing .jsonl files across all sources on startup.
// Returns the number of sessions and total turns ingested.
func (w *Watcher) catchUp() (sessions int, turns int) {
	for _, src := range w.sources {
		_ = filepath.WalkDir(src.dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if strings.HasSuffix(path, ".jsonl") && !strings.Contains(path, "subagents") {
				// Check project filter for adapters that need it (e.g. Windsurf).
				if pf, ok := src.adapter.(ProjectFilter); ok {
					if !pf.BelongsToProject(path) {
						return nil
					}
				}
				n := w.ingestFile(path)
				if n > 0 {
					sessions++
					turns += n
				}
			}
			return nil
		})
	}
	return
}

// ingestFile reads new lines from the transcript file since the last cursor,
// normalizes them via the adapter, and appends to .mom/raw/.
// Returns the number of entries ingested.
func (w *Watcher) ingestFile(path string) int {
	sessionID := sessionIDFromPath(path)
	cursorFile := filepath.Join(w.rawDir, ".watch-cursor-"+sessionID)

	// Read cursor offset.
	offset := readWatchCursor(cursorFile)

	// Open and seek.
	f, err := os.Open(path)
	if err != nil {
		w.logf("opening %s: %v", path, err)
		return 0
	}
	defer f.Close()

	// If file shrank (truncation/rotation), reset cursor to re-ingest (#154).
	if offset > 0 {
		if info, err := f.Stat(); err == nil && offset > info.Size() {
			w.logf("file %s shrank (cursor %d > size %d) — resetting cursor", path, offset, info.Size())
			offset = 0
		}
	}

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			w.logf("seeking %s to %d: %v", path, offset, err)
			return 0
		}
	}

	// Read new content. Use ReadBytes('\n') instead of Scanner to distinguish
	// complete lines (terminated by \n) from truncated trailing data (#153).
	var entries []recorder.RawEntry
	var committedBytes int64
	reader := bufio.NewReaderSize(f, 2*1024*1024)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			// EOF without trailing \n — incomplete line, don't advance cursor past it.
			break
		}
		committedBytes += int64(len(line))
		raw := line[:len(line)-1] // strip trailing \n
		if len(raw) == 0 {
			continue
		}

		entry, ok := w.adapterForPath(path).ParseLine(raw, sessionID)
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}

	if committedBytes == 0 {
		return 0
	}

	// Write entries to .mom/raw/<YYYY-MM-DD>.jsonl.
	if len(entries) > 0 {
		if err := w.writeEntries(entries); err != nil {
			w.logf("writing entries from %s: %v", path, err)
			return 0
		}
		if w.p != nil && !w.catchingUp {
			sid := sessionIDFromPath(path)
			short := sid
			if len(short) > 8 {
				short = short[:8]
			}
			rt := w.adapterForPath(path).Name()
			w.p.Checkf("ingested %d turns from %s — %s", len(entries), w.p.HighlightValue(rt), w.p.HighlightValue(short))
		}
	}

	// Advance cursor only past complete lines (#153).
	writeWatchCursor(cursorFile, offset+committedBytes)

	// Publish to Herald so downstream processors (Logbook, Drafter) run.
	if len(entries) > 0 && w.cfg.Bus != nil {
		w.cfg.Bus.Publish(herald.RecordAppended, map[string]any{
			"transcript_path": path,
			"session_id":      sessionID,
			"count":           len(entries),
			"mom_dir":         w.cfg.MomDir,
			"runtime":         w.adapterForPath(path).Name(),
		})
	}

	return len(entries)
}

// writeEntries appends normalized entries to today's raw JSONL file.
func (w *Watcher) writeEntries(entries []recorder.RawEntry) error {
	now := time.Now().UTC()
	dailyFile := filepath.Join(w.rawDir, now.Format("2006-01-02")+".jsonl")

	f, err := os.OpenFile(dailyFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening daily file: %w", err)
	}
	defer f.Close()

	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			continue
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// addDir adds a directory and all its subdirectories to the fsnotify watcher.
func (w *Watcher) addDir(dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible paths
		}
		if d.IsDir() {
			if werr := w.fw.Add(path); werr != nil {
				w.logf("watching dir %s: %v", path, werr)
			}
		}
		return nil
	})
}

// sessionIDFromPath extracts a session ID from a .jsonl transcript path.
// Claude Code paths: ~/.claude/projects/{project-slug}/{sessionId}.jsonl
// We use the filename stem as the session ID.
func sessionIDFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".jsonl")
}

// readWatchCursor reads the byte offset stored in the cursor file.
// Returns 0 if the file doesn't exist or is unreadable (fresh start).
func readWatchCursor(cursorFile string) int64 {
	data, err := os.ReadFile(cursorFile)
	if err != nil {
		return 0
	}
	var offset int64
	if _, err := fmt.Sscan(string(data), &offset); err != nil {
		return 0
	}
	return offset
}

// writeWatchCursor persists a byte offset to the cursor file.
func writeWatchCursor(cursorFile string, offset int64) {
	_ = os.WriteFile(cursorFile, []byte(fmt.Sprintf("%d", offset)), 0644)
}

// projectSlug converts an absolute project path to the Claude Code project
// directory slug format: replace "/" with "-".
// e.g. "/Users/vmarino/Github/discovery" → "-Users-vmarino-Github-discovery"
func projectSlug(projectDir string) string {
	return strings.ReplaceAll(projectDir, "/", "-")
}

// expandTilde replaces a leading "~" with the user's home directory.
func expandTilde(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[1:]), nil
}

// logf appends a timestamped message to the watcher log file, best-effort.
func (w *Watcher) logf(format string, args ...any) {
	f, err := os.OpenFile(w.logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(f, "%s watcher: "+format+"\n", append([]any{ts}, args...)...)
}
